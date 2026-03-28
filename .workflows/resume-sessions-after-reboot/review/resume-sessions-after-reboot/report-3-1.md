TASK: ListAllPanes Tmux Method

ACCEPTANCE CRITERIA:
- Calls `tmux list-panes -a -F #{pane_id}`
- Returns empty slice on Commander error (no server running)
- Returns empty slice when output is empty
- All tests pass: `go test ./internal/tmux/...`

STATUS: Complete

SPEC CONTEXT: The "Stale Registration Cleanup" section requires cross-referencing hook pane IDs against live tmux panes. `ListAllPanes` provides the set of all live pane IDs across all sessions, which is needed for the cleanup logic. The `-a` flag on `tmux list-panes` returns panes from all sessions. The error-swallowing pattern (empty slice on Commander error) allows callers to treat "no server" as "no live panes" without special error handling.

IMPLEMENTATION:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/internal/tmux/tmux.go:235-241`
- Notes: Implementation is clean and correct. Calls `c.cmd.Run("list-panes", "-a", "-F", "#{pane_id}")` exactly as specified. Returns `[]string{}, nil` on Commander error, matching the `ListSessions` pattern. Delegates output parsing to the shared `parsePaneOutput` helper (lines 205-220), which handles empty output, whitespace trimming, and empty line filtering. Good DRY -- `parsePaneOutput` is shared with `ListPanes` (line 229).

TESTS:
- Status: Adequate
- Coverage: Four subtests in `TestListAllPanes` (lines 766-831):
  1. "returns pane IDs across multiple sessions" -- verifies multi-pane parsing with `%0`, `%1`, `%5`, `%12`
  2. "returns empty slice when no tmux server running" -- verifies Commander error produces empty slice and nil error
  3. "returns empty slice when output is empty" -- verifies empty string output produces empty slice
  4. "calls list-panes with -a flag" -- verifies exact command arguments: `list-panes -a -F #{pane_id}`
- Notes: All four tests from the task's test list are present. The plan's expanded test list included two additional tests ("returns single pane ID" and "trims whitespace from output lines") that are not present as separate subtests. The single pane case is implicitly tested in test 4 (output is `%0`). Whitespace trimming is handled by `parsePaneOutput` which is shared with `ListPanes` and the behavior is deterministic, so the absence of a dedicated whitespace test for `ListAllPanes` is not a gap -- it would be over-testing. Test balance is appropriate.

CODE QUALITY:
- Project conventions: Followed. Uses MockCommander pattern, table-style subtests, no `t.Parallel()`, same error-handling conventions as existing methods.
- SOLID principles: Good. Single responsibility -- `parsePaneOutput` is a pure parsing function extracted for reuse. `ListAllPanes` follows the same contract pattern as `ListSessions`.
- Complexity: Low. Method is 7 lines with a single branch (error check).
- Modern idioms: Yes. Idiomatic Go error handling, explicit empty slice return.
- Readability: Good. Clear doc comment explains return behavior. Method is self-documenting.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
