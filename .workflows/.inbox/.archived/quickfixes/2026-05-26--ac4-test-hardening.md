# AC4 test hardening — drift-mirror comment + missing negative control

Two test-quality improvements to `TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4` at `cmd/bootstrap/eager_signal_hydrate_integration_test.go:260-400`. Both have small surfaces and no open design choices — promoted from `2026-05-11--ac4-test-hardening` idea on 2026-05-26 with the design picks baked in.

## Atom 1 — drift-mirror breadcrumb in `captureAndCommit`

The integration test exercises `runDaemonTick` (`cmd/bootstrap/daemon_tick_test_helpers_test.go:86-149`), which mirrors the production `captureAndCommit` body at `cmd/state_daemon.go:115-158` byte-for-byte under the integration build tag. The mirror is defensible (production lives in `cmd`, not `state` — exposing a public API for a transient test seam would be overkill) but it creates real drift risk: any future change to production `captureAndCommit` (e.g., a new pre-write filter) must be mirrored in the test helper or AC4 could pass under a broken production tick.

Fix: add a one-line comment in `cmd/state_daemon.go` `captureAndCommit` pointing to `runDaemonTick` in `cmd/bootstrap/daemon_tick_test_helpers_test.go` as a drift-mirror site. Comment text along the lines of:

```
// Drift-mirror: cmd/bootstrap/daemon_tick_test_helpers_test.go runDaemonTick
// shadows this body byte-for-byte under the integration build tag for AC4
// coverage. Mirror any structural change here in that helper or AC4 may pass
// under a broken production tick.
```

Mechanical, zero behaviour change. The alternative considered (extract a shared `state.RunCaptureOnce`) is rejected here as premature — it should only happen if drift actually becomes observable.

## Atom 2 — inline negative-control sub-test

`TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4` currently relies on a docstring-level mapping of regression failure modes to its assertions. Add an inline negative-control sub-test that wires `bootstrap.NoOpEagerHydrateSignaler{}` instead of the production signaler and asserts that beta's scrollback file is absent. Symmetric with the existing `TestScrollbackResumption_WithoutCleanupScrollbackNotSaved` shape.

The cost is additional tmux-fixture wall time for one extra sub-test. The benefit is an active assertion replacing a docstring invariant — exactly the guard needed to catch a silent break of the eager-signal seam in future work.

## Scope

Both atoms touch:
- `cmd/state_daemon.go` (atom 1 — one comment line)
- `cmd/bootstrap/eager_signal_hydrate_integration_test.go` (atom 2 — one new sub-test)

No changes to production logic. No new exports. No new test helpers. Build + vet clean.

Source: `2026-05-11--ac4-test-hardening` idea (archived 2026-05-26 as folded into this quickfix). Originating context: review of `killed-sessions-resurrect-on-restart`.
