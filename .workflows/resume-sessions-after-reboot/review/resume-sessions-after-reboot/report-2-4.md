TASK: Hook Execution in TUI Selection Path

ACCEPTANCE CRITERIA:
- Selected session triggers hook execution before `connector.Connect`
- Hook executor receives selected session name
- Quitting TUI without selection does not call hook executor
- Existing `TestProcessTUIResult` tests pass
- All tests pass: `go test ./cmd -run TestProcessTUIResult`

STATUS: Complete

SPEC CONTEXT: The spec's "Execution Mechanics" section requires hook execution to be inserted **before** session connection in all three connection paths, including TUI picker selection (`processTUIResult`/`openTUI`). The insertion point must be before `connector.Connect` because `AttachConnector` uses `syscall.Exec` which replaces the process. Hook execution should be automatic (no confirmation prompt) and scoped to the target session's panes.

IMPLEMENTATION:
- Status: Implemented
- Location: `cmd/open.go:337-346` (`processTUIResult` function), `cmd/open.go:403-405` (`openTUI` caller that builds and passes `hookExec`)
- Notes: The `processTUIResult` function now accepts a `HookExecutorFunc` parameter. It calls `hookExec(selected)` after confirming a non-empty selection and before `connector.Connect(selected)`. The `openTUI` function at line 404 calls `buildHookExecutor(client)` and passes the result to `processTUIResult` at line 405. The nil-guard (`if hookExec != nil`) allows existing tests to pass `nil` for the hook parameter. No drift from the plan.

TESTS:
- Status: Adequate
- Coverage:
  - "clean exit without selection returns nil" (line 1156) - passes `nil` hookExec, verifies connector not called
  - "selected session name forwarded to connector" (line 1171) - passes `nil` hookExec, verifies existing behavior preserved
  - "selected session triggers hook execution before connect" (line 1194) - uses `orderTrackingConnector` to verify hooks run before connect
  - "hook executor receives selected session name" (line 1229) - verifies the correct session name ("dev-project") is passed to hookExec
  - "user quits TUI without selection triggers no hook execution" (line 1255) - verifies hookExec is not called when `Selected()` returns ""
- Notes: All five required test scenarios from the plan are covered. The ordering test uses an `orderTrackingConnector` (defined in `cmd/attach_test.go:283-292`) that records call sequence, which is a clean approach. The two pre-existing tests pass `nil` for hookExec, confirming backward compatibility. Tests are focused and not over-tested -- each test verifies a distinct behavior.

CODE QUALITY:
- Project conventions: Followed. Uses the existing DI pattern (injectable function type). `HookExecutorFunc` is defined in a dedicated file (`cmd/hook_executor.go`), consistent with the project's separation concerns. The `buildHookExecutor` factory follows the existing pattern of `buildSessionConnector`.
- SOLID principles: Good. `HookExecutorFunc` is a single-method function type (interface segregation). `processTUIResult` receives its dependencies as parameters (dependency inversion). The function has a single responsibility: handle TUI result by optionally executing hooks then connecting.
- Complexity: Low. The function is 9 lines with straightforward branching (empty selection -> return nil; non-nil hookExec -> call it; connect).
- Modern idioms: Yes. Function type as dependency injection is idiomatic Go.
- Readability: Good. The doc comment on `processTUIResult` (lines 333-336) clearly explains the two paths (selection vs quit). The nil-guard pattern is standard Go.
- Issues: None identified.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
