TASK: Update ListPanes and ListAllPanes to Return Structural Keys

ACCEPTANCE CRITERIA:
- ListPanes uses format string #{session_name}:#{window_index}.#{pane_index}
- ListAllPanes uses format string #{session_name}:#{window_index}.#{pane_index}
- Doc comments describe structural keys, not pane IDs
- All existing pane tests pass with structural key values
- New edge case tests pass
- go test ./internal/tmux/... passes

STATUS: Complete

SPEC CONTEXT: The specification requires changing `ListPanes` and `ListAllPanes` from returning ephemeral pane IDs (`%0`, `%1`) to structural keys (`session_name:window_index.pane_index`). This format survives tmux-resurrect because it matches the positional addressing scheme resurrect uses internally. Function signatures remain `[]string` -- only the semantic meaning of the returned strings changes. Keys are treated as opaque strings, so session names with colons or dots are acceptable.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tmux/tmux.go:233-241` -- `ListPanes` uses `"#{session_name}:#{window_index}.#{pane_index}"` format string
  - `internal/tmux/tmux.go:244-254` -- `ListAllPanes` uses `"#{session_name}:#{window_index}.#{pane_index}"` format string
  - `internal/tmux/tmux.go:216-231` -- `parsePaneOutput` shared helper unchanged (correctly reused)
  - `internal/tmux/tmux.go:257-258` -- `SendKeys` doc comment updated to reference structural keys
- Notes: Implementation exactly matches the acceptance criteria. No old `#{pane_id}` format remains in the source. Doc comments for both methods explicitly describe structural keys and note they survive tmux server restarts unlike ephemeral pane IDs. The only remaining references to "pane ID" in `tmux.go` are in `ResolveStructuralKey` (which legitimately takes a pane ID as input) and in contrast clauses ("unlike ephemeral pane IDs"), both correct.

TESTS:
- Status: Adequate
- Coverage:
  - TestListPanes (lines 702-825): 6 subtests
    - "returns structural keys for session with multiple panes" -- verifies 3-pane output AND exact format string in tmux command
    - "returns structural keys for multi-window multi-pane session" -- verifies cross-window distinction (0.0, 0.1, 1.0, 1.1)
    - "handles session names with colons" -- edge case: "my:project:0.0"
    - "handles session names with dots" -- edge case: "my.project:0.0"
    - "returns empty slice when session has no panes" -- existing test preserved
    - "returns error when session does not exist" -- existing test preserved
  - TestListAllPanes (lines 827-953): 7 subtests
    - "returns structural keys across multiple sessions" -- verifies multi-session output
    - "returns structural keys for multi-window multi-pane session" -- 5-pane cross-window test
    - "handles session names with colons" -- edge case
    - "handles session names with dots" -- edge case
    - "returns empty slice when no tmux server running" -- existing test preserved
    - "returns empty slice when output is empty" -- existing test preserved
    - "calls list-panes with -a flag and structural key format" -- verifies exact tmux command args
- Notes: All 6 expected test names from the task are present. Edge cases for colons and dots are covered in both TestListPanes and TestListAllPanes. The format string verification tests confirm the exact tmux `-F` argument, which means tests would fail if the feature regressed. No over-testing detected -- each test covers a distinct scenario (single session, multi-session, multi-window, colons, dots, empty, error).

CODE QUALITY:
- Project conventions: Followed. Table-driven style not used here but subtests with `t.Run` are consistent with the existing test patterns in this file. No `t.Parallel()` (per CLAUDE.md rules).
- SOLID principles: Good. `parsePaneOutput` helper is correctly shared (DRY). `ListPanes` and `ListAllPanes` have single responsibilities.
- Complexity: Low. Both methods are straightforward: call tmux, parse output. No branching beyond error handling.
- Modern idioms: Yes. Standard Go error wrapping with `fmt.Errorf` and `%w`.
- Readability: Good. Doc comments are clear and include examples of the structural key format. The contrast with "ephemeral pane IDs" helps readers understand why the change was made.
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
