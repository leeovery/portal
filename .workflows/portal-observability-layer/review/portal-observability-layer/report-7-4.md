TASK: Remove the redundant daemon "starting" INFO line dropped by the spec (portal-observability-layer-7-4)

ACCEPTANCE CRITERIA:
- cmd/state_daemon.go no longer emits a daemon: starting (or equivalent logger.Info("starting")) line.
- Daemon-component INFO events limited to the spec's catalog (lock acquired, self-eject, shutdown).
- Daemon startup remains observable via process: start process_role=daemon + daemon: lock acquired.
- go build / go test ./cmd/... pass.

STATUS: Complete

SPEC CONTEXT:
Spec § Saver and daemon lifecycle taxonomy (867-921): closed catalog of three daemon-component INFO events (lock acquired, self-eject, shutdown). Process/subsystem boundary (900): redundant daemon: spawn dropped (same instant/data as process: start process_role=daemon), its unique tmux_pane attr moves onto lock acquired. Line 919: daemon startup marked by process: start, not a daemon: event. The old logger.Info("starting") was the surviving migration-bridge artifact.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/state_daemon.go:594-599 (logger.Info("starting") gone, replaced by explanatory comment citing spec). Daemon emits exactly the three: lock acquired (:270), self-eject (:298), shutdown (:547/552/563).
- Notes: tmux_pane relocated onto lock acquired (:270). version/pid ride as auto-baseline. Repo-wide grep "starting" in state_daemon.go shows only the explanatory comment (:597); other matches unrelated or test assertions verifying removal. No production path depends on the removed line.

TESTS:
- Status: Adequate
- Location: cmd/state_daemon_test.go
- Coverage: TestStateDaemon_DoesNotEmitStartingINFO (:209-237 — asserts daemon: starting NOT present AND daemon: lock acquired present); TestStateDaemon_OpensLogFileInStateDir (:183, lock acquired marks startup); TestStateDaemon_StartupLogIncludesVersionAndPID (:409, version + pid ride on lock acquired).
- Notes: Exercises real path (runStateDaemon → rootCmd.Execute() with ["state","daemon"]); withImmediateRun swaps only inner tick loop preserving acquire+pid ceremony + lock acquired emission. PORTAL_LOG_LEVEL=info set. Negative + positive assertions both load-bearing. Not over-tested.

CODE QUALITY:
- Project conventions: Followed (daemonLogger = log.For("daemon"); no t.Parallel).
- SOLID: Good — pure deletion of a redundant side-effect.
- Complexity: Low (net reduction).
- Modern idioms: Yes; in-source comment documents the deliberate omission with spec citation.
- Readability: Good — replacement comment explains startup observability + why starting dropped (prevents reintroduction).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
