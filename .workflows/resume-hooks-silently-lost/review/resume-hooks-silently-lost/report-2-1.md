TASK: resume-hooks-silently-lost-2-1 — The Live Enumeration Answers With Pane Tokens

ACCEPTANCE CRITERIA:
1. `@portal-pane-id` appears as a literal in exactly one place in non-test source — `state.PortalPaneIDOption` — and the enumeration format is composed from it
2. `ListAllPaneHookKeys` returns one `PaneHookRow` per live pane, including panes carrying no token, whose `Token` is `""` and whose `Location` is still populated
3. A row's `Location` survives an embedded `|` in the session name (first-separator split, unbounded remainder)
4. Empty tmux output returns a non-nil empty slice; a `list-panes -a` failure returns `(nil, err)` and never an empty slice
5. `runHookStaleCleanup`'s empty-set guard counts rows: with rows present and every token empty it does not take the guard branch, emits no guard WARN, and calls `CleanStale` with an empty live-token slice
6. `CleanStale` and `checkStaleHooks` receive only non-empty tokens, through one shared projection helper used by both call sites
7. With a server holding hooks and zero stamped panes, the sweep emits no `reason=empty-pane-read` WARN and deletes no non-token-shaped entry
8. A token-shaped key absent from the live token set is still deleted when rows are present
9. The daemon sweep's live-pane fixture still measures liveness: `TestMaybeRunHookCleanup_RunsAndResetsOnceIntervalElapsed` enumerates a row carrying a token and keeps its retained entry keyed by that same token
10. `go test ./...` and `go test -tags integration -p 1 ./...` both pass

STATUS: complete

SPEC CONTEXT:
§2.1 requires the `@portal-pane-id` option name to be declared once, in `internal/state`, with every format string / option argument / stamp composing from it — that single declaration is what retires the two `@portal-id` literal-binding guards rather than re-pointing them. §3.3 specifies `ListAllPaneHookKeys()` becoming an all-pane enumeration returning one row per live pane, each carrying the pane's token (empty for an unstamped pane) and its `<session>:<window>.<pane>` location, with the location field explicitly display-only so rendering the same shape as the positional siblings couples nothing to them; one `list-panes -a -F` read over a two-field format serves the sweep, `portal doctor` and `hook list` alike. §5.4 is the load-bearing constraint this task exists for: under lazy stamping zero stamped panes is the ordinary steady state, so the mass-deletion guard's question ("did the tmux read succeed?") is answered by the pane row count, never by the token set — the two questions must not be conflated in either consumer. The Corrigenda are consistent with the delivered shape (`judgeAgainstLivePanes` is now the exported `hooksweep.JudgeAgainstLivePanes`, and the decline vocabulary grew to six reasons in later phases).

IMPLEMENTATION:
- Status: Implemented (verified in the delivered end state, which later phases moved but did not weaken)
- Location:
  - `internal/state/markers.go:26` — `const PortalPaneIDOption = "@portal-pane-id"`, sited beside `SkeletonMarkerPrefix` / `RestoringMarkerName` / `BootstrappedMarkerName` as specified
  - `internal/tmux/tmux.go:637-640` — `PaneHookRow{Token, Location}` with the doc the task prescribed (`Token` empty for an unstamped pane; `Location` display-only and never a key)
  - `internal/tmux/tmux.go:645` — `paneHookRowSeparator = "|"`, with the non-whitespace rationale recorded at :642-644
  - `internal/tmux/tmux.go:650-651` — `paneHookRowFormat`, composed by constant concatenation from `HookKeyFormat` (itself `"#{" + state.PortalPaneIDOption + "}"` at :631) plus the location half
  - `internal/tmux/tmux.go:658-664` — `ListAllPaneHookKeys() ([]PaneHookRow, error)`, reading through the existing `ListAllPanesWithFormat`
  - `internal/tmux/tmux.go:668-681` — `parsePaneHookRows`, `strings.Cut` at the first separator, blank-line skip, wrapped error naming the offending line, `[]PaneHookRow{}` literal for the non-nil-empty contract
  - `internal/hooksweep/sweep.go:30-32` — the seam (`PaneHookLister`, formerly `cmd`'s `AllPaneLister`) with the rewritten two-field doc comment
  - `internal/hooksweep/sweep.go:85-105` — `JudgeAgainstLivePanes`, guard on `len(rows) == 0 && entries > 0`
  - `internal/hooksweep/sweep.go:110-118` — `liveTokensFrom`, the single shared projection
  - `internal/hooksweep/sweep.go:141` — the counts DEBUG line carrying `panes = view.PaneRows`
  - `cmd/doctor.go:379-387` — `checkStaleHooks` re-pointed through the same `JudgeAgainstLivePanes`, so `hooks.StaleKeys` gets `view.LiveTokens`
- Notes:
  - AC1 verified by enumeration, not assumed: `@portal-pane-id` occurs as a string literal in non-test source exactly once (`internal/state/markers.go:26`). The only other non-test occurrence in the tree is prose inside `PaneHookRow`'s doc comment (`internal/tmux/tmux.go:634`), which is a documentation reference and not a second code literal. Four occurrences exist in test files (`internal/tmux/target_composition_guard_test.go:336`, `internal/tmux/target_type_test.go:61,68,70`), all golden argv/format literals in guards whose whole purpose is to pin the rendered string; the criterion scopes to non-test source and they are legitimate goldens.
  - AC6 holds through one helper reached two ways: both `CleanStale`'s enumeration closure (`sweep.go:139-146`) and `checkStaleHooks` (`doctor.go:379`) call `JudgeAgainstLivePanes`, which is the sole caller of `liveTokensFrom`. There is no second projection anywhere in the tree.
  - The rows-versus-tokens separation is airtight in both consumers: the guard reads `len(rows)`, the comparison reads `View.LiveTokens`. `View.PaneRows` is only ever read past the `Decline.Declined()` check, so the "meaningful only where Enumerated is set" comment at `sweep.go:47-49` cannot be falsified by either call site.
  - Ordering held: the task landed before registration wrote tokens (2-2), so no freshly-written token key was ever exposed to a still-positional live set.
  - The import added to `internal/tmux` (`internal/state`) introduces no cycle — `internal/state` imports only `tmuxout`/`tmuxerr` from that direction, verified.
  - Token alphabet is `[A-Za-z0-9]` (`internal/nanoid/nanoid.go:17`), so a minted token can never contain the `|` separator; the first-cut split is sound against every token the mint produces.
  - Deviation from the plan text, judged sound: the plan named `cmd/run_hook_stale_cleanup.go` and its `AllPaneLister`. Later phases moved the whole cycle into `internal/hooksweep` and renamed the seam `PaneHookLister`. Every substantive requirement (row-counting guard, shared projection, `panes=` as row count, rewritten doc comment) survives the move intact — this is a relocation, not a loss.
  - All four issues raised in `fix-tracking-resume-hooks-silently-lost-2-1.md` are resolved in the end state: the empty-key subtest exists (`internal/hooksweep/sweep_test.go:459`), the counts-line subtest now asserts the value (`sweep_test.go:454`), the doctor row-versus-token branch has coverage (`cmd/doctor_test.go:977`), and the `cmd/doctor.go` comment correction landed verbatim (`doctor.go:356-362`).

TESTS:
- Status: Adequate
- Coverage: Every test named in the task's Tests list exists and measures what its name claims.
  - `internal/tmux/pane_hook_rows_test.go:13,30,40,53,68,83` — one row per live pane over mixed stamped/unstamped output; the leading-separator row parsing to `{Token:"", Location:"sess:0.0"}`; first-separator-only split proven with both `tokA|pipe|name:0.0` and a `:`/`.`-bearing location; the separator-less row erroring with the offending line named AND `rows == nil` asserted (so a half-parsed set cannot reach a caller); non-nil empty slice on empty output; and the argv assertion pinning one `list-panes -a -F` read composed from `state.PortalPaneIDOption`.
  - `internal/tmux/list_all_pane_hookkeys_realtmux_test.go:11` — real tmux, one stamped pane and an unstamped split sibling in one read, asserting both rows, the correct token and the empty sibling token (which also pins the no-inheritance property).
  - `internal/tmux/list_all_pane_hookkeys_realtmux_test.go:35` — server killed, asserting `errors.AsType[*tmux.CommandError]` and explicitly `rows != nil` → error, covering AC4's "never an empty slice" half.
  - `internal/hooksweep/sweep_test.go:381` — the AC5/AC7 case: rows present, every token empty, unjudgeable entries seeded; asserts no `DeclineReason`, zero records at or above WARN, and the `hooks.json` bytes unchanged.
  - `internal/hooksweep/sweep_test.go:405,423` — AC8 (a token-shaped key absent from the live token set still reaped when rows are present) and the zero-row branch still standing down with the entry retained.
  - `internal/hooksweep/sweep_test.go:444,459` — the counts line asserted at `panes == 4` for four unstamped rows (mutation-effective against a token count), and the empty-key reap proving the `liveTokensFrom` filter is load-bearing.
  - `cmd/doctor_test.go:957,977,994` — doctor's zero-row not-evaluable, its evaluate-on-unstamped-rows counterpart, and the "does not reach the no-hooks door with unstamped rows present" case.
  - `cmd/state_daemon_hook_cleanup_test.go:23,67` — AC9: `livePaneRowOut = hookstest.LiveSeedA + "|live:0.0"` carries a token, and the retained entry is keyed on that same token (`StaleHookSeed` keys the live entry on `LiveSeedA`), so the entry survives on liveness rather than on Phase 1's shape retention. The comment at :20-22 states exactly that trap, so a future edit that drops the token is visibly wrong.
  - `cmd/hook_prune_rename_survival_integration_test.go:20` — the re-pointed rename-survival test: stamps `LiveSeedA` on the pane with raw tmux, renames the session, confirms the token still enumerates, then asserts the token-keyed entry survives the sweep while a token-shaped key naming no row is reaped in the same cycle.
  - `internal/tmux/hookkey_cross_site_realtmux_test.go` — narrowed in this task per the Edge Cases, then restored to its end-state cross-site form in 2-2, which is what it holds now.
- Notes:
  - Not over-tested. The unit suite covers the parse contract, the real-tmux suite covers what only a real server can answer (tmux's own per-pane option resolution and the failure argv), and neither duplicates the other.
  - Fixture hygiene is right: `tmuxtest.StampPaneToken` (`internal/tmuxtest/stamp.go:13`) stages through raw tmux rather than the client method under test, so no fixture certifies itself.
  - Lane placement is correct: the two `*_realtmux_test.go` files are per-test `-L` socket client tests with no daemon and no built binary, which CLAUDE.md admits to the unit lane; the rename-survival test carries `//go:build integration`.
  - AC10 not independently executed — test execution is outside this review's remit. Judged by reading: the seam fakes were consolidated onto a single `stubStaleSweepReader` (`cmd/hookkey_vocabulary_test.go:93`) satisfying `hooksweep.Reader`, every fixture the task listed as a compile-time blocker is re-pointed to the two-field row shape, and the retired `cmd/hookkey_no_regression_upgrade_test.go` is gone with its helper's job re-homed. No unresolved reference to the old `[]string` signature exists anywhere in the tree.

CODE QUALITY:
- Project conventions: Followed. `internal/state` stays a leaf; `internal/tmux`'s import of it was pre-existing via `portal_saver.go`; the `hooks` log component and the `panes`/`entries` attrs are unchanged from the pre-existing counts line, so no call-site vocabulary was invented.
- SOLID principles: Good. The seam is declared consumer-side as a one-method interface; `liveTokensFrom` is the single projection with one caller, so the row/token distinction has exactly one place to go wrong.
- Complexity: Low. `parsePaneHookRows` is one loop with one branch; `JudgeAgainstLivePanes` is one early return plus one guard.
- Modern idioms: Yes — `strings.Cut` for the first-separator split and `strings.SplitSeq` for the line walk, both consistent with the `modernize` linter the project enables.
- Readability: Good. Each non-obvious decision carries its reason at the point of decision: why the separator must be non-whitespace (`tmux.go:642-644`), why the location half deliberately does not reuse `StructuralKeyFormat` (`tmux.go:647-649`), why a failure must not degrade to an empty slice (`tmux.go:655-657`), and why the guard counts rows (`sweep.go:94-97`).
- Comment accuracy (changed code): Every comment introduced by this task holds against the code. The `[]PaneHookRow{}` literal really does deliver the non-nil-empty contract the doc claims; the separator comment's premise (the commander trims the whole output) is true of `Commander.Run` as documented at `internal/tmux/tmux.go:33-34`.
- Issues: None in the delivered change-set.

BLOCKING ISSUES:
- None.

FINDINGS:
- [out-of-scope] [contained] internal/tmux/tmux.go:601 — `ListAllPanesWithFormat`'s doc says it returns "the raw untrimmed output", but its body calls `c.cmd.Run` (`tmux.go:604`), which the `Commander` contract documents as trimming surrounding whitespace (`tmux.go:33-34`; `RunRaw` is the verbatim variant). Change "returning the raw untrimmed output" to say the output is returned trimmed. FAILS: a caller that needs trailing whitespace preserved — the exact concern this task's own `paneHookRowSeparator` comment forty lines below reasons about, and reasons about correctly — is told by this doc that the whitespace survives, so a future change of the separator to a space or tab would look safe and would silently drop the separator from the first row whenever that pane is unstamped. Marked out-of-scope because the false claim predates this work unit (introduced 2026-08-11 in `915e7fcb`, a comments audit) and this task added a caller of the function without touching its doc; it is noted here only because the two comments now contradict each other in one file and this task's correctness argument depends on the newer one. Remedy is comment text alone, so it is never blocking.
