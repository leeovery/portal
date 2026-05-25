TASK: 6-7 — Assert Component F end-state observables on _portal-saver (composite end-to-end)

STATUS: Complete

SPEC CONTEXT: Spec § Composite bullet 9 — after full A+B+C+D+E+F, pane process is `portal state daemon` AND `destroy-unattached == off`. Phase 3 task 3-5 verified isolation; 6-7 verifies post-A+B convergence.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/composition_e2e_f_observables_integration_test.go` `TestCompositeBootstrap_FObservables` (line 71)
- Calls `setupCompositeHarness(t)` (3-daemon pre-state)
- Bootstrap slice SweepOrphanDaemons → BootstrapPortalSaver in orchestrator order
- `fObservablesConvergenceTimeout = 6s` (65); REMAINING budget at poll site enforces "within 6s of bootstrap entry"
- Observable 1 (pane_pid): `list-panes -t _portal-saver -F '#{pane_pid}'` via `sock.TryRun`; rejects empty/multi-line/non-integer/≤0 (119-146)
- Observable 2 (process args): inlined `psArgsForPIDInline` (222) byte-equivalent to `psArgsForPID` in `internal/tmux/portal_saver_endstate_integration_test.go:612`. Asserts `Contains(args, "portal state daemon")` AND `!Contains(args, "tail -f /dev/null")` — negative assertion catches F regression where placeholder→respawn ordering never fires
- Observable 3 (destroy-unattached): prefix-strip + `tmuxout.StripMatchedOuterQuotes` (206), tolerating quoted and unquoted forms
- 3×50ms retry ladder mentioned in plan NOT implemented — test fails fast; acceptable since pgrep == 1 convergence barrier proves steady-state

TESTS:
- Status: Adequate
- Single `TestCompositeBootstrap_FObservables` exercising all three observables; four conceptual tests collapsed
- Rich diagnostic enumerating setup-time PIDs, alive status, pgrep snapshot

CODE QUALITY:
- Project conventions: Followed; `//go:build integration`; no `t.Parallel`; `sock.TryRun`
- SOLID: Good
- Complexity: Low; linear
- Modern idioms: Good
- Readability: Excellent; file-header documents three observables, parsing rationale, relationship to 3-5

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `psArgsForPIDInline` (222) duplicated byte-for-byte with `psArgsForPID` in `internal/tmux/portal_saver_endstate_integration_test.go:612`; deliberate ~3-line duplication; if more sites grow, promote to `internal/portaltest`
- [idea] 3×50ms retry ladder not implemented; if integration-CI flake appears, retry ladder is documented mitigation
- [quickfix] File-header at line 16 says "AFTER the composite A+B+F bootstrap slice converges"; full composition per spec is A+B+C+D+E+F (header lines 31-39 already clarify scope)
