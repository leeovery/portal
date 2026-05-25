TASK: 5-8 — Integration test legitimate first-tick self-check inside fresh _portal-saver

STATUS: Complete

SPEC CONTEXT: Component D "Skipped check on first tick is benign" — legitimate daemon ticking first time inside freshly-created `_portal-saver` passes self-check on tick 1 (pane pid matches its pid). Phase 3 + Phase 4 + Phase 1 enable end-to-end cold-start.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon_self_supervision_integration_test.go:960-1189` `TestSelfEject_LegitimateColdStartDoesNotFalsePositive`
- Supporting constants: 864-901 (`legitimateColdStartHysteresisMirror=3`, `legitimateColdStartObservationWindow=5s`, `legitimateColdStartLockAcquireBudget=1500ms`)
- Production wiring verified: `selfSupervisionHysteresisTicks = 3`; `tmux.BootstrapPortalSaver`; `portaltest.IsolateStateForTest`
- Invokes `tmux.BootstrapPortalSaver(client, stateDir)` directly (per Edge Case allowance), exercising full Phase 3 placeholder→destroy-unattached=off→respawn→readiness path

TESTS:
- Status: Adequate
- Observation window `(N+2)*TickerPeriod = 5s` exceeds hysteresis threshold (3)
- Structural binding (`daemon.pid == pane_pid`) asserted pre- and post-window
- Liveness via `state.IdentifyDaemon == IdentifyIsPortalDaemon` (stronger than file presence — catches stale-pidfile masquerade)
- Absence of self-supervision log marker gated on `PORTAL_LOG_LEVEL=INFO` propagation with belt-and-braces path check
- LIFO `t.Cleanup` ordering: `kill-session _portal-saver` before `kill-server`, allowing daemon's SIGHUP flush path
- `legitimateColdStartHysteresisMirror = 3` duplicates production const (no access to unexported); drift only widens headroom

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel`; `tmuxtest.SkipIfNoTmux`
- SOLID: Good; linear flow
- Complexity: Acceptable (~230 LoC) but linear with heavy inline rationale
- Modern idioms: `t.Setenv`, `errors.Is`, typed `state.IdentifyDaemon` result
- Readability: Good; every assertion cites spec bullet + regression class

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Mirror constant could be replaced by `export_test.go` re-export so drift impossible by construction
- [idea] Assertion D depends on `PORTAL_LOG_LEVEL=INFO` propagating test process → tmux server → respawn-pane'd daemon; fragile chain; assert via positive log marker that log level is actually INFO
- [quickfix] Plan AC names `portaltest.NewIsolatedStateEnv`; codebase uses `IsolateStateForTest`
