TASK: 2-5 — Wire daemon call site to pass real ComponentDaemon logger and assert log delivery

STATUS: Complete

SPEC CONTEXT: Component E line 338 — every per-session skip emits WARN with session name + error under existing `ComponentDaemon` constant; new constant NOT introduced. Daemon's logger handle is `daemonDeps.Logger`.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - Call site: `cmd/state_daemon.go:327` — `state.CaptureStructure(deps.Client, skipSet, deps.PrevIndex, deps.Logger)`
  - Tests: `cmd/state_daemon_capture_logging_test.go` (244 lines, new file)
- End-to-end `tick → captureAndCommit → CaptureStructure → logger.Warn` verified

TESTS:
- Status: Adequate
- Coverage:
  - `TestDaemonTick_LogsAnomalousShowEnvironmentFailureUnderComponentDaemon` — asserts WARN, ComponentDaemon, failing session name, sentinel `bravo-boom-sentinel`
  - `TestDaemonTick_LogsPerSessionWarnAndCommitsEmptyOnAllNaturalChurn` — strict equality `warnCount == 2` defends against tick-wrapper leak; both session names present; sessions.json committed with zero sessions
- Isolation via `t.Setenv("PORTAL_STATE_DIR", dir)`
- Unique sentinel prevents cross-test substring collisions

CODE QUALITY:
- Project conventions: Followed; no `t.Parallel()`, file-level comment notes convention; `envFailingCommander` wraps rather than mutates fake
- SOLID: Good; composition over modification
- Complexity: Low; linear AAA
- Modern idioms: `t.TempDir`, `t.Setenv`, `t.Cleanup`, idempotent `logger.Close`
- Readability: Good; docstrings map to spec
- Minor: double-close of logger intentional but undocumented; ~60% setup duplication between two tests

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Add a "happy path emits no spurious WARN" test for regression safety
- [idea] Extract `makeLoggingTickHarness(t, envErrs)` if Component E tests grow beyond two callsites
- [quickfix] One-line comment for intentional double-close in `readPortalLog`
- [quickfix] Update file-level comment to cite live call-site line `:327` (planning doc's `:149` is stale)
