TASK: Hook Execution in Attach Command

ACCEPTANCE CRITERIA:
- Hook execution runs before connector.Connect
- Hook executor receives correct session name
- Runs before syscall.Exec replaces process (verified by test ordering)
- AttachDeps.HookExecutor injectable for testing
- Existing attach tests pass
- All tests pass: go test ./cmd -run TestAttach

STATUS: Complete

SPEC CONTEXT: The specification's "Execution Mechanics" section requires hook execution before connecting to a session. This is critical for AttachConnector because syscall.Exec replaces the process -- nothing can run after it. All Portal connection paths (TUI picker, direct path, portal attach) must trigger hook execution. The two-condition check (persistent entry exists AND volatile marker absent) determines which panes need restart commands.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/attach.go:19-23 (AttachDeps struct with HookExecutor field)
  - cmd/attach.go:34 (buildAttachDeps returns hookExecutor as third value)
  - cmd/attach.go:40-42 (nil-guarded hookExecutor call between HasSession and Connect)
  - cmd/attach.go:51-59 (buildAttachDeps function returning connector, validator, hookExecutor)
  - cmd/hook_executor.go:7-8 (HookExecutorFunc type definition)
  - cmd/hook_executor.go:10-21 (buildHookExecutor factory function)
- Notes: Implementation matches the plan precisely. The insertion point is correct -- hook execution at line 40-42 is after `validator.HasSession(name)` (line 36) and before `connector.Connect(name)` (line 44). The nil guard at line 40 ensures backward compatibility with existing tests that don't set HookExecutor. The `HookExecutorFunc` type and `buildHookExecutor` are placed in a shared `cmd/hook_executor.go` file, which the plan explicitly allows for reuse across tasks 3-5.

TESTS:
- Status: Adequate
- Coverage:
  - "hook execution runs before connect" (line 155): Uses orderTrackingConnector to verify ["hooks", "connect"] ordering. This directly validates the syscall.Exec constraint.
  - "hook execution receives correct session name" (line 196): Verifies the session name is passed through to the hook executor.
  - "non-existent session skips hook execution" (line 226): Verifies hooks don't fire when session doesn't exist (early return before hook call).
  - "hook executor is called when session exists" (line 252): Verifies positive case that hook executor is called for existing sessions.
  - Existing tests (lines 34-153): All 5 original tests remain unchanged and function correctly because HookExecutor defaults to nil and the nil guard skips execution.
- Notes: The plan listed a test "session with no panes triggers no hook execution" but the implementation has "hook executor is called when session exists" instead. This is a reasonable adaptation -- at the attach command abstraction level, the command calls the hook executor regardless of pane count. The "no panes" edge case is an internal detail of hooks.ExecuteHooks, properly tested in task 2-2. The replacement test verifies the complementary positive case at the correct abstraction level.

CODE QUALITY:
- Project conventions: Followed. Uses the established DI pattern (package-level *Deps struct with test injection via t.Cleanup). No t.Parallel() as required by CLAUDE.md. Follows the existing mockSessionConnector/mockSessionValidator patterns for test mocks.
- SOLID principles: Good. Single responsibility maintained -- HookExecutorFunc encapsulates all hook execution details, keeping the attach command lean. Dependency inversion via the function type injection. Interface segregation maintained by using the existing TmuxOperator composition in hooks package.
- Complexity: Low. The attach command RunE is straightforward linear flow: bootstrapWait, validate, hooks, connect. The nil guard is the only branch added.
- Modern idioms: Yes. Function type as dependency (HookExecutorFunc) is idiomatic Go. The orderTrackingConnector test helper is clean and focused.
- Readability: Good. Clear variable names, comments on exported types and functions. The hook_executor.go file has proper documentation explaining the purpose.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The orderTrackingConnector helper (attach_test.go:283-292) is well-designed for verifying call ordering and could be reused across other command tests if needed.
