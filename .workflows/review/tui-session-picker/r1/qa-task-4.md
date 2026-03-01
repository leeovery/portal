TASK: Rename Modal with TextInput (tick-34ba3d)

ACCEPTANCE CRITERIA:
- [x] r (lowercase) on a selected session opens the rename modal
- [x] Rename modal shows a textinput pre-populated with the current session name
- [x] Enter with non-empty input renames the session and refreshes
- [x] Enter with empty input is a no-op (modal stays open)
- [x] Esc dismisses the rename modal without renaming
- [x] After rename, the session list refreshes showing the new name
- [x] r on an empty list is a no-op
- [x] r with no session renamer configured is a no-op

STATUS: Complete

SPEC CONTEXT: The spec says "r triggers a modal overlay with a textinput pre-populated with the current session name. On confirm: rename session via tmux, refresh list." The help bar shows `[r] rename`. The modal system uses lipgloss.Place()-style overlay with styled content centered over the list view.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:940-941` - r keybinding routed to handleRenameKey
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1026-1042` - handleRenameKey: guards for empty list + nil renamer, creates textinput with "New name: " prompt, pre-populates with session name, focuses
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1044-1068` - updateRenameModal: Enter trims/rejects empty, calls renameAndRefresh; Esc dismisses; delegates other input to textinput.Update
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1070-1078` - renameAndRefresh: calls RenameSession then ListSessions, returns SessionsMsg (with Err on failure)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1182-1189` - viewSessionList: renders renameInput.View() as modal content when modal == modalRename
  - `/Users/leeovery/Code/portal/internal/tui/model.go:974-992` - updateModal dispatches modalRename to updateRenameModal
  - `/Users/leeovery/Code/portal/internal/tui/model.go:356` - Help bar keybinding: key "r", help "rename"
- Notes: Implementation is clean and matches the task spec precisely. Lowercase r (not uppercase R) is used correctly. The textinput.Model and renameTarget fields are on the Model struct (lines 125-126). The modal overlay integrates with the shared renderListWithModal helper and modal system from Task 3.

TESTS:
- Status: Adequate
- Coverage:
  - "r opens rename modal with pre-populated session name" (line 1796) -- verifies prompt text, border styling, and pre-populated name
  - "enter in rename modal renames session and refreshes" (line 1825) -- clears input, types new name, verifies cmd returns SessionsMsg, verifies renamer called with correct old/new
  - "empty rename input is rejected on enter" (line 1868) -- clears input, presses enter, verifies nil cmd, renamer not called, modal stays open
  - "Esc dismisses rename modal without renaming" (line 1904) -- verifies modal dismissed, session still in list, renamer not called
  - "rename to same name is allowed" (line 1934) -- presses enter without changing pre-filled name, verifies renamer called with same old/new
  - "rename error triggers refresh" (line 1969) -- uses error-returning renamer, verifies SessionsMsg.Err is non-nil
  - "r on empty list is no-op" (line 2001) -- verifies no modal shown
  - "r without renamer configured is no-op" (line 2018) -- uses model without renamer, verifies no modal
  - "session list refreshes after successful rename" (line 2034) -- full end-to-end: rename, execute cmd, feed result back, verify new name in view
  - Additional coverage in progressive-back tests (line 4372): "Esc during rename modal dismisses modal" -- verifies Esc does not quit, modal dismissed, renamer not called
  - Ctrl+C during rename modal test (line 4554) -- verifies force-quit works even with rename modal active
  - "Esc during rename then Esc again quits TUI" (line 4594) -- two-step progressive back
- Notes: All 8 planned tests are present, plus 4 additional tests covering progressive-back/Ctrl+C interactions. Tests verify behavior through View() output and mock assertions rather than internal state -- good practice. No over-testing observed; each test covers a distinct scenario.

CODE QUALITY:
- Project conventions: Followed. Uses functional options pattern (WithRenamer), table-style mock structs, and Go naming conventions.
- SOLID principles: Good. SessionRenamer interface is minimal (single method). handleRenameKey and updateRenameModal have clear single responsibilities. Modal dispatch in updateModal follows open/closed principle (adding a new modal type requires only a new case).
- Complexity: Low. handleRenameKey has 2 guard clauses then straight-line setup. updateRenameModal has a clean switch on key type with 3 cases (Enter, Esc, delegate). renameAndRefresh is a simple 2-step async operation.
- Modern idioms: Yes. Uses bubbles/textinput correctly (New, SetValue, Focus, Update pattern). Uses strings.TrimSpace for input validation. Error wrapping with %w in renameAndRefresh.
- Readability: Good. Method names are self-documenting (handleRenameKey, updateRenameModal, renameAndRefresh). Comments explain non-obvious flow (modal routing, delegation to textinput).
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
