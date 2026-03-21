TASK: WaitForSessions polling function

ACCEPTANCE CRITERIA:
- Named constants DefaultMinWait (1s), DefaultMaxWait (6s), DefaultPollInterval (500ms) are exported
- WaitForSessions always waits at least MinWait even if sessions appear immediately
- WaitForSessions exits as soon as sessions are detected after MinWait has elapsed
- WaitForSessions always returns by MaxWait even if no sessions appear
- WaitConfig.HasSessions is injectable for testability
- All tests run in under 1 second

STATUS: Complete

SPEC CONTEXT: The spec's "Timing" section requires session-detection with min/max bounds: minimum 1 second (prevents jarring flash), maximum 6 seconds (proceed regardless). Poll interval 500ms. Not user-configurable. Values defined as named constants. The function serves both CLI and TUI paths (TUI uses the constants directly via its own refresh cycle rather than calling WaitForSessions).

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tmux/wait.go:1-72
- Notes: Implementation uses `time.NewTicker` for polling, `time.NewTimer` for MinWait and MaxWait deadlines, and a `select` loop to coordinate. All three are properly cleaned up with `defer Stop()`. No goroutines are spawned, so no lifecycle concerns. The `DefaultWaitConfig` constructor (line 26-36) correctly wires `client.ListSessions()` into the `HasSessions` callback. The function matches all acceptance criteria: min wait is enforced via `minElapsed` flag gating the ticker return path; early exit after min wait works because the ticker case returns when `minElapsed && sessionsFound`; max wait is enforced by the unconditional `return` in the `deadline.C` case.

TESTS:
- Status: Adequate
- Coverage: All 5 required test cases from acceptance criteria are present:
  1. "returns after min wait when sessions appear before min wait" (line 12)
  2. "returns at max wait when no sessions ever appear" (line 33)
  3. "exits early when sessions appear between min and max" (line 53)
  4. "polls at the configured interval" (line 78)
  5. "sessions detected on first poll still waits for min wait" (line 103)
- Notes: Tests use fast timing (50ms/200ms/10ms) so total suite runs well under 1 second. Tests use `atomic.Int32` for thread-safe poll counting. Tests 1 and 5 have similar setups (HasSessions always true, assert >= MinWait) but each has a distinct assertion focus (timing bounds vs poll count verification), so this is acceptable overlap rather than redundancy. The plan's full test list included a 6th test ("returns immediately after min wait when sessions appear exactly at min wait") which is not implemented, but it was not in the acceptance criteria provided and the behavior is adequately covered by tests 1 and 3. The test file uses external test package (`tmux_test`) which is idiomatic Go.

CODE QUALITY:
- Project conventions: Followed. External test package, subtests, exported types/constants properly documented.
- SOLID principles: Good. Single responsibility (WaitForSessions does one thing). Dependency inversion via injectable HasSessions callback. WaitConfig struct enables open/closed principle for future extensions.
- Complexity: Low. Single select loop with three cases, two boolean flags. Easy to reason about.
- Modern idioms: Yes. Uses time.NewTicker/NewTimer (not time.After in a loop), proper defer cleanup, atomic operations in tests.
- Readability: Good. Clear variable names (sessionsFound, minElapsed, deadline), good doc comments on struct fields and function.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- WaitForSessions does not accept a context.Context parameter. The golang-pro skill recommends "Add context.Context to all blocking operations." However, the function has a bounded MaxWait (6s production) and the spec describes this as a one-shot non-cancellable wait, so this is acceptable. If cancellation support is needed later (e.g., user presses Ctrl+C during wait), a context parameter could be added.
- The 30ms tolerance in timing assertions is tight enough to be reliable on modern hardware but could theoretically flake under extreme CI load. The current values appear to work well given the project's test history.
