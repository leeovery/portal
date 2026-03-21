TASK: StartServer method

ACCEPTANCE CRITERIA:
- StartServer() calls Commander.Run("start-server")
- Returns nil when Commander.Run succeeds
- Returns a wrapped error when Commander.Run fails
- No retry logic -- single call to Commander.Run, result returned immediately
- Tests use MockCommander

STATUS: Complete

SPEC CONTEXT: The spec states "Command: tmux start-server -- starts the tmux server without creating any sessions" and "Server start is a single attempt -- no retry if tmux start-server fails." StartServer is the low-level primitive for the bootstrap mechanism's first phase (server start), called by EnsureServer and ultimately by PersistentPreRunE.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tmux/tmux.go:119-127
- Notes: Implementation is correct and matches all acceptance criteria.
  - Calls `c.cmd.Run("start-server")` (line 122) -- criterion met
  - Returns nil on success (line 126) -- criterion met
  - Returns wrapped error via `fmt.Errorf("failed to start tmux server: %w", err)` (line 124) -- criterion met, uses %w for proper error wrapping
  - Single call, no retry logic, no loops -- criterion met
  - Follows the exact same error wrapping pattern as NewSession, KillSession, RenameSession, SwitchClient -- consistent with codebase conventions
  - Comment on the method documents behavior and no-retry contract (line 119-120)

TESTS:
- Status: Adequate
- Coverage:
  - "starts tmux server successfully" (line 391): Verifies nil return and that exactly "start-server" was passed to Commander.Run via mock.Calls inspection. Covers happy path and correct argument passing.
  - "returns error when start-server fails" (line 411): Verifies non-nil error return, checks error message contains "failed to start tmux server" and the wrapped original error "tmux failed". Covers error wrapping with %w.
  - "does not retry on failure" (line 433): Verifies len(mock.Calls) == 1 after a failing call. Covers the one-shot/no-retry requirement.
- All three planned tests exist with the exact names specified in the plan.
- Tests use MockCommander as required.
- Tests are focused -- each verifies a distinct behavior. No redundancy, no over-testing.
- Would tests fail if feature broke? Yes: removing StartServer breaks compilation; changing the command arg fails test 1; removing error wrapping fails test 2; adding retry fails test 3.

CODE QUALITY:
- Project conventions: Followed. Error wrapping uses the same `fmt.Errorf("failed to ...: %w", err)` pattern as all other Client methods. Method is on *Client receiver, uses Commander interface -- consistent with the rest of the file.
- SOLID principles: Good. Single responsibility (start server, nothing else). Depends on Commander interface (dependency inversion). No violations.
- Complexity: Low. Linear code path, single conditional, cyclomatic complexity 2.
- Modern idioms: Yes. Uses %w for error wrapping (Go 1.13+). Variadic args via Commander interface.
- Readability: Good. Method is 6 lines including the doc comment. Self-documenting. The doc comment explicitly states "No retry logic" which documents the design decision.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
