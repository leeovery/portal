TASK: Add ResolveStructuralKey Method to tmux.Client

ACCEPTANCE CRITERIA:
- ResolveStructuralKey method exists on tmux.Client
- Method runs correct tmux display-message command
- Returns structural key string on success
- Returns wrapped error on failure with pane ID in message
- All three test cases pass
- go test ./internal/tmux/... passes

STATUS: Complete

SPEC CONTEXT: The specification requires a new tmux.Client method that resolves an ephemeral pane ID (from $TMUX_PANE) into a structural key (session_name:window_index.pane_index) using `tmux display-message -p -t <paneID> "#{session_name}:#{window_index}.#{pane_index}"`. This method is consumed by Phase 3 tasks for hook registration and removal, replacing direct use of $TMUX_PANE as the storage key. The structural key format survives tmux-resurrect; pane IDs do not.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:157-166
- Notes: Method is placed after CurrentSessionName, consistent with the plan. Uses the exact tmux command specified: `display-message -p -t <paneID> "#{session_name}:#{window_index}.#{pane_index}"`. Error wrapping uses `%w` for proper error chain support and includes the pane ID via `%q` formatting. Follows the same pattern as CurrentSessionName (lines 149-155). The method is already consumed by Phase 3 code in cmd/hooks.go via a StructuralKeyResolver interface (line 26), confirming proper integration.

TESTS:
- Status: Adequate
- Coverage: All three required test cases present in internal/tmux/tmux_test.go:955-1009
  1. "returns structural key for valid pane ID" (line 956) - verifies correct return value, correct tmux command args (display-message -p -t %3 #{session_name}:#{window_index}.#{pane_index}), and single call to mock
  2. "returns error for invalid pane ID" (line 979) - verifies error returned and error message contains the pane ID "%99"
  3. "returns error when tmux command fails" (line 993) - verifies error contains "failed to resolve structural key" prefix and the pane ID "%0"
- Notes: Tests are focused and not over-tested. Each test verifies distinct behavior. The happy path test validates both the return value and the exact args passed to tmux (important for correctness). Minor deviation from plan: test uses "my-project:0.1" instead of plan's "my-project-abc:0.0" as the mock return value — inconsequential, the structural key format is still correctly represented.

CODE QUALITY:
- Project conventions: Followed. Uses the same DI pattern (Commander interface) as all other Client methods. Error wrapping with fmt.Errorf and %w is consistent with the codebase. Doc comment present on exported method.
- SOLID principles: Good. Single responsibility — one method, one concern. Follows the existing interface/implementation pattern of the Client type.
- Complexity: Low. Linear code path, single tmux command, simple error handling.
- Modern idioms: Yes. Proper error wrapping with %w, clean method receiver pattern.
- Readability: Good. Doc comment clearly explains input (pane ID like "%3") and output (structural key like "my-project:0.1"). Method name is descriptive.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The structural key format string "#{session_name}:#{window_index}.#{pane_index}" is duplicated across ResolveStructuralKey, ListPanes, and ListAllPanes. This was noted in the implementation analysis (analysis-duplication-c2) as low severity. A constant could reduce duplication but is not necessary for correctness.
