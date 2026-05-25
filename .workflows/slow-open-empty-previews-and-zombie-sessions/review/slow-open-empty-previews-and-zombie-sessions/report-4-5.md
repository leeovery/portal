TASK: 4-5 — Integration test for SweepOrphanDaemons (3 daemons converge to 1, clean state sends zero signals)

STATUS: Complete

SPEC CONTEXT: Component B acceptance bullets 1-3 — N daemons (N-1 orphans), sweep kills N-1; on clean state zero signals + zero "sweep: killed orphan daemon" entries; identity-check refusal of recycled PIDs.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/orphan_sweep_integration_test.go` (433 lines, 3 scenarios + 4 helpers)
- Supporting infra: `internal/portaltest/spawn_daemon.go` (`SpawnIsolatedDaemon` + `RegisterSubprocessCleanup`); `internal/portaltest/pgrep.go` (`PgrepPortalDaemons` thin forwarder to `state.PgrepPortalDaemons`); `IsolateStateForTest`; `RecordingLogger`
- Scenario A's "each orphan gets its own stateDir" motivated by inline comment block: with three daemons against single stateDir, Component C pre-acquire converges to 1 before pgrep can witness 3

TESTS:
- Status: Adequate
- Scenario A (`ThreeDaemonsConvergeToOne`): real saver bootstrap, two real orphan daemons, pgrep == 3 precondition barrier, post-sweep pgrep == 1 + survivor identity assertion
- Scenario B (`CleanStateZeroSignals`): saver only, recording logger via type-assertion, forbidden-substring scan
- Scenario C (`RecycledPIDRefusal`): non-daemon `sleep 30`, custom Pgrep injects sleeper PID, real `state.IdentifyDaemon`, 200ms settle window + `kill(pid, 0)` ESRCH
- All scenarios faithfully exercise production wiring via `bootstrapadapter.NewOrphanSweeper`
- Failure diagnostics exemplary; `skipIfNoPgrep` + `tmuxtest.SkipIfNoTmux` cover CI-without-procps edge

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; `portaltest.IsolateStateForTest`
- SOLID: Good; production seams overridden via struct-field DI
- Complexity: Low; three linear scenarios
- Modern idioms: `errors.Is(killErr, syscall.ESRCH)`; compile-time interface guard
- Readability: Excellent; file-level docstring documents scenarios + isolation rationale

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] Scenario C preamble docstring says "override Pgrep to inject sleep PID into candidate set" but actually injects `{saverPID, sleeperPID}`
- [idea] `pgrepConvergenceTimeout = 3 * time.Second` duplicated locally; could be lifted to portaltest if CI proves flaky
