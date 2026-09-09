TASK: resume-hooks-silently-lost-2-5 — A Moved Pane Keeps Its Hook (tick-91a738)

ACCEPTANCE CRITERIA:
- Token resolves unchanged after `break-pane`, after `kill-window` under `renumber-windows on`, after `move-pane` back, and after `move-pane` into another session — each asserted immediately after its own move
- Each move is proven to have moved: the enumerated `Location` differs from the value recorded before it
- Exactly one enumerated row carries the token after every move
- The token survives `respawn-pane -k`
- A pane created by `split-window` or `new-window` from the stamped pane enumerates with an empty token
- Every assertion runs through `ResolveHookKey` / `ListAllPaneHookKeys` / `SetPaneOption` rather than raw tmux format reads
- `renumber-windows on` is set explicitly on the fixture server, not assumed
- The suite runs in the unit lane and spawns no daemon and no built binary

STATUS: complete

SPEC CONTEXT:
§2.1 records the hand-verified survival table for the `@portal-pane-id` pane user-option (`break-pane`, `kill-window` under `renumber-windows on`, `move-pane` back, `respawn-pane -k`, `rename-session`) and the no-inheritance property (a pane created by `split-window`/`new-window` reads back empty). §3.1 makes the hook key the token alone — no session component, no coordinates — which is why the cross-session move is a named case rather than a variation: a composite `<portal-id>:<token>` key would have failed it. §9.1 places fast real-tmux *client* tests in the unit lane under `internal/tmux/*_realtmux_test.go`. §9.2's table row "A pane that moves keeps its hook" names exactly this suite and its four moves, and states the gap it closes: no test ever moved a pane. §9.5 leaves the positional siblings to Phase 3's non-contiguous-window-index restore test, which this task correctly does not pre-empt. No corrigendum touches this task's subject (the §9.1 corrigendum only widens the integration-lane rule to include *building* a binary — this suite builds none).

IMPLEMENTATION:
- Status: Implemented (with one deliberate, later-task divergence — see Notes)
- Location:
  - `internal/tmux/hookkey_moved_pane_realtmux_test.go` — the whole suite: `TestHookKeyDurability_PaneMoves:12` (break-pane `:31`, kill-window `:39`, move back `:48`, cross-session `:53`, sole-row `:66`), `TestHookKeyDurability_RespawnPane:74`, `TestHookKeyDurability_NoInheritance:97` (split `:107`, new window `:115`), and the `paneTokenProbe` helpers `:128`-`:264`
  - `internal/tmux/hookkey_realtmux_shared_test.go:35` (`seedRealTmuxServer`), `:62` (`hookKeyFixture`), `:92` (`sessionPaneIDs`) — the shared fixture preamble this suite's `newStampedPaneFixture:138` is built on
  - `internal/tmuxtest/stamp.go:13` (`StampPaneToken`) — the stamp route
- Notes:
  - Criterion-by-criterion: all four moves are driven and asserted in their own subtest (`:35`, `:41`, `:49`, `:55`), each through `assertSurvivedMove:200`, which fatals if the location did not change (`:204`) — so a move that did not move fails rather than passing vacuously. The kill-window case additionally pins that the *window half* renumbered (`:43`) and the cross-session case that the *session half* changed and equals the destination (`:57`-`:63`). `tokenRow:168` fatals unless exactly one enumerated row carries the token, and every assertion path calls it, so the "exactly one row" invariant holds after every move rather than only at the end. `RespawnPane` (`internal/tmux/tmux.go:687`) composes `respawn-pane -k`, so `TestHookKeyDurability_RespawnPane:86` is the `-k` case the criterion names, and it also pins the location *unchanged* (`:92`) — the right assertion for an operation that must not move the pane. Non-inheritance is asserted for both `SplitWindow` and `NewWindow` via `assertCreatedPaneCarriesNoToken:213`, which distinguishes "empty token" from "gone pane" because `ResolveHookKey` errors on a target no pane answers to (`internal/tmux/tmux.go:236`-`238`), and also re-asserts the stamped pane is still the sole holder.
  - `renumber-windows on` is set explicitly on the fixture server (`:145`) with a comment stating why (vanilla tmux has it off), satisfying the criterion the plan flags as the one that would otherwise make the kill-window case pass for the wrong reason.
  - Lane: no build constraint on the file, no daemon spawned, no `portal` binary built or exec'd; the server is an isolated per-test socket from `tmuxtest.New` behind `SkipIfNoTmux`. `tmuxtest` uses an absolute `-S` path rather than `-L` (`internal/tmuxtest/socket.go:22`-`23`: `-L` under `t.TempDir()` overruns darwin's ~104-byte socket-path cap) — a pre-existing harness property, and CLAUDE.md's cleanup section recognises both forms as disposable test sockets. Nothing touches the ambient server: every command is issued on the fixture socket with `-f /dev/null`.
  - Divergence (deliberate, sound): the plan's Do bullet and criterion 6 name `client.SetPaneOption` as the stamp route, and the task's own commit (4285110e) used it. A later task (70c7e50a, 2-7 — "one real-tmux fixture preamble and one way to stamp a pane") moved the stamp to `tmuxtest.StampPaneToken`, which writes the option with raw tmux so a fixture never stages itself through a client call (`internal/tmuxtest/stamp.go:11`-`15`). Nothing is lost: the argv is byte-identical to the one `internal/tmux/pane_option_test.go:25` pins `SetPaneOption` to (`set-option -p -t <target> @portal-pane-id <value>`), `internal/tmux/resolve_hookkey_realtmux_test.go:97` pins that same raw form failing against a bogus pane on a real server, and the property this suite exists to pin — that a stamped pane's token outlives a rearrangement — is a tmux behaviour independent of which process wrote the stamp. Both *reads* the criterion cares about (`ResolveHookKey`, `ListAllPaneHookKeys`) are still Portal's own.
  - Scope respected: the suite touches only `internal/tmux` test files and pre-empts none of Phase 3's `@portal-id` removal work.

TESTS:
- Status: Adequate
- Coverage: All eight of the plan's named tests exist with the wording it prescribed. The measured surface is Portal's own argv on both reads: `client.ResolveHookKey` (`:188`) and `client.ListAllPaneHookKeys` (`:170`). Raw tmux is used only for the arrangement — the moves themselves (`break-pane`/`kill-window`/`move-pane`, which have no client method), the `#{pane_id}` handle read (`paneIDAt:242`), and the stamp — which is what the task sanctions: the `%N` id is explicitly the test's handle and never the identity under test (`:124`-`:127`).
- Notes:
  - Would it fail if the feature broke? Yes, on both directions of the defect. Reintroduce a positional component into `HookKeyFormat` or the enumeration format and every `assertSurvivedMove` fails at `:192` or `:181`; make a split inherit the stamp and `:219` fails; break the restore-facing property and `TestHookKeyDurability_RespawnPane` fails. The "proven to have moved" guard (`:204`) is what stops the suite degrading into a no-op if a future tmux stops renumbering or a move silently becomes a no-op.
  - Not over-tested: three test functions over two fixtures, no redundant mocking, no assertions on implementation detail. The one near-redundancy is the final subtest `:66` — `tokenRow` has already enforced the sole-row invariant on every prior assertion, so this subtest can only fail on non-determinism; it costs one extra enumeration and is the plan's named test, so it stays.
  - Order dependence is by design and documented (`:27`-`:30`): the four moves compose on one pane, which is what lets each be asserted immediately after its own move (the plan's edge case: "a single end-state assertion cannot say which operation broke the key"). The community `golang-testing` default prefers independently-runnable tests, but the project plan prescribes the chained sequence and the sequence is what gives the kill-window case a broken-out window to renumber.
  - No `t.Parallel()` anywhere in the suite, per CLAUDE.md.

CODE QUALITY:
- Project conventions: Followed. Unit-lane real-tmux client test in `internal/tmux/*_realtmux_test.go` with `SkipIfNoTmux` + a per-test isolated socket; `"it …"` subtest naming; helpers carry `t.Helper()` (`:169`, `:187`, `:201`, `:214`, `:230`, `:243`, `:249`); no ambient-tmux, filesystem or process reach beyond the fixture.
- SOLID principles: Good. `paneTokenProbe` has one job (hold a stamped pane and what it was last seen as) and the three assertion helpers each state one property; the fixture preamble is shared through `seedRealTmuxServer` rather than copied.
- Complexity: Low. Straight-line subtests; the only branching is in `newPaneID:248` and `splitLocation:229`, both trivial.
- Modern idioms: Yes — `strings.Cut` for the location split, `for range 2` for the window seeding (`:147`), map-set diff for the created-pane identification.
- Readability: Good. Every non-obvious choice carries its reason: why `break-pane` names its destination (`:32`-`:34`), why `renumber-windows on` must be set (`:143`-`:145`), why `respawn-pane -k` belongs here (`:85`), why zero-or-many token rows are each a distinct defect (`:165`-`:167`), why a move that did not move proves nothing (`:197`-`:199`).
- Comment accuracy: Checked against the code. `newStampedPaneFixture`'s doc (`:135`-`:137`) — "three-window session whose second window holds two panes, stamps the token on the second of those" — matches the seeding (initial window + two from `:147`-`:151`, split of `:1` at `:152`, stamp on `:1.1` at `:157`-`:158`). "Restore arms every pane this way" (`:85`) is true of `internal/restore`'s arm phase. No comment references a task id, phase or spec section.
- Issues: None.

BLOCKING ISSUES:
- None.

FINDINGS:
- None.
