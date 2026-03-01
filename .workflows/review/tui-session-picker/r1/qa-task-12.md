TASK: Delete Confirmation Modal for Projects (tick-5509f1)

ACCEPTANCE CRITERIA:
- d on a selected project opens the delete confirmation modal
- Modal displays "Delete {name}? (y/n)" in a styled overlay
- y confirms deletion, removes project from store, and refreshes list
- n dismisses the modal without deleting
- Esc dismisses the modal without deleting
- All other keys are ignored while modal is active
- After deleting the last project, the list shows the empty state
- d on an empty list is a no-op
- Delete while filter is active works correctly

STATUS: Complete

SPEC CONTEXT: The specification defines a modal system where `d` triggers a delete confirmation modal overlay for the selected project. On confirm, the project is removed from config and the list is refreshed. The modal uses the same `lipgloss.Place()` overlay pattern established for kill confirmation in Phase 1. The projects page help bar includes `[d] delete`. In command-pending mode, `d` is not registered (pressing it does nothing).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/modal.go:18` -- `modalDeleteProject` constant in modalState enum
  - `internal/tui/model.go:127-128` -- `pendingDeletePath` and `pendingDeleteName` fields on Model
  - `internal/tui/model.go:658-662` -- `d` key handler in `updateProjectsPage`, guards against command-pending mode
  - `internal/tui/model.go:700-712` -- `handleDeleteProjectKey()` sets modal state, stores pending project path/name
  - `internal/tui/model.go:714-736` -- `updateDeleteProjectModal()` handles y/n/Esc and ignores other keys
  - `internal/tui/model.go:516-525` -- `deleteAndRefreshProjects()` calls `Remove`, then `CleanStale`+`List` to refresh
  - `internal/tui/model.go:1122-1131` -- `viewProjectList()` renders modal with "Delete {name}? (y/n)" text
  - `internal/tui/model.go:974-992` -- `updateModal()` routes `modalDeleteProject` to `updateDeleteProjectModal`
- Notes: Implementation matches all acceptance criteria. The `selectedProjectItem()` helper (model.go:613-620) naturally handles the empty list case by returning `false` when no item is selected, which causes `handleDeleteProjectKey` to return early as a no-op. The `projectStore == nil` guard at model.go:705 adds defensive safety. The `deleteAndRefreshProjects` command correctly wraps errors with context via `fmt.Errorf`.

TESTS:
- Status: Adequate
- Coverage:
  - "d opens delete confirmation modal for selected project" (line 3872) -- verifies modal text and border styling
  - "y in delete modal removes project and refreshes list" (line 3908) -- verifies Remove called with correct path, ProjectsLoadedMsg returned
  - "n in delete modal dismisses without deleting" (line 3958) -- verifies modal cleared, project remains, Remove not called
  - "Esc in delete modal dismisses without deleting" (line 3997) -- same assertions as n test
  - "other keys ignored during delete modal" (line 4036) -- tests q, d, s, x, down, up, enter are all no-ops
  - "delete last remaining project shows empty state" (line 4093) -- verifies "No saved projects" message appears
  - "d on empty project list is no-op" (line 4135) -- verifies no modal and nil command
  - "delete error propagated via ProjectsLoadedMsg" (line 4161) -- verifies error wrapping when Remove fails
  - "delete while filter active removes the correct project" (line 4207) -- applies filter, verifies correct project targeted
  - Command-pending mode blocks d (line 2612) -- verified in the command-pending test suite
- Notes: All 8 tests specified in the plan are present, plus an additional error propagation test which is valuable. Tests verify behavior, not implementation details. The `mockProjectStore` (line 731) tracks `removeCalled`, `removedPath`, and `removeErr` to enable thorough verification. Test balance is good -- no redundant tests, each covers a distinct scenario.

CODE QUALITY:
- Project conventions: Followed. Table-driven subtests under a parent `TestDeleteProject` function. Functional options pattern for dependencies. Error wrapping with `%w`.
- SOLID principles: Good. Single responsibility maintained -- `handleDeleteProjectKey` sets up state, `updateDeleteProjectModal` handles modal input, `deleteAndRefreshProjects` handles the async operation. The modal routing through `updateModal` follows open/closed principle.
- Complexity: Low. The delete modal handler has simple y/n/Esc branches with a default no-op. No nested complexity.
- Modern idioms: Yes. Uses Bubble Tea command pattern correctly (returning `tea.Cmd` for async operations). Value receiver on Model for immutable update flow.
- Readability: Good. Function names are descriptive. The flow from `d` keypress through modal to confirmation/dismissal is easy to follow.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The `pendingDeletePath` and `pendingDeleteName` clearing is duplicated in both the y and n/Esc branches of `updateDeleteProjectModal` (lines 724-725 and 730-731). This is minor and arguably clearer than a deferred cleanup, but a single cleanup at the top of the dismiss path could reduce duplication.
