TASK: session-scrollback-preview-1-5 — Window-grouped pane enumeration on tmux.Client

ACCEPTANCE CRITERIA:
- ListWindowsAndPanesInSession(session) returns ([]WindowGroup, error), exported from internal/tmux.
- Multi-window sessions with multiple panes return correctly grouped/ordered.
- Non-contiguous window_index preserved verbatim without gap-padding.
- Base-index 1 / pane-base-index 1 preserved verbatim.
- Window names with whitespace and with the chosen-delimiter character handled.
- Multiple panes per window grouped and sorted ascending by pane_index.
- Uses c.cmd.Run (trim variant), no direct os/exec.
- No changes to CapturePane or any existing capture wrapper.

STATUS: Complete

SPEC CONTEXT:
Per spec § Multi-pane Rendering Shape > Concrete enumeration call, preview chrome and structural cycling needs a single read-only call returning window-grouped panes plus window names. Spec preferred option (a): a new tmux.Client method using tmux list-panes -s -t <session> -F .... Counter semantics: M and X are 1-based ordinal positions in enumeration order, not raw tmux indices — chrome layer derives them from slice position. Spec's "no new tmux wrapper" rule applies to capture wrappers only.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tmux/tmux.go:424-434 (WindowGroup type)
  - internal/tmux/tmux.go:436-441 (listWindowsAndPanesFieldSep = \x1f)
  - internal/tmux/tmux.go:443-511 (ListWindowsAndPanesInSession method)
- Notes:
  - Picked option (i) from the plan — non-printable ASCII 0x1f (Unit Separator) as field delimiter — the more robust strategy, fully documented.
  - Uses c.cmd.Run (trim variant), matching ListPanesInSession's pattern.
  - Grouping via byIndex map[int]int → result-slice position avoids O(N²) re-scan on append.
  - Final sort.Slice on groups and sort.Ints per PaneIndices ensures deterministic ascending order.
  - First-seen window name wins on duplicate-index rows; documented and pinned by test.
  - Diff to internal/tmux/ is strictly additive — no changes to CapturePane.

TESTS:
- Status: Adequate
- Location: internal/tmux/tmux_test.go:2293-2605 (TestListWindowsAndPanesInSession with 14 sub-tests + assertWindowGroups helper)
- Coverage:
  - Command-vector assertion (list-panes -s -t <session> -F #{window_index}\x1f#{window_name}\x1f#{pane_index}).
  - Window-grouped happy path (3 windows × 2 panes).
  - Non-contiguous window_index [0,2,5] preserved, no padding.
  - Base-index 1 / pane-base-index 1 preserved verbatim.
  - Window names with whitespace preserved.
  - Window names with | round-trip intact.
  - Out-of-order pane_index sorted ascending within window.
  - First-seen window name wins on shared-index rows.
  - Out-of-order window_index sorted ascending in result.
  - Error path: non-zero exit → (nil, err).
  - Empty stdout → ([]WindowGroup{}, nil).
  - Whitespace-only stdout → ([]WindowGroup{}, nil).
  - Wrapped error contains session name, preserves original via errors.Is, exact prefix shape locked.
- Notes: Some empty-stdout / errors.Is / prefix-shape tests technically belong to task 1-6's contract; their presence here doesn't over-test.

CODE QUALITY:
- Project conventions: Followed. Mirrors ListPanesInSession for parsing/grouping/error-wrap shape. Standard Commander-injection DI.
- SOLID principles: Good. Single responsibility, depends on Commander interface.
- Complexity: Low. Linear scan, single-pass grouping via map+slice, two final sorts.
- Modern idioms: Yes. strings.SplitN with explicit cap, sort.Slice + sort.Ints, %w wrapping for errors.Is.
- Readability: Good. Method-level doc comment explains the verbatim-index contract, chrome-layer ordinal mapping, and 0x1f delimiter rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] Implementation silently drops WindowName for repeat-window rows (the documented "first-seen wins" contract). Pinned by a test, so no change needed.
- [idea] sort.Slice on groups could be slices.SortFunc (Go 1.21+); the file uses sort.Slice elsewhere, so consistency wins.
- [quickfix] Doc comment line 451-453 says "non-contiguous window_index values (after window kills) are preserved" — the parenthetical narrows the cause; minor wording.
