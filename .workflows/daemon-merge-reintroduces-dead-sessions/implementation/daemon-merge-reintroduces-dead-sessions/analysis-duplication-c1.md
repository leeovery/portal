AGENT: duplication
STATUS: findings
FINDINGS_COUNT: 3

FINDINGS:

- FINDING: Two near-identical "daemon tick" helpers across cmd/bootstrap/ test files
  SEVERITY: medium
  FILES: cmd/bootstrap/reboot_roundtrip_test.go:533-569 (captureAndCommit), cmd/bootstrap/scrollback_resumption_test.go:421-465 (runDaemonTick)
  DESCRIPTION: Both files contain a t.Helper that drives a daemon-tick-equivalent capture-and-commit: ListSkeletonMarkers -> CaptureStructure -> walk sessions/windows/panes building a per-pane key, write scrollback, then state.Commit. They live in the same package (`package bootstrap_test`, both behind `//go:build integration`) so they share the build/test target. The only material differences are: (1) `runDaemonTick` honours a per-pane skipSet guard and calls CaptureAndHashPane (production-shape capture), while (2) `captureAndCommit` always writes empty bytes. Both assert/fatal on the same calls. The duplicated walk + Commit body is ~25 lines; it has been independently authored in two tasks and is at risk of silent drift (e.g. if state.Commit grows a new arg).
  RECOMMENDATION: Consolidate into one helper in a shared `bootstrap_test_helpers_test.go` (or extend the existing `runDaemonTick`) that takes optional knobs — a `skipGuard bool` and either `captureFn func(client, target) ([]byte, uint64)` or a `useEmptyScrollback bool` switch. Keep it in `cmd/bootstrap/` package-internal so both files import the same definition.

- FINDING: Bootstrap orchestrator construction is rebuilt inline across eleven integration test sites
  SEVERITY: medium
  FILES: cmd/bootstrap/scrollback_resumption_test.go:114-127, 228-238, 333-346; cmd/bootstrap/reboot_roundtrip_test.go:341-354, 997-1010, 1258-1271; cmd/bootstrap/phase5_integration_test.go:69-78, 211-220, 329-342; cmd/bootstrap/phase5_marker_suppression_integration_test.go:154-163; cmd/reattach_integration_test.go:167-176
  DESCRIPTION: Eleven sites build a `bootstrap.Orchestrator{...}` literal that wires the same eight-step shape: real RestoringMarker, NoOp/real Saver, real/NoOp Restore via RestoreAdapter, NoOp or production StaleMarkers, NoOp or production Sweeper, NoOp Hooks/CleanStale. cmd/reattach_integration_test.go has already extracted `buildReattachOrchestrator` for its own seven call sites; cmd/bootstrap/scrollback_resumption_test.go has three near-identical literals where only the `StaleMarkers` field flips between production adapter and `NoOpMarkerCleaner{}`. Each new step interface added to `bootstrap.Orchestrator` (as `StaleMarkers` was in this work unit) requires touching every one of these literals — a real maintenance hazard.
  RECOMMENDATION: Promote a single test-only orchestrator builder to a shared file in `cmd/bootstrap/` modeled on cmd/reattach_integration_test.go's `buildReattachOrchestrator`. Take an `orchestratorOpts` struct with optional fields defaulting each to its NoOp form when unset.

- FINDING: Integration-test "stateDir + PORTAL_STATE_DIR + EnsureDir + OpenLogger" preamble repeated across nine sites
  SEVERITY: low
  FILES: cmd/bootstrap/scrollback_resumption_test.go:62-66 + 108-112; 192-196 + 217-221; 289-293 + 327-331; cmd/bootstrap/reboot_roundtrip_test.go:181-185 + 330-334; 942-946 + 986-990; 1180-1184 + 1242-1246; cmd/bootstrap/phase5_integration_test.go:155-159 + 199-203; 266-270 + 317-321; cmd/bootstrap/phase5_marker_suppression_integration_test.go:78-82 + 142-146
  DESCRIPTION: Each integration test opens a state dir, exports it via `t.Setenv("PORTAL_STATE_DIR", stateDir)`, calls `state.EnsureDir()`, then later opens a non-rotating portal.log logger. The boilerplate is ~9 lines per test. cmd/reattach_integration_test.go has already partially extracted this via `setupReattachEnv`, but cmd/bootstrap/'s integration files (added across multiple tasks in this work unit — scrollback_resumption_test.go alone has three copies) each re-paste it.
  RECOMMENDATION: Add a `newIntegrationStateDir(t)` and `newIntegrationLogger(t, stateDir)` (or a single combined helper) to a new shared `cmd/bootstrap/integration_helpers_test.go` (gated by `//go:build integration`).

SUMMARY: Three duplications cluster around bootstrap integration test scaffolding — two near-identical daemon-tick helpers, eleven inline Orchestrator literals, and nine copies of the stateDir/logger preamble. The implementation code itself (capture.go's mergeSkippedPanes, stale_marker_cleanup.go's StaleMarkerCleaner, the bootstrapadapter wiring) is well-factored and shows no significant cross-file duplication.
