TASK: 5-3 — Integrate per-tick self-check before captureAndCommit with os.Exit(0) eject

STATUS: Complete

SPEC CONTEXT: Component D — per-tick saver-membership probe, hysteresis N=3, INFO log + `os.Exit(0)` without final flush, intentional stale `daemon.pid` handled by C. Planning broadened "before captureAndCommit" to "before `IsRestoringSet`" so orphans can't gain restore-window immunity.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon.go:233-259` (`defaultDaemonTickLoop`); seams at `:67` (`osExit`), `:80` (`saverMembershipProbe`); constant at `:149`
- Self-check in `defaultDaemonTickLoop` (extracted from `defaultDaemonRun`) inside ticker for-loop, `case <-ticker.C:` arm BEFORE `tick(ctx, deps)`
- Counter closure-scoped (`var consecutiveAbsenceTicks int`); resets to 0 on probe-true (243), increments on probe-false (245)
- Triggers eject at `>= selfSupervisionHysteresisTicks` (246); INFO under `ComponentDaemon` with `"self-supervision: saver-membership lost for %d consecutive ticks, exiting"`
- `osExit(0)` via package-level seam (250); explicit `return nil` after (251) defends test fakes
- `daemon.pid` NOT deleted; no `os.Remove`; no new defer
- `state.Logger.Info` already existed
- Detailed doc block at `:166-192` documents spec ordering, why-not-inside-tick, why `os.Exit`, why `daemon.pid` retained

TESTS:
- Status: Adequate
- Coverage in `cmd/state_daemon_self_supervision_test.go`:
  - `TestDaemonLoop_SelfCheckBypassesShutdownOnEject` (75) — osExit fires once code 0, daemonShutdownFunc never invoked
  - `TestDaemonLoop_SelfCheckSkipsCaptureOnEjectTick` (139) — eject short-circuits before `tick()`; list-sessions bounded
  - `TestDaemonLoop_SelfCheckRunsBeforeIsRestoringSet` (176) — eject fires even with `@portal-restoring` set
  - `TestDaemonLoop_SelfCheckDoesNotDeleteDaemonPID` (218)
  - `TestDaemonLoop_SelfCheckResetsCounterOnProbeTrue/EjectsExactlyOnNthFalse/ResetOnEachTrue/LogsInfoOnEject`
  - `TestSelfSupervisionHysteresisTicks_LowerBound` (711)
- Integration coverage in `cmd/state_daemon_self_supervision_integration_test.go` for "self-eject on absent saver"
- Seams via `t.Cleanup`; no `t.Parallel`; fast `TickerPeriod=1ms`; panic-on-osExit pattern for unwinding

CODE QUALITY:
- Project conventions: Followed
- SOLID: Good; sub-seam `daemonTickLoopFunc` allows acquire+pid ceremony and tick loop test independently
- Complexity: Low; self-check arm ~10 lines
- Modern idioms: closure-scoped counter, function seams, `errors.Is`
- Readability: Excellent; doc block cites spec, planning resolution, rationale

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] `state_daemon.go:251` has `return nil` after `osExit(0)`; unreachable in production; one-line "unreachable; defends test seams" comment would clarify
- [quickfix] Plan's "if `state.Logger.Info` doesn't exist, add it" branch was no-op; plan text now stale
