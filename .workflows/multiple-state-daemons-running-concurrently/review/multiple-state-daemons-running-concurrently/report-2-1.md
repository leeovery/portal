# Review Report — Task 2.1

TASK: Add seam-injectable killSaverAndWaitForDaemon helper

STATUS: Complete
FINDINGS_COUNT: 0

ACCEPTANCE CRITERIA:
- Helper signature `killSaverAndWaitForDaemon(c *Client, stateDir string) error` exists, unexported, in `portal_saver.go`.
- Four seam `var`s exist with documented defaults.
- WARN logger seam exists; tests record exactly one WARN on timeout and zero on every other branch.
- Prior-PID-dies-within-timeout → nil, zero WARN.
- Prior-PID-never-dies → nil after timeout, exactly one WARN, wall time bounded.
- ReadPIDFile error (absent / parse / generic) → tolerant kill, no polling, no WARN.
- IsProcessAlive(priorPID)==false at first check → tolerant kill, no polling, no WARN.
- Helper issues `c.KillSession(PortalSaverName)` exactly once per invocation.
- Helper does not write to the state directory.
- All seams reset via `t.Cleanup`.
- `go build ./...` and `go test ./internal/tmux/...` pass; no `t.Parallel()`.

SPEC CONTEXT:
Spec §"Fix Part 2: Synchronous Kill Barrier" — helper makes common-case kill+respawn quiet so the singleton lock (Part 1) does not fire its WARN on every recycle. Reads prior PID, kills, polls `IsProcessAlive` at 50–100 ms cadence, bounded by 5 s timeout sized above 3.9 s cold-sweep ceiling. Timeout is non-fatal: returns silently on happy path, emits one WARN on timeout.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/tmux/portal_saver.go:47-186` (seams + interface + helper); `internal/tmux/export_test.go:1-35` (test-only re-exports).
- Notes:
  - Helper signature matches spec (line 150).
  - Four seams declared with correct defaults: `killBarrierReadPID = state.ReadPIDFile` (52), `killBarrierIsAlive = state.IsProcessAlive` (58), `killBarrierPollInterval = 50 * time.Millisecond` (64), `killBarrierTimeout = 5 * time.Second` (71).
  - Logger seam: `BarrierLogger` interface (81-83), `noopBarrierLogger` (88-90), `killBarrierLogger BarrierLogger = noopBarrierLogger{}` (97), and exported `SetBarrierLogger` setter (108-113) that ignores nil.
  - Control-flow matches spec steps 1–4:
    1. `killBarrierReadPID` error → tolerant kill, return nil immediately (151-156).
    2. `killBarrierIsAlive` false on first probe → tolerant kill, return nil immediately (158-162).
    3. Tolerant kill issued exactly once (165), then ticker-driven re-probe loop with deadline (167-184).
    4. Timeout path emits one WARN via `killBarrierLogger.Warn(state.ComponentBootstrap, ...)` and returns nil (175-183).
  - Kill issued exactly once per invocation across every branch.
  - No state-directory writes.

TESTS:
- Status: Adequate
- Coverage (all in `internal/tmux/portal_saver_test.go`):
  - Prior PID dies within timeout → `TestKillSaverAndWaitForDaemon_ReturnsNilWithNoWarnWhenPriorPIDDiesBeforeTimeout` (1004-1037).
  - Prior PID never dies → `TestKillSaverAndWaitForDaemon_EmitsOneWarnAndReturnsNilWhenPriorPIDNeverDies` (1039-1069).
  - PID file absent → `TestKillSaverAndWaitForDaemon_SkipsPollingWhenPIDFileAbsent` (1071-1103).
  - PID file corrupted → `TestKillSaverAndWaitForDaemon_SkipsPollingWhenPIDFileCorrupted` (1105-1139).
  - PID file unreadable → `TestKillSaverAndWaitForDaemon_SkipsPollingWhenPIDFileUnreadable` (1141-1175).
  - Prior PID already dead → `TestKillSaverAndWaitForDaemon_SkipsPollingWhenPriorPIDAlreadyDead` (1177-1209).
  - Tolerates failing KillSession → `TestKillSaverAndWaitForDaemon_ToleratesFailingKillSession` (1211-1237).
  - Does not mutate state directory → `TestKillSaverAndWaitForDaemon_DoesNotMutateStateDirectory` (1239-1278).
- Notes:
  - Seam helpers follow existing `stubAliveCheck` / `shrinkRetryDelay` precedent with `t.Cleanup`.
  - `recordingBarrierLogger` records `component | format` — asserts presence/count without coupling to literal text.
  - No `t.Parallel()`.
  - Degenerate config edge case (poll == timeout) lacks a dedicated test but is implicitly covered.

CODE QUALITY:
- Project conventions: Followed. Seam pattern mirrors `BootstrapAliveCheck` / `PortalSaverRetryDelay`.
- SOLID: Good. Minimal one-method `BarrierLogger` interface structurally satisfied by `*state.Logger`.
- Complexity: Low. Flat function, three early-return branches, one bounded loop.
- Modern idioms: `time.NewTicker` + `defer ticker.Stop()`; package-level `var` seams.
- Readability: Good. Doc comments enumerate the 4-step flow.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Add a dedicated test for the degenerate-config edge case `killBarrierPollInterval == killBarrierTimeout` (task Edge Cases bullet 4).
- [idea] The never-dies test's upper-bound assertion `elapsed < 1*time.Second` (with timeout=20ms) is loose. Tightening to e.g. `< 200*time.Millisecond` would catch regressions.
- [idea] `SetBarrierLogger` and `killSaverAndWaitForDaemonFn` indirection are Task 2.2 infrastructure that landed alongside Task 2.1. Benign — Task 2.1's "do not wire" rule applies to call-site replacements, not support scaffolding.
