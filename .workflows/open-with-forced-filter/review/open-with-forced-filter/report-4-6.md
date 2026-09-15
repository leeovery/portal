TASK: open-with-forced-filter-4-6 (tick-3024a0) — Classify a command-only line with no target as a picker invocation

ACCEPTANCE CRITERIA:
- `isTUIPath` answers both disjuncts from the same positional helper — `preDashPositionals` for the no-target arm, `searchFormPositionals` (itself routed through it) for the search arm.
- `portal open -- claude` classifies as the TUI path, so its cold boot takes the concurrent bootstrap and the loading page, and its accumulated soft warnings stay in the sink for the notice band rather than being written to the terminal the picker is about to claim.
- Every other classified line keeps the verdict the phase already pins: bare `open` true, `-f text` true, `-e cmd` true, `~/dir` false, `api blog` false, `~/Code/api -- ls /tmp` false, `/port` true, `/port -s api` false.
- `TestOpenCommand_CommandNoTarget_DashDash_OpensProjectsPicker` (`cmd/open_test.go`) still passes unchanged.
- `go test ./...` is green.

STATUS: complete

SPEC CONTEXT:
§7 (cold-start classification) of `.workflows/open-with-forced-filter/specification/open-with-forced-filter/specification.md`. §7.1 (line 288) states a sigil invocation is classified as a picker invocation — concurrent bootstrap, loading page, in-TUI warning route. §7.2 (line 292) now quotes the classifier verbatim: "`cmd/root.go`'s `isTUIPath` → `return len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0`, after a domain-pin veto" — i.e. the spec was amended to the shape this task produced. §7.6 (line 314) and the Corrigendum 2026-09-14 (line 486) record that this change admitted a *second* member to the concurrent-bootstrap set alongside the sigil — `portal open -- <command>` — and that the command-pending picker had no loading gate, tracked as a separate defect (closed by phase 7's tasks 7-1 `tick-417c10` and 7-2 `tick-424d49`).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/root.go:177` — `return len(preDashPositionals(cmd, args)) == 0 || len(searchFormPositionals(cmd, args)) > 0` (the whole production change; commit `eae96c47c` replaced `len(args) == 0` in the first disjunct).
  - Helpers it routes through: `cmd/open_search.go:29-34` (`preDashPositionals`) and `cmd/open_search.go:17-25` (`searchFormPositionals`, which itself iterates `preDashPositionals`).
  - Consumers, both unedited and taking the corrected verdict: `cmd/root.go:112` (latch-satisfied stderr-drain gate), `cmd/root.go:146` (post-bootstrap stderr-drain gate), `cmd/root.go:190` (`shouldRunConcurrentBootstrap`).
- Notes:
  - Criterion 1 holds exactly: the function body (`cmd/root.go:173-178`) reads `args` only by handing it to the two helpers — no raw-slice read survives anywhere in it.
  - Criterion 3 verified case by case against the current code. `open -- claude`: pflag records `argsLenAtDash == 0`, so `preDashPositionals` returns `args[:0]` → first disjunct true. `~/Code/api -- ls /tmp`: dash index 1 → pre-dash `["~/Code/api"]`, so neither disjunct fires → false, and the post-dash `/tmp` is never offered to `IsSearchSigil`. `api blog` → 2 pre-dash, no sigil → false. `/port` → 1 pre-dash, 1 sigil → true. `/port -s api` → domain-pin veto at `cmd/root.go:174` → false. Bare `open`, `-f text`, `-e cmd` all carry zero positionals → true. `~/dir` → 1 pre-dash, no sigil → false.
  - No slice-bound hazard is introduced. `preDashPositionals` slices `args[:dash]`, and every production caller of `isTUIPath` runs from `PersistentPreRunE`, after cobra's single `ParseFlags` on that flagset, where `ArgsLenAtDash() <= len(args)` holds. The completion path's known dash-index quirk (documented at `cmd/open_search.go:40-45`) does not reach `isTUIPath`: `__complete` is bootstrap-exempt and would fail the `cmd.Name() != "open"` guard regardless, and the completer routes through `completingPreDashPositional`, which absorbs the quirk with its `<=`.
  - Two deliberate divergences from the *wording* of criterion 2, both settled after this task by later, spec-recorded work — neither is a loss, so neither is reported as a finding:
    1. "and the loading page" — a command-pending model is forced onto `PageProjects` by `WithCommand` (`internal/tui/model.go:529-536`), applied after the options in `tui.Build` (`internal/tui/build.go:174-176`), so it overrides the `PageLoading` that `WithServerStarted(true)` sets (`internal/tui/model.go:661-668`). That was known at authoring time (Corrigendum, specification.md:486) and was closed on its own terms by task 7-2, which holds the mint behind the bootstrap's terminal event and announces the wait in the Projects band instead of adding a loading page. The property criterion 2 exists to protect — the picker cannot act on a half-bootstrapped server — is delivered, by a different mechanism.
    2. "stay in the sink for the notice band" — the warnings do stay in the sink here (`cmd/root.go:146-148` no longer drains for this line) and are staged on the model by `stageBootstrapWarningsOnModel` (`cmd/bootstrap_warnings.go:44-52`, called at `cmd/open.go:741`); for a command-pending model they surface at teardown rather than in the notice band, which is the route tasks 6-4 and 7-1 chose on purpose. The warning is not dropped, which is the substance.

TESTS:
- Status: Adequate
- Coverage:
  - `cmd/concurrent_bootstrap_gate_test.go:79-87` — `"open -- cmd (command after the separator, no target) IS the TUI path"`: builds `openProbeCmd()`, `ParseFlags([]string{"--", "claude"})`, asserts `isTUIPath(c, c.Flags().Args())` is true. This is the `ArgsLenAtDash() == 0` shape — exactly the input the old raw-slice read got wrong, so the test fails if the first disjunct is reverted to `len(args) == 0`.
  - `cmd/concurrent_bootstrap_gate_test.go:236-244` — `"it routes concurrent for open -- cmd (command after the separator, not satisfied)"`: same parsed command through `shouldRunConcurrentBootstrap` with `latchSatisfied=false` and a `probeClient()`, so the routing consequence is pinned, not just the predicate.
  - The regression guard the task named is intact and unedited: `cmd/concurrent_bootstrap_gate_test.go:131-139` — `"words after a -- separator are never inspected"` (`~/Code/api -- ls /tmp` must stay false). It is what stops the fix from being over-applied to a line that has a pre-dash target.
  - The rest of the verdict table is pinned by the existing subtests at `cmd/concurrent_bootstrap_gate_test.go:29-160` (bare, positional, four domain pins, `-f`, `-e`, repeated pins, non-open, sigil forms, sigil-at-index, path positionals, two targets, sigil beside a pin, sigil≡`-f` parity).
  - `TestOpenCommand_CommandNoTarget_DashDash_OpensProjectsPicker` (`cmd/open_test.go:2452`) is untouched by commit `eae96c47c` (the commit's whole diff is `cmd/root.go` + `cmd/concurrent_bootstrap_gate_test.go`, +21/−1), so the "reaches the picker with the zero landing and `["claude"]` threaded" contract is asserted by the same test as before.
- Notes: Both additions are pure additions — `git show eae96c47c` shows no edit to an existing subtest in either test function, as the task required. No redundancy: the two rows assert different functions (the predicate and the router) and the second is not derivable from the first without also fixing `client != nil` and the latch. No mocking beyond the package's existing `openProbeCmd`/`probeClient` helpers.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()` (the `cmd` package forbids it); the new rows use the package's own `openProbeCmd()` / `probeClient()` constructors rather than hand-rolling a command or a live `tmux.Client`; `probeClient()` is backed by `commandertest.Quiet()`, so the added rows issue zero tmux round-trips and touch no real server.
- SOLID principles: Good. The change removes the split responsibility the task named — the function now asks "which positionals are this invocation's own?" once, through the one helper that owns the `--` rule, instead of half-answering it inline.
- Complexity: Low. One expression, two helper calls, no new branch.
- Modern idioms: Yes.
- Readability: Good. The two disjuncts are now symmetric and read alike, which is the point of the change.
- Issues: None. The doc comment above `isTUIPath` (`cmd/root.go:167-172`) still holds true of the new body — it describes the search arm's shape-not-outcome rule, which the change did not alter, and makes no claim about how the no-target arm reads its positionals.

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "`go test ./...` is green." — needs a suite run. Reading confirms the two added subtests compile against symbols that exist in the package (`openProbeCmd`, `probeClient`, `isTUIPath`, `shouldRunConcurrentBootstrap`) and that no other test in the tree asserts a contradicting verdict (`isTUIPath` / `shouldRunConcurrentBootstrap` are referenced from `cmd/concurrent_bootstrap_gate_test.go` alone), but greenness itself is not settleable by reading.
