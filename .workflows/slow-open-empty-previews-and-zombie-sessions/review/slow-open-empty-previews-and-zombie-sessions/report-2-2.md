TASK: 2-2 — Thread *state.Logger parameter into CaptureStructure (no behaviour change)

STATUS: Complete

SPEC CONTEXT: Component E — `CaptureStructure` gains `*state.Logger` for per-session WARN emission. Task 2-2 is signature plumb sequenced before 2-3.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/capture.go:86` — `func CaptureStructure(c CaptureClient, skipSet map[string]struct{}, prev *Index, logger *Logger) (Index, error)`
- Godoc lines 66-85 documents param, nil-no-op, ComponentDaemon WARN, natural-churn discriminator
- Production call site `cmd/state_daemon.go:327` passes `deps.Logger`
- Test call sites updated everywhere (`capture_test.go` ~45 sites, `internal/restore/integration_test.go`, etc.); `cmd/state_commit_now.go:208` consumes `deps.CaptureStructure(client, nil, &prev, logger)` consistent with new arity

TESTS:
- Status: Adequate (no own-test addition required; regression backstop via compiling/passing existing tests under new arity)

CODE QUALITY:
- Project conventions: Followed; mirrors `Commit`'s `*state.Logger` arg
- SOLID/Complexity: Good; nil-tolerant DI
- Modern idioms: errors.Join + %w
- Readability: Good; thorough forward-looking godoc

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] Plan separated 2-2/2-3 explicitly; current capture.go shows both landed together rather than as discrete commits
- [quickfix] Plan cites `cmd/state_daemon.go:149`; actual is `:327` (line drift)
