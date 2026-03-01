TASK: Sessions Page with bubbles/list Core (tick-c64e34)

ACCEPTANCE CRITERIA:
- Sessions page renders using bubbles/list.Model.View()
- SessionsMsg populates the list via SetItems()
- Inside-tmux mode excludes current session from items and sets title to "Sessions (current: {name})"
- Enter on a session sets Selected() and quits
- q and Ctrl+C quit the TUI
- SessionsMsg with error triggers quit
- tea.WindowSizeMsg updates list dimensions
- Empty session list shows the list's built-in empty state
- Help bar shows session-specific keybindings

STATUS: Complete

SPEC CONTEXT: The spec defines a two-page architecture with sessions and projects as equal peers using bubbles/list. The Sessions page replaces the hand-rolled session list. Inside-tmux mode excludes the current session from items and displays it in the list title: "Sessions (current: {session-name})". Enter attaches to the selected session and exits the TUI.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:103-146` — Model struct with `sessionList list.Model` field, no old `cursor int` or `loaded bool`
  - `internal/tui/model.go:364-375` — `newSessionList()` creates `list.New()` with `SessionDelegate{}`, sets title "Sessions", disables quit keybindings, disables status bar, enables filtering, sets additional help keys
  - `internal/tui/model.go:421-431` — `New()` constructor with functional options
  - `internal/tui/model.go:434-446` — `NewModelWithSessions()` test helper pre-populates list via `ToListItems()`
  - `internal/tui/model.go:448-460` — `filteredSessions()` excludes current session when inside tmux
  - `internal/tui/model.go:529-542` — `Init()` returns session fetch command
  - `internal/tui/model.go:545-599` — `Update()` handles `tea.WindowSizeMsg` (sets size on both lists), `SessionsMsg` (converts via `ToListItems`, filters, calls `SetItems`, sets title if inside tmux), delegates to `updateSessionList`
  - `internal/tui/model.go:913-958` — `updateSessionList()` handles enter (selects session + quit), q (quit), Ctrl+C (quit), Esc (progressive back), delegates to list for cursor nav
  - `internal/tui/model.go:1091-1098` — `handleSessionListEnter()` sets `m.selected` from `SelectedItem()` cast to `SessionItem`
  - `internal/tui/model.go:1181-1189` — `viewSessionList()` returns `renderListWithModal(m.sessionList, modalContent)`
  - `internal/tui/model.go:281-288` — `WithInsideTmux()` sets insideTmux, filters sessions, updates title
- Notes: All acceptance criteria are addressed in the implementation. The old `cursor int` and `loaded bool` fields have been removed. The `viewSessionList()` method delegates to `list.Model.View()` via `renderListWithModal()`. The `Init()` keeps the existing `SessionsMsg` fetch command.

TESTS:
- Status: Adequate
- Coverage:
  - "SessionsMsg populates list items" — `TestUpdate` line 254, `TestSessionListWithBubblesList` line 2724: verifies sessions convert to list items with correct names
  - "SessionsMsg with error triggers quit" — `TestUpdate` line 278, `TestSessionListWithBubblesList` line 2748: verifies error produces tea.QuitMsg
  - "enter selects session and quits" — `TestEnterSelection` line 485, `TestSessionListWithBubblesList` line 2762: verifies Selected() set and tea.QuitMsg returned
  - "q key triggers quit" — `TestQuitHandling` line 391, `TestSessionListWithBubblesList` line 2784
  - "Ctrl+C triggers quit" — `TestQuitHandling` line 391, `TestSessionListWithBubblesList` line 2799
  - "inside tmux excludes current session from list" — `TestInsideTmuxSessionExclusion` line 1236, `TestSessionListWithBubblesList` line 2814
  - "inside tmux sets title with current session name" — `TestInsideTmuxSessionExclusion` line 1264, `TestSessionListWithBubblesList` line 2838
  - "empty session list shows empty state" — `TestEmptyState` line 1161, `TestSessionListWithBubblesList` line 2871, `TestSessionsPageEmptyText` line 4266 (verifies "No sessions running" text)
  - "WindowSizeMsg updates list dimensions" — `TestSessionListWithBubblesList` line 2886: verifies width and height via SessionListSize()
  - "inside tmux with only current session shows empty list" — `TestInsideTmuxSessionExclusion` line 1296, `TestSessionListWithBubblesList` line 2903
  - "sessions page renders using list View" — `TestSessionListWithBubblesList` line 2919: verifies session names appear in View() output
  - "help bar shows session-specific keybindings" — `TestSessionListHelpBar` line 1748: verifies attach, rename, kill, projects, new in cwd, filter appear in view
  - Additional: `TestView` line 17 covers rendering (names, window counts, attached badge, cursor, pluralization, ordering); `TestKeyboardNavigation` line 294 covers arrow key navigation; `TestEnterSelection` covers edge cases (no sessions, navigation + enter)
- Notes: Tests are well-structured with table-driven patterns where appropriate. There is some duplication between `TestUpdate`/`TestEmptyState`/`TestInsideTmuxSessionExclusion` and `TestSessionListWithBubblesList` (the latter re-tests several of the same scenarios), but each duplicate tests through a slightly different code path (e.g., `New()` vs `NewModelWithSessions()`, `WithInsideTmux` before vs after `SessionsMsg`). This is borderline over-tested but not egregiously so — the duplication provides value by exercising both test helper paths.

CODE QUALITY:
- Project conventions: Followed — uses functional options pattern, table-driven tests, explicit error handling, proper Go doc comments on exported types
- SOLID principles: Good — `SessionLister` interface for dependency inversion, single-responsibility separation of `filteredSessions()`, `newSessionList()`, `handleSessionListEnter()`
- Complexity: Low — the `Update()` method has a reasonable type-switch structure; `updateSessionList()` cleanly delegates to the list for unhandled messages
- Modern idioms: Yes — uses `bubbles/list` correctly with `SetItems()`, `SelectedItem()`, custom delegate, `DisableQuitKeybindings()`, `AdditionalShortHelpKeys`/`AdditionalFullHelpKeys`
- Readability: Good — clear method names, consistent patterns, helper methods extract behavior well
- Issues: None significant

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- There is moderate test duplication between `TestUpdate`/`TestEmptyState`/`TestInsideTmuxSessionExclusion` and `TestSessionListWithBubblesList`. The `TestSessionListWithBubblesList` function re-covers many of the same scenarios already tested in the earlier test functions. Consider consolidating to reduce maintenance burden, though the duplication is not harmful.
- The `TestView` struct has a `cursor int` field (line 22) that appears vestigial — it is declared but never used to set up test state (NewModelWithSessions always starts with cursor at 0). This could be cleaned up.
