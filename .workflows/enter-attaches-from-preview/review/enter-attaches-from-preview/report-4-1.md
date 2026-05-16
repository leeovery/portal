TASK: enter-attaches-from-preview-4-1 — Close previewLogger to remove lifecycle asymmetry

ACCEPTANCE CRITERIA:
- `cmd/open.go` contains `defer previewLogger.Close()` directly after the `state.OpenLogger` assignment in `openTUI`
- No other call sites or signatures change
- Build passes; existing `cmd` tests pass unchanged

STATUS: Complete

SPEC CONTEXT: Cycle-2 LOW finding: previewLogger was the only resource opened in `openTUI` without a matching deferred Close. Fd reclamation today happens via `syscall.Exec` or process exit, so no observable leak — but repeated `openTUI` invocations would leak fds, and the asymmetry forces future maintainers to re-derive the safety argument. Logger.Close is documented nil-safe (internal/state/logger.go:214-219).

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/open.go:432 — `defer previewLogger.Close()` placed immediately after the `state.OpenLogger` assignment on line 431
- Notes: Defer is positioned BEFORE the err-check on lines 433-435 that nils out `previewLogger`. This is correct: `previewLogger.Close` evaluates the receiver at defer-statement time, capturing the original pointer from OpenLogger. If OpenLogger returned a non-nil logger plus an err, the original file is still closed; if it returned nil, Close is nil-safe per internal/state/logger.go:214-219. Already flagged as a readability quickfix in report-1-5; not re-raised here.

TESTS:
- Status: Adequate (no new test required per task spec)
- Coverage: Logger.Close nil-safety is covered by existing internal/state/logger_test.go.
- Notes: None.

CODE QUALITY:
- Project conventions: Followed.
- SOLID: N/A.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Acceptable; defer-before-err-check captured in report-1-5.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES: None.
