TASK: ServerRunning method

ACCEPTANCE CRITERIA:
- ServerRunning() returns true when Commander.Run("info") succeeds
- ServerRunning() returns false when Commander.Run("info") returns an error
- The method passes exactly "info" as the sole argument to Commander.Run
- Tests use MockCommander -- no real tmux process in unit tests

STATUS: Complete

SPEC CONTEXT: The spec requires detection of whether the tmux server is running (e.g., "tmux info" succeeding) as part of the bootstrap mechanism. ServerRunning is the detection primitive used by the higher-level EnsureServer function. The spec explicitly mentions "tmux info" as a valid detection method.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:47-50
- Notes: Clean 3-line method on *Client. Calls c.cmd.Run("info") and returns err == nil. Matches acceptance criteria exactly. Good doc comment explains why "info" is used ("succeeds even with zero sessions"). Follows the same boolean-from-error pattern used by HasSession() on line 54-57, maintaining consistency within the file.

TESTS:
- Status: Adequate
- Coverage: All three required test cases present in internal/tmux/tmux_test.go:125-165:
  1. "returns true when tmux server is running" (line 126) -- MockCommander with zero-value Err (nil), asserts true
  2. "returns false when no tmux server is running" (line 137) -- MockCommander with realistic error message, asserts false
  3. "calls tmux info to check server status" (line 148) -- Verifies exactly 1 call, exactly 1 arg, arg is "info"
- Notes: Tests use MockCommander (defined at line 12-28), no real tmux process. Tests are well-balanced -- each tests a distinct concern (happy path, error path, argument verification). Not over-tested: no redundant assertions. The mock records calls via Calls slice, enabling the argument verification test without excessive coupling.

CODE QUALITY:
- Project conventions: Followed. Uses table-driven subtests pattern consistent with golang-pro skill guidance. External test package (tmux_test). MockCommander follows the interface mocking pattern from the testing reference.
- SOLID principles: Good. Dependency inversion via Commander interface. Single responsibility -- method does one thing (detect server).
- Complexity: Low. Cyclomatic complexity of 1 (single return path with no branching).
- Modern idioms: Yes. Idiomatic Go error-to-bool pattern. Variadic args in Commander.Run interface.
- Readability: Good. Self-documenting method name, clear doc comment, minimal code.
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
