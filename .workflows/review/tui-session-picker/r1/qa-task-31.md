TASK: Add [q] quit binding to all help bars (tick-1c66e9)

ACCEPTANCE CRITERIA:
- [q] quit appears in the help bar on the Sessions page
- [q] quit appears in the help bar on the Projects page
- [q] quit appears in the help bar in command-pending mode
- No change to actual quit behavior (already handled elsewhere)

STATUS: Complete

SPEC CONTEXT: The specification defines exact help bar layouts for all three contexts. Sessions: `[enter] attach [r] rename [k] kill [p] projects [n] new in cwd [/] filter [q] quit`. Projects: `[enter] new session [e] edit [d] delete [s] sessions [n] new in cwd [b] browse [/] filter [q] quit`. Command-pending: `[enter] run here [n] new in cwd [b] browse [/] filter [q] quit`. In all cases `[q] quit` is the last entry.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:360` -- `sessionHelpKeys()` includes `key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))` as the last entry
  - `/Users/leeovery/Code/portal/internal/tui/model.go:386` -- `projectHelpKeys()` includes the same binding as the last entry
  - `/Users/leeovery/Code/portal/internal/tui/model.go:397` -- `commandPendingHelpKeys()` includes the same binding as the last entry
- Notes: All three help key functions now include the `q`/`quit` binding as the final entry in their returned slice, matching the spec layout exactly. The actual quit behavior remains unchanged at lines 936-937 (sessions page) and 642-643 (projects page), where `isRuneKey(msg, "q")` triggers `tea.Quit`. These are display-only bindings as intended.

TESTS:
- Status: Adequate
- Coverage:
  - `TestHelpBarQuitBinding/session help bar includes quit binding` (line 7294): Renders the sessions view and asserts "quit" appears in the output
  - `TestHelpBarQuitBinding/project help bar includes quit binding` (line 7307): Navigates to projects page, renders view, asserts "quit" appears
  - `TestHelpBarQuitBinding/command-pending help bar includes quit binding` (line 7334): Creates model in command-pending mode, renders view, asserts "quit" appears
- Notes: All three acceptance criteria are covered. Tests verify rendered output contains "quit", which is the correct level of abstraction (behavior, not implementation). The tests would fail if the binding were removed. No over-testing observed -- one test per help bar context is appropriate.

CODE QUALITY:
- Project conventions: Followed -- consistent with existing help key function patterns
- SOLID principles: Good -- display-only binding added to existing functions, no new responsibilities introduced
- Complexity: Low -- simple addition of one binding per function
- Modern idioms: Yes -- uses the `key.NewBinding` API consistently with existing bindings
- Readability: Good -- the bindings are self-documenting
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
