TASK: Extract Pane Output Parsing Helper in Tmux Package

ACCEPTANCE CRITERIA:
- `parsePaneOutput` is a private function in `internal/tmux/tmux.go`
- `ListPanes` and `ListAllPanes` both call `parsePaneOutput` instead of inlining the parsing
- No duplicated split/trim/filter logic remains between the two methods

STATUS: Complete

SPEC CONTEXT: The specification describes `ListPanes` (returns pane IDs for a session) and `ListAllPanes` (returns all pane IDs across all sessions) as tmux operations needed for hook execution scoping and stale cleanup. Both produce the same output format (newline-delimited pane IDs like `%0`, `%3`). This task is a Phase 4 refactor to DRY up the identical parsing logic that was duplicated across both methods.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tmux/tmux.go:204-220
- `parsePaneOutput` is a private (lowercase) function at line 205
- `ListPanes` delegates to it at line 229: `return parsePaneOutput(output), nil`
- `ListAllPanes` delegates to it at line 240: `return parsePaneOutput(output), nil`
- The function handles: empty string short-circuit (returns `[]string{}`), newline splitting, trimming whitespace from each line, filtering empty lines
- No duplicated split/trim/filter logic remains. The only other `strings.Split` in the file (line 89) is in `ListSessions` which parses a completely different pipe-delimited format
- Notes: None. Implementation matches acceptance criteria exactly.

TESTS:
- Status: Adequate
- Coverage: `parsePaneOutput` is private, so it is tested indirectly through `TestListPanes` and `TestListAllPanes` in `/Users/leeovery/Code/portal/internal/tmux/tmux_test.go`
- `TestListPanes` (line 702): Tests multiple panes, empty output, and error case
- `TestListAllPanes` (line 766): Tests multiple panes across sessions, no server running, empty output, and correct tmux args
- Edge cases from the plan are covered: empty output returns empty slice (both TestListPanes line 733, TestListAllPanes line 802); whitespace-only lines are filtered by the trim+empty check in `parsePaneOutput` (implicitly tested via the mock outputs that contain clean newline-separated data)
- The task's acceptance criteria state: "All existing tests pass: `go test ./internal/tmux/...`" and "All existing tests pass: `go test ./cmd/...`" -- this is a refactor task, so existing tests passing confirms behavioral preservation
- Notes: No direct unit test for `parsePaneOutput` itself, which is appropriate since it is private and fully exercised through the public API. Not over-tested.

CODE QUALITY:
- Project conventions: Followed. Uses the same patterns as the rest of the file (private helper, doc comment, early return for empty input).
- SOLID principles: Good. Single responsibility -- the helper does one thing (parse newline-delimited output). Follows DRY by eliminating the duplication this task was created to address.
- Complexity: Low. Linear scan with simple trim and filter. No branching complexity.
- Modern idioms: Yes. Idiomatic Go with pre-allocated slice (`make([]string, 0, len(lines))`).
- Readability: Good. Clear doc comment, self-explanatory function name, straightforward loop body.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
