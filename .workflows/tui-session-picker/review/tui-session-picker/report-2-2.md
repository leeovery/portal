TASK: Command-Pending Status Line and Help Bar (tick-f8d97a)

ACCEPTANCE CRITERIA:
- Status line shows "Select project to run: {command}" when command-pending active
- Title remains "Projects" in command-pending mode
- Single-word command renders correctly
- Multi-word command renders all args space-separated
- Status line absent in normal mode
- Long command text not artificially truncated

STATUS: Complete

SPEC CONTEXT: The specification defines command-pending mode behavior: "Title stays 'Projects' for consistency. A status line below the title indicates the pending command: Select project to run: {command}". Help bar keybindings for command-pending: "[enter] run here  [n] new in cwd  [b] browse  [/] filter  [q] quit". The s, x, e, d keybindings are not registered in command-pending mode.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1100-1119` -- View() method renders status line when commandPending is true
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1106` -- Status line construction: `"Select project to run: " + strings.Join(m.command, " ")`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1108-1109` -- Insertion after first line (title) of list view
  - `/Users/leeovery/Code/portal/internal/tui/model.go:390-398` -- `commandPendingHelpKeys()` returns restricted keybindings (enter/run here, browse, new in cwd, quit)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:267-276` -- `WithCommand()` sets commandPending=true, activePage=PageProjects, and swaps help keys to commandPendingHelpKeys
- Notes: Implementation is clean and matches the spec. The status line is inserted after the title by finding the first newline in the rendered list view. The help bar correctly shows "run here" instead of "new session" and omits s, x, e, d. Title stays "Projects" because `WithCommand` only changes help keys and sets commandPending/activePage -- it does not modify the list title.

TESTS:
- Status: Adequate
- Coverage:
  - `TestCommandPendingStatusLine` at line 5991 contains 6 subtests:
    1. "status line shows pending command text" (line 5992) -- verifies single-word command "claude"
    2. "status line absent in normal mode" (line 6017) -- verifies no status line without command-pending
    3. "multi-word command shown space-separated" (line 6045) -- verifies "claude --resume --model opus"
    4. "long command text renders without truncation" (line 6070) -- verifies long command string is fully present
    5. "title stays Projects in command-pending mode" (line 6097) -- verifies "Projects" title persists
    6. "status line appears after title line not before" (line 6123) -- verifies ordering: title before status line
  - Additional help bar tests exist elsewhere:
    - "help bar omits s, x, e, and d in command-pending mode" (line 2621)
    - "help bar shows run here for enter in command-pending mode" (line 2657)
    - "command-pending help bar includes quit binding" (line 7334)
  - Additional overlap coverage from earlier command-pending core tests (lines 2434, 2439, 2466, 2484)
- Notes: All 5 planned tests from the task are present, plus a bonus test for status line ordering relative to title. The test at line 6017 ("status line absent in normal mode") is somewhat fragile -- it manually sends a ProjectsLoadedMsg to switch the view to projects page rather than using the normal Init flow, but this is acceptable since it correctly verifies the absence of the status line. Help bar tests are thorough and spread across multiple test functions covering the restriction of keybindings. There is minor duplication between the dedicated TestCommandPendingStatusLine tests and similar assertions in earlier test functions (e.g., lines 2434-2462), but these are in different test contexts (the earlier ones test broader command-pending behavior) so the overlap is justified.

CODE QUALITY:
- Project conventions: Followed -- functional options pattern, table-driven test style (via subtests), idiomatic Go
- SOLID principles: Good -- View() has a clear single responsibility; status line rendering is inline in View() rather than extracted, but given its simplicity (2 lines of logic) this is appropriate
- Complexity: Low -- the status line insertion is a simple string split and concatenation
- Modern idioms: Yes -- uses strings.IndexByte for efficient single-byte search, strings.Join for slice-to-string
- Readability: Good -- the code is self-documenting with a clear comment explaining the insertion point
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The status line rendering in View() (lines 1104-1111) works by string manipulation on the already-rendered list view, inserting a line after the title. This is a pragmatic approach but couples the rendering to the assumption that the first line of the list view is always the title. If bubbles/list ever changes its rendering to add a blank line or prefix before the title, this would break silently. This is a very unlikely scenario and not worth over-engineering against, but worth noting.
