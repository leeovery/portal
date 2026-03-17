TASK: Modal Overlay System and Kill Confirmation (tick-b29c05)

ACCEPTANCE CRITERIA:
- [x] k (lowercase) on a selected session opens the kill confirmation modal
- [x] Modal displays "Kill {name}? (y/n)" in a styled overlay
- [x] y confirms kill, refreshes session list, and dismisses modal
- [x] n dismisses the modal without killing
- [x] Esc dismisses the modal without killing
- [x] All other keys are ignored while modal is active
- [x] After killing the last session, the list shows empty state
- [x] k on an empty list is a no-op (no selected item)
- [x] Kill error triggers a session list refresh (via SessionsMsg error path)
- [x] renderModal overlays content over the list view string

STATUS: Complete

SPEC CONTEXT: The spec requires "a single reusable modal overlay pattern" using lipgloss.Place() to position styled content over the list output in View(). All key input routes to the modal while active. Esc always dismisses. Kill confirmation is "Kill {name}? (y/n)". After confirm: kill via tmux, fetch fresh session list, call SetItems(). The spec uses lowercase k for kill.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/tui/modal.go:1-86 -- modalState type, renderModal function, renderListWithModal helper, modalStyle
  - /Users/leeovery/Code/portal/internal/tui/model.go:123 -- modal field on Model struct
  - /Users/leeovery/Code/portal/internal/tui/model.go:961-972 -- handleKillKey() opens modal
  - /Users/leeovery/Code/portal/internal/tui/model.go:974-992 -- updateModal() dispatches to correct modal handler
  - /Users/leeovery/Code/portal/internal/tui/model.go:994-1014 -- updateKillConfirmModal() handles y/n/Esc
  - /Users/leeovery/Code/portal/internal/tui/model.go:1016-1024 -- killAndRefresh() kills and fetches fresh sessions
  - /Users/leeovery/Code/portal/internal/tui/model.go:1181-1189 -- viewSessionList() renders modal overlay when active
- Notes:
  - The task description specifies using lipgloss.Place() for centering, but the implementation uses a custom ANSI-aware string compositing approach instead. This is actually a better design choice: lipgloss.Place() positions content in a blank canvas and does not overlay onto a background. The custom approach correctly composites the modal on top of the visible list background. Phase 4 Task 1 (tick-d2056e) exists in the plan as "Replace ANSI-unaware placeOverlay with lipgloss.Place" suggesting this was an iterative refinement area. The current implementation is ANSI-aware and correct.
  - The modalState enum (modal.go:14-20) already includes future states (modalRename, modalDeleteProject, modalEditProject) added by subsequent tasks. This is fine as this is the final codebase state.
  - The modal input routing pattern is clean: both updateSessionList() and updateProjectsPage() check `m.modal != modalNone` first and delegate to updateModal(), which dispatches to the correct handler.
  - The kill confirmation properly clears pendingKillName on both confirm and dismiss paths.

TESTS:
- Status: Adequate
- Coverage:
  - "k opens kill confirmation modal for selected session" -- verifies modal text and border styling (line 1355)
  - "y in confirmation mode triggers kill and refresh" -- verifies kill command execution and SessionsMsg return (line 1379)
  - "n in confirmation mode cancels" -- verifies dismiss without kill (line 1416)
  - "Esc in confirmation mode cancels" -- verifies dismiss without kill (line 1444)
  - "session list refreshes after kill" -- full roundtrip: k, y, lister update, feed SessionsMsg back (line 1471)
  - "cursor adjusts when last session killed" -- verifies bubbles/list cursor repositioning (line 1507)
  - "kill error returns error in SessionsMsg" -- verifies error wrapping in killAndRefresh (line 1550)
  - "kill error clears confirmation state via SessionsMsg" -- verifies modal cleared on error path (line 1583)
  - "killing last remaining session empties list" -- verifies 0 items after last kill (line 1610)
  - "other keys ignored during kill modal" -- sends q, k, r, p, x, down, up, enter; all ignored (line 1680)
  - "k on empty list is no-op" -- verifies no modal, nil command (line 1727)
  - renderModal tests in modal_test.go (line 11-124): overlay contains modal content, non-empty output, differs from plain list, border styling present, ANSI-aware centering test
- All 9 expected tests from the task are covered. The implementation adds a few extra useful tests (cursor adjustment, error clearing, constructor variants).
- Tests verify behavior not implementation details (checking view output and command results, not internal state).
- The ANSI-aware centering test in modal_test.go is particularly thorough, verifying that escape sequences in the background do not shift the overlay position.

CODE QUALITY:
- Project conventions: Followed -- uses table-driven test subtests, functional options, interfaces for dependencies, error wrapping with %w
- SOLID principles: Good
  - Single responsibility: modal.go handles rendering, model.go handles state transitions
  - Open/closed: modalState enum is extensible for new modal types
  - Dependency inversion: SessionKiller interface injected, not concrete type
- Complexity: Low -- clear separation between modal state transitions (updateKillConfirmModal) and rendering (viewSessionList). Each function has a single clear purpose.
- Modern idioms: Yes -- uses Go 1.21 builtin max(), string builder, lipgloss styling
- Readability: Good -- function names are descriptive (handleKillKey, updateKillConfirmModal, killAndRefresh), flow is easy to follow
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The task description specified lipgloss.Place() but the implementation uses a custom ANSI-aware overlay. This is documented as an intentional design evolution through Phase 4. The current approach is functionally superior for the overlay use case.
- The renderListWithModal helper (modal.go:31-46) has hardcoded fallback dimensions (80x24) when the list has zero width/height. This is a reasonable default but could be documented as to why it exists (pre-WindowSizeMsg calls).
