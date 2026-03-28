TASK: ListPanes and SendKeys Tmux Methods

ACCEPTANCE CRITERIA:
- ListPanes calls `tmux list-panes -t <sessionName> -F #{pane_id}`
- ListPanes returns pane IDs as string slice
- ListPanes returns empty slice for no output
- SendKeys calls `tmux send-keys -t <paneID> <command> Enter`
- All tests pass: `go test ./internal/tmux/...`

STATUS: Complete

SPEC CONTEXT: The Execution Mechanics section specifies that Portal uses `tmux send-keys` to deliver restart commands to panes, typing the command as if the user typed it. Hook execution is scoped to the target session's panes -- Portal queries tmux for the session's panes and cross-references the registry. The `list-panes` command with `-F #{pane_id}` returns pane IDs in the `%N` format matching the hook store's key format.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tmux/tmux.go:224-250
- Notes:
  - `ListPanes` (lines 224-230) calls `c.cmd.Run("list-panes", "-t", sessionName, "-F", "#{pane_id}")` -- exactly matches acceptance criteria
  - `SendKeys` (lines 244-250) calls `c.cmd.Run("send-keys", "-t", paneID, command, "Enter")` -- exactly matches acceptance criteria
  - Both return properly wrapped errors with `%w` verb for error chaining
  - A shared `parsePaneOutput` helper (lines 205-220) was extracted to handle newline splitting, trimming, and empty filtering -- reused by both `ListPanes` and `ListAllPanes`
  - `ListAllPanes` (lines 233-241) was added beyond the plan scope -- uses `-a` flag to list all panes across all sessions. This supports stale cleanup (spec: "cross-reference pane IDs against live tmux panes") and is a reasonable addition

TESTS:
- Status: Adequate
- Coverage:
  - TestListPanes (lines 702-764): 3 subtests covering multi-pane happy path with arg verification, empty output, and error propagation with session name in message
  - TestListAllPanes (lines 766-831): 4 subtests covering multi-session panes, no-server error (empty slice), empty output, and arg verification with `-a` flag
  - TestSendKeys (lines 833-874): 2 subtests covering happy path with full arg verification (including "Enter"), and error propagation with pane ID in message
- The plan listed 8 tests; 5 distinct test cases exist for ListPanes/SendKeys. The "missing" ones (single pane, separate arg verification tests) are effectively covered within existing tests -- the multi-pane test verifies args, so a separate "calls with correct args" test would be redundant
- Tests use the existing MockCommander pattern consistently with the rest of the file
- Tests would fail if the feature broke (arg order, error wrapping, output parsing)

CODE QUALITY:
- Project conventions: Followed -- uses Commander interface DI, MockCommander for tests, same error wrapping pattern as all other Client methods, table-driven-style test structure
- SOLID principles: Good -- single responsibility (parsePaneOutput is a focused helper), open/closed (Commander interface unchanged), interface segregation (Commander stays minimal at 1 method)
- Complexity: Low -- both methods are thin wrappers around Commander.Run with straightforward error handling
- Modern idioms: Yes -- error wrapping with %w, slice pre-allocation in parsePaneOutput, consistent Go conventions
- Readability: Good -- clear doc comments on all exported methods, parsePaneOutput name is self-documenting
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- `ListAllPanes` was added beyond plan scope -- this is acceptable as it serves the stale cleanup requirement from the spec and follows the same pattern
- `parsePaneOutput` extraction is good DRY practice -- avoids duplicating the newline splitting logic between `ListPanes` and `ListAllPanes`
