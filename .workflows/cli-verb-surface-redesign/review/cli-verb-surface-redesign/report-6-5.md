TASK: cli-verb-surface-redesign-6-5 — Bare `portal` prints help/usage and does not launch the picker (control-plane root guard)

ACCEPTANCE CRITERIA:
- `rootCmd` has no `Run`/`RunE` so cobra returns `ErrHelp` before `PersistentPreRunE` — no bootstrap, no picker (verify).
- Bare `portal` exits without launching the TUI.
- `x` / `portal open` remain the only picker doors while `xctl` / bare `portal` are the management plane.
- Verification/guard-only — no production change expected (flag if the guard test already passes as-is).
(Phase 6 AC bullet: "Bare `portal` prints help/usage and does NOT launch the picker (control-plane root), leaving `x` / `portal open` as the only picker doors.")

STATUS: Complete

SPEC CONTEXT:
Spec § "Bare `portal` (no subcommand)" (specification.md:373-377): bare `portal` prints help/usage and does NOT launch the picker. The picker has exactly two doors (`portal open`, `x`). Bare `portal` is the control-plane root — making it open the picker would also make bare `xctl` open the picker (`xctl() { portal "$@" }`), muddying the deliberate two-tier split: `x` = launcher, `xctl`/`portal` = management plane. Back-compat § confirms `x`/`xctl` shell functions are untouched and keep working.

IMPLEMENTATION:
- Status: Implemented (guard-only, no production change — as the task expected).
- Location: cmd/root.go:162-289 (`rootCmd`); cmd/bare_root_test.go (new guard test file).
- Notes:
  - `rootCmd` declares only `PersistentPreRunE`, `SilenceUsage`, `SilenceErrors` — no `Run`/`RunE`. Verified no other site assigns `rootCmd.Run`/`rootCmd.RunE` (grep: only the test file references them).
  - Cobra v1.10.2 mechanism verified directly against the pinned source (go.mod: spf13/cobra v1.10.2): `Runnable()` = `c.Run != nil || c.RunE != nil` (command.go:1596); in `execute()` the guard `if !c.Runnable() { return flag.ErrHelp }` (command.go:955-956) fires BEFORE `c.preRun()` (959) and the `PersistentPreRunE` loop (984-986); `ExecuteC` maps `errors.Is(err, flag.ErrHelp)` → HelpFunc + returns nil (command.go:1152). So bare `portal` prints help and exits 0 with zero bootstrap and no picker — exactly the acceptance claim.
  - Guard-only confirmed: task commit c593bb4a changed only `cmd/bare_root_test.go` (+ workflow metadata). No `root.go` edit. This matches the task's "flag if the guard test already passes as-is" — the executor correctly documented this in the test's `VERIFICATION-AND-GUARD` comment block ("passes against the current tree with ZERO production change"). Expected outcome, not a defect.

TESTS:
- Status: Adequate.
- Coverage:
  - `TestBarePortalPrintsHelpAndDoesNotLaunchPicker` (behavioral, end-to-end Execute with empty args): (a) no error returned; (b) help/usage text emitted (`Usage:` or `Available Commands:`); (c) injected `recordingRunner` orchestrator Run count == 0 (no bootstrap); (d) overridden `openTUIFunc` never invoked (no picker). Covers all four AC facets.
  - `TestRootCmdIsNotRunnable` (structural invariant): `rootCmd.Run == nil`, `rootCmd.RunE == nil`, `rootCmd.Runnable() == false`.
- Notes:
  - The two tests are complementary, not redundant — one asserts observable behaviour through `Execute()`, the other pins the structural precondition on the command object. A future accidental `Run`/`RunE` is caught by both, in multiple modes: (c) fires if `PersistentPreRunE` runs and reaches `runBootstrap` (client is nil → not-satisfied → orchestrator invoked); (d) fires if a regressing `RunE` launched the picker; (a) fires if bootstrap errored. Robust regression net.
  - The `recordingRunner` injection and `openTUIFunc` override are load-bearing sentinels, not dead setup — they are the mechanism that detects a Run/RunE regression. Not over-tested.
  - Follows project conventions: file header documents package-level state mutation and the no-`t.Parallel` rule; uses established seams (`recordingRunner`, `openTUIFunc`, `resetRootCmd`).

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, package-state mutation documented, reuses shared test seams, cobra-in-cmd conventions).
- SOLID principles: Good — test asserts behaviour (help + no bootstrap + no picker) not internal wiring; structural test isolates the one invariant.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good — the mechanism comment cites exact cobra v1.10.2 source line numbers, all verified accurate (955, 956, 1152, 1596 exact).
- Issues: None material.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] cmd/bare_root_test.go:26 — the mechanism comment cites "command.go:983" for the PersistentPreRunE loop; in cobra v1.10.2 the `for _, p := range parents` loop is at command.go:984 (the `p.PersistentPreRunE != nil` check at 985). Every other citation in the block is line-exact; retarget 983 → 984 for consistency.
