TASK: 9-7 — Fix Component C log-acquire error level (Error to Warn per spec)

STATUS: **Issues Found — BLOCKING (test failure confirmed)**

SPEC CONTEXT: Spec § Component C step 4 mandates "log WARN under ComponentDaemon and exit with status 1" for non-EWOULDBLOCK lock acquisition failures. Prior code emitted Logger.Error, contradicting spec.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/state_daemon.go:213`
- Production correctly uses `deps.Logger.Warn(state.ComponentDaemon, "acquire daemon lock: %v", err)`
- Wrapped-error return on line 215 (`fmt.Errorf("acquire daemon lock: %w", err)`) preserved → exit status 1 maintained
- Sibling ErrDaemonLockHeld branch (line 208) was already WARN
- Commit `c2e2d4c9` carries the change

TESTS:
- Status: **BROKEN** — confirmed by `go test ./cmd -run TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` returning FAIL
- `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` at `cmd/state_daemon_test.go:591-650` was NOT updated:
  1. Line 594 sets `PORTAL_LOG_LEVEL=error` with stale comment (lines 592-593)
  2. Line 633 asserts `strings.Contains(got, "ERROR")` — FAILS post-change
  3. Lines 640-649 assert exactly one line containing both "ERROR" and "acquire daemon lock" — FAILS
  4. Comment block at lines 624-627 still says "ERROR-level log line"
- Per `internal/state/logger.go` write filter (line 247-253), pinning minLevel to LevelError filters out LevelWarn writes; portal.log will not contain "ERROR" substring
- Acceptance criterion "log-level-asserting tests updated" was missed

CODE QUALITY:
- Project conventions: Followed (production side)
- SOLID: Good
- Complexity: Low (unchanged)
- Modern idioms: Yes
- Production side clean; test-side comments factually wrong

BLOCKING ISSUES:
- **`cmd/state_daemon_test.go:591` `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` will fail** (verified by `go test` run). The plan's acceptance criteria explicitly required test updates and they are absent. This is a shipped regression that breaks the test suite.

NON-BLOCKING NOTES:
- [quickfix] Rename `TestStateDaemon_ReturnsErrorOnNonContentionLockFailure` → `TestStateDaemon_ReturnsErrorAndLogsWarnOnNonContentionLockFailure`
- [quickfix] Update comments at 592-593 and 624-627 to describe WARN-level mandate
- [idea] Add table-driven test asserting exact log level for both lock-acquire branches (ErrDaemonLockHeld + non-contention) — pins both spec mandates symmetrically
