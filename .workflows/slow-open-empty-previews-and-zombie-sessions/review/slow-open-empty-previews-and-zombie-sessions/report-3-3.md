TASK: 3-3 — Add post-respawn readiness barrier polling daemon.pid + state.IdentifyDaemon

STATUS: Complete

SPEC CONTEXT: Component F ¶4 — post-respawn readiness barrier polling daemon.pid + IdentifyDaemon at 50 ms cadence, bounded 2 s. On timeout WARN literal `"saver respawn: daemon did not come up within 2s"` under ComponentBootstrap and return nil. Best-effort.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/portal_saver.go:478-498` — `waitForSaverDaemonReady` main loop
  - `internal/tmux/portal_saver.go:505-515` — `isSaverDaemonReady` extracted single-tick predicate
  - `internal/tmux/portal_saver.go:152-155` — `SaverReadinessSeams` struct
  - `internal/tmux/portal_saver.go:253-256` — defaults 50 ms / 2 s
  - `internal/tmux/portal_saver.go:283` — wired into `saver.Ops.WaitForReady` at init
  - `internal/tmux/portal_saver.go:576-589` — called from BootstrapPortalSaver after RespawnPane, gated by `createdSession`
- Deadline computed once at entry (clock-skew safe)
- WARN sink shared with kill-barrier via `saver.Barrier.Logger`
- WARN message matches spec/plan grep anchor verbatim
- Seam names consolidated into `SaverSeams` (with ReadPID/IdentifyDaemon shared with kill barrier) — structural improvement over plan's literal seam names; per-field overridable via export_test.go accessors

TESTS:
- Status: Adequate
- Coverage: 12 unit tests in `portal_saver_test.go` covering success path, ErrPIDFileAbsent retry, transient ps error retry, IdentifyDead retry, recycled-PID, timeout with WARN literal, wall-clock cap, transient read error, single-deadline-at-entry, ordering [respawn-pane, readiness], no-readiness-on-happy-path, stateDir threading

CODE QUALITY:
- Project conventions: Followed; seam-struct, BarrierLogger reuse, swapSeam + t.Cleanup
- SOLID: Good; predicate extracted
- Complexity: Low; ~20-line loop
- Modern idioms: `time.NewTicker` + `defer Stop`
- Readability: Good

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan's literal seam names consolidated into `SaverSeams`. Future plans citing old names would need updating
- [idea] `waitForSaverDaemonReady` always returns nil — `error` return is purely defensive; could drop in future cleanup
