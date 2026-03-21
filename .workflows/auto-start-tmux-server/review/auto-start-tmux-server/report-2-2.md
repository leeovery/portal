TASK: Propagate serverStarted via command context

ACCEPTANCE CRITERIA:
- serverWasStarted(cmd) returns true when EnsureServer returned serverStarted=true
- serverWasStarted(cmd) returns false when EnsureServer returned serverStarted=false
- serverWasStarted(cmd) returns false when context value was never set (skipTmuxCheck commands)
- PersistentPreRunE stores serverStarted in command context
- Context key type is unexported
- All existing tests continue to pass

STATUS: Complete

SPEC CONTEXT: The spec's "Two-phase ownership" section describes that bootstrap has two phases: server start (PersistentPreRunE, shared) and session wait (context-specific). CLI and TUI paths need to know whether the server was just started so they can decide whether to show loading UX. This task provides the mechanism via cobra command context propagation.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - cmd/bootstrap_context.go:1-42 — context key type, serverStartedKey constant, serverWasStarted() helper, plus tmuxClient() helper (added by later phases)
  - cmd/root.go:62-70 — PersistentPreRunE stores serverStarted and client in context via cmd.SetContext
- Notes:
  - The contextKey type is unexported (line 9: `type contextKey string`) -- meets acceptance criteria
  - serverStartedKey is a typed constant of type contextKey (line 12) -- prevents collisions
  - serverWasStarted() handles nil context (line 21-23) -- safe for bare cobra.Command instances
  - PersistentPreRunE at root.go:62 captures the return value from EnsureServer and stores it at line 66
  - The implementation also includes tmuxClientKey and tmuxClient() helper (lines 14-42) which were added by later tasks (Phase 4 task 4-2). This is scope beyond task 2-2 but is additive and non-conflicting.
  - Minor deviation from plan: the plan showed `serverWasStarted` as a one-liner without nil-context guard. The implementation adds a nil-context check (lines 20-23), which is a defensive improvement. The plan's example code did not handle `cmd.Context()` returning nil, which would panic. This is a positive drift.

TESTS:
- Status: Adequate
- Coverage:
  - cmd/bootstrap_context_test.go:34-72 — TestServerWasStarted with 4 subtests matching all planned tests exactly:
    1. "returns true when context has serverStarted=true" (line 35)
    2. "returns false when context has serverStarted=false" (line 45)
    3. "returns false when context has no serverStarted value" (line 55) -- bare cobra.Command, no SetContext
    4. "returns false for nil context value of wrong type" (line 63) -- string value for bool key
  - cmd/root_test.go:230-290 — Integration tests verifying PersistentPreRunE actually stores the value and it flows to RunE:
    1. "PersistentPreRunE stores serverStarted=true in context" (line 230) -- end-to-end with mock bootstrapper
    2. "PersistentPreRunE stores serverStarted=false in context" (line 259) -- end-to-end with mock bootstrapper
  - cmd/bootstrap_context_test.go:10-31 — TestTmuxClient panic test (bonus, from later task)
- Notes:
  - All 4 planned tests are present with exact names matching the plan
  - Integration tests in root_test.go go beyond unit testing serverWasStarted -- they verify the full PersistentPreRunE -> RunE flow, which is exactly what acceptance criterion #4 requires
  - Tests are focused and non-redundant. Each tests a distinct scenario
  - Test for "no serverStarted value" (line 55) tests the case where cobra.Command has no context set at all (nil context path), covering skipTmuxCheck commands
  - Would the tests fail if the feature broke? Yes -- removing the context.WithValue in root.go would cause the root_test.go integration tests to fail. Changing serverWasStarted logic would cause bootstrap_context_test.go to fail.

CODE QUALITY:
- Project conventions: Followed -- uses idiomatic Go context propagation, unexported types for encapsulation, cobra patterns for hook-to-run communication
- SOLID principles: Good -- serverWasStarted is a single-responsibility helper; the context key type prevents cross-package collisions (dependency inversion via context); ServerBootstrapper interface in root.go enables testing
- Complexity: Low -- serverWasStarted is a simple nil-check + type assertion, ~10 lines. PersistentPreRunE integration is a straightforward context.WithValue + SetContext
- Modern idioms: Yes -- uses context.WithValue with typed keys (not string keys), cobra's SetContext/Context pattern, two-value type assertion
- Readability: Good -- clear comments on all exported and unexported symbols, function names are self-documenting (serverWasStarted reads naturally as a question)
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The bootstrap_context.go file also contains tmuxClient() and tmuxClientKey from a later task (Phase 4). This is fine structurally -- the file name "bootstrap_context" accurately describes both context values. No action needed.
