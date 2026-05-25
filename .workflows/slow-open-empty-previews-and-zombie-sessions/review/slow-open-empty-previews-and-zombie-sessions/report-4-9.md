TASK: 4-9 — Integration test for Component C upgrade-path two-binary scenario

STATUS: Complete

SPEC CONTEXT: Spec line 231 — integration test simulating v(N) → v(N+1) upgrade landmine; new daemon either acquires cleanly (A/B swept) or refuses via Component C pre-check.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/bootstrap/upgrade_path_integration_test.go` (433 lines)
- Three tests:
  - `TestUpgradePath_TwoBinary_AllComponentsCompose` (134) — Scenario A; spawns v(N) directly, drives v(N+1) bootstrap via `bootstrapadapter.NewOrphanSweeper.SweepOrphanDaemons` + `tmux.BootstrapPortalSaver`; asserts convergence within 6s, survivor PID ≠ v(N), `daemon.pid` references survivor, fresh `AcquireDaemonLock` returns `ErrDaemonLockHeld`
  - `TestUpgradePath_ComponentC_IsolatedRefusesCleanly` (262) — Scenario B; spawns v(N), waits for daemon.pid + IdentifyDaemon convergence, calls `AcquireDaemonLock` from test goroutine, asserts ErrDaemonLockHeld, then 200ms sleep + v(N) still alive
  - `TestUpgradePath_PostBootstrap_FreshAcquireDaemonLockRefuses` (337) — steady-state singleton
- File-header (47-58) explains saver adapter is cmd-private (import cycle if reconstructed); invokes the two load-bearing orchestrator steps directly — faithful to v(N+1) bootstrap

TESTS:
- Status: Adequate
- All three plan-named test cases as distinct top-level functions
- Edge cases covered (file-header 60-72): v(N) exits between bootstrap entry and AcquireDaemonLock; stale daemon.pid documented as unit-tested elsewhere
- pgrep skip via `skipIfNoPgrep` in every test
- `waitForDaemonPID` waits for expected PID (not just any), guards against stale daemon.pid bleed
- `portaltest.RegisterSubprocessCleanup` invoked for every spawned v(N); saver-pane cleaned via `tmuxtest.New`
- `fd.Close()` defensive on AcquireDaemonLock error-path return

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; `IsolateStateForTest`; `//go:build integration`
- SOLID: Good; helpers single-purpose; shared helpers reused from `orphan_sweep_integration_test.go`
- Complexity: Low; linear AAA
- Modern idioms: `errors.Is`, `t.Helper()`, `t.Setenv`, explicit tests over table
- Readability: Excellent; 80-line file-header documents scenarios, rationale, edge cases

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [quickfix] Plan text references `portaltest.NewIsolatedStateEnv` / `portalbintest.BuildPortalBinary`; production helpers are `IsolateStateForTest` / `StagePortalBinary`. Implementation uses correct current names; plan slightly outdated
- [idea] Scenario A could tag v(N)/v(N+1) daemons via `PORTAL_DAEMON_TAG` env var for easier convergence-failure diagnostics; low value
- [idea] One-line cross-reference to unit-test location for the "stale daemon.pid" branch would aid traceability
