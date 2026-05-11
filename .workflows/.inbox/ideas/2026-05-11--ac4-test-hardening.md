# AC4 test hardening — drift risk + missing negative control

Two related improvements to `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4` at `cmd/bootstrap/eager_signal_hydrate_integration_test.go:260-400`.

1. **Production-helper drift risk.** The plan's edge case "expose `state.RunCaptureOnce` as a test seam if not present" was not taken. Instead `runDaemonTick` (`cmd/bootstrap/daemon_tick_test_helpers_test.go:86-149`) mirrors the production `captureAndCommit` body at `cmd/state_daemon.go:115-158` byte-for-byte under the integration build tag. Defensible — production lives in `cmd` not `state`; exposing a public API for a transient test seam is overkill — but the byte-for-byte mirror creates drift risk: any future change to production `captureAndCommit` (e.g., a new pre-write filter) must be mirrored in the test helper or AC4 could pass under a broken production tick. Options: (a) extract a shared `state.RunCaptureOnce` later if drift becomes observable; (b) add a one-line comment in `cmd/state_daemon.go captureAndCommit` pointing to the helper as a drift-mirror site.

2. **Missing negative control.** Consider adding an inline negative-control sub-test that wires `bootstrap.NoOpEagerHydrateSignaler{}` and asserts beta's scrollback file is absent — symmetric with `TestScrollbackResumption_WithoutCleanupScrollbackNotSaved`. Current implementation relies on a docstring-level regression-failure-mode mapping; an active negative-control would be a sharper guard at the cost of additional tmux-fixture wall time.

Source: review of killed-sessions-resurrect-on-restart/killed-sessions-resurrect-on-restart
