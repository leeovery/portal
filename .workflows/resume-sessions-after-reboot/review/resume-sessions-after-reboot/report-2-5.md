TASK: Hook Execution in Direct Path

ACCEPTANCE CRITERIA:
- Inside-tmux: hook execution before SwitchClient
- Outside-tmux: hook execution before Exec
- Hook executor receives correct session name in both paths
- New session creation results in hook execution being a no-op
- Existing TestPathOpener tests pass
- All tests pass: go test ./cmd -run TestPathOpener

STATUS: Complete

SPEC CONTEXT: The specification's "Execution Mechanics" section states: "Hook execution happens before connecting to the session. This is required for AttachConnector (syscall.Exec replaces the process -- nothing can run after) and consistent for SwitchConnector. All Portal connection paths trigger hook execution: TUI picker selection, direct path argument, and portal attach." The direct path (`portal open /path`) is one of the three connection paths requiring hook execution before connect.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `cmd/open.go:198-206` -- PathOpener struct with `hookExec HookExecutorFunc` field
  - `cmd/open.go:211-232` -- PathOpener.Open method with hook execution in both branches
  - `cmd/open.go:217-219` -- Inside-tmux: hookExec called after CreateFromDir, before SwitchClient
  - `cmd/open.go:228-230` -- Outside-tmux: hookExec called after QuickStart.Run, before Exec
  - `cmd/open.go:249-256` -- openPath wires `buildHookExecutor(client)` into PathOpener
  - `cmd/hook_executor.go:1-21` -- Shared HookExecutorFunc type and buildHookExecutor function
- Notes: Implementation uses nil guard (`if po.hookExec != nil`) which allows existing tests that don't set hookExec to continue working without modification. This is a clean backward-compatible approach. The hookExec field receives the correct session name: `sessionName` from CreateFromDir in the inside-tmux branch, and `result.SessionName` from QuickStart in the outside-tmux branch.

TESTS:
- Status: Adequate
- Coverage:
  - "inside tmux executes hooks before switch-client" (line 523) -- verifies ordering via shared callOrder slice with orderTrackingSwitcher
  - "inside tmux hook executor receives session name from CreateFromDir" (line 564) -- verifies correct session name passed
  - "outside tmux executes hooks before exec" (line 592) -- verifies ordering via shared callOrder slice with orderTrackingExecer
  - "outside tmux hook executor receives session name from QuickStart" (line 636) -- verifies correct session name passed
  - "new session creation has no hooks to execute" (line 670) -- verifies hookExec is still called (executor handles no-op internally)
  - Existing tests (lines 285-521) pass because hookExec is nil and Open has nil guards
- Notes: All six required tests from the plan are present. The ordering tests use a shared `callOrder` slice pattern with tracking wrapper types, which is an effective way to verify call sequencing. The "new session" test correctly verifies that hookExec IS called (the no-op behavior is internal to ExecuteHooks, not a skip at the PathOpener level). Not over-tested -- each test targets a distinct concern.

CODE QUALITY:
- Project conventions: Followed. Uses the established DI pattern (struct fields for dependencies, nil checks for optional deps). HookExecutorFunc is a shared type in cmd/hook_executor.go, consistent with the plan's suggestion. The mockSwitchClient and mockExecer tracking wrappers follow the same pattern used elsewhere in the test file.
- SOLID principles: Good. PathOpener has a single responsibility (open a path as a session). The hookExec is injected as a function (dependency inversion). The HookExecutorFunc type is narrow (interface segregation equivalent for functions).
- Complexity: Low. The Open method has two clear branches (insideTmux or not), each with a linear flow: create/run -> hookExec -> connect/exec.
- Modern idioms: Yes. Function type as injectable dependency is idiomatic Go.
- Readability: Good. The nil guard pattern is clear and self-documenting. The comment on processTUIResult (line 333-336) documents the hookExec behavior.
- Issues: None found.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The nil guard pattern (`if po.hookExec != nil`) is pragmatic for backward compatibility, but means PathOpener callers must remember to set hookExec. The production path (openPath) correctly wires it. If a future caller forgets, hooks will silently not execute. An alternative would be to require it in a constructor, but the current approach matches the existing codebase pattern (struct literal construction).
