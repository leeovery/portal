TASK: Fix command-pending status line position to below title (tick-717ebc)

ACCEPTANCE CRITERIA:
- In command-pending mode, the "Projects" title renders first, followed by the status line, followed by list items
- The status line text remains "Select project to run: {command}"
- All existing command-pending tests pass

STATUS: Complete

SPEC CONTEXT: The specification states (Command-Pending Mode section): "Title stays 'Projects' for consistency" and "A status line below the title indicates the pending command: Select project to run: {command}". The original implementation prepended the status line above the entire list view (including the title). This task fixes the ordering so the title appears first, then the status line, then list items.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:1100-1119
- Notes: The `View()` method now splits the list view output at the first newline (which follows the title line), and inserts the status line after it. The approach uses `strings.IndexByte(listView, '\n')` to find the title boundary, then composes: `listView[:idx+1] + statusLine + "\n" + listView[idx+1:]`. If no newline is found (edge case where view is a single line), it appends the status line after. This correctly produces the ordering: "Projects" title, status line, then list items. The status line text format "Select project to run: " + strings.Join(m.command, " ") matches the spec exactly.

TESTS:
- Status: Adequate
- Coverage: The `TestCommandPendingStatusLine` function at /Users/leeovery/Code/portal/internal/tui/model_test.go:5991-6157 covers:
  1. Status line shows pending command text (line 5992)
  2. Status line absent in normal mode (line 6017)
  3. Multi-word command shown space-separated (line 6045)
  4. Long command text renders without truncation (line 6070)
  5. Title stays "Projects" in command-pending mode (line 6097)
  6. Status line appears after title line, not before (line 6123) -- the key test for this fix
- The ordering test at line 6123-6156 uses string index comparison (`strings.Index`) to verify the "Projects" title comes before "Select project to run: claude" in the rendered output. This test would fail if the status line were positioned above the title.
- Existing tests in `TestCommandPendingMode` (line 2139) also verify status line content and remain passing.
- Notes: The test coverage is well-balanced. The position test (subtest 6) directly validates the fix. The other subtests cover the status line content, multi-word commands, long commands, and absence in normal mode -- all relevant and non-redundant.

CODE QUALITY:
- Project conventions: Followed. Uses table-driven subtest style, idiomatic Go error handling, and integrates with the bubbles/list rendering pattern.
- SOLID principles: Good. The View() method has a clear single responsibility. The string-splitting approach is minimal and doesn't leak implementation details.
- Complexity: Low. The fix is a simple string manipulation in the View() method (3 lines of logic). The IndexByte + slice approach is clear and efficient.
- Modern idioms: Yes. Uses strings.IndexByte for efficient single-byte searching.
- Readability: Good. The comment "// Insert status line after the first line (title) of the list view" on line 1107 clearly explains the intent. The code is self-documenting.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
