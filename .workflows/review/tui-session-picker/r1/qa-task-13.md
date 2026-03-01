TASK: Project Edit Modal (tick-fd7b3f)

ACCEPTANCE CRITERIA:
- e on a selected project opens the edit modal with current name and aliases
- Tab switches focus between name field and alias section
- Enter saves name change and alias modifications, then refreshes list
- Esc cancels edit without saving changes
- Empty name is rejected with error message
- Alias collision shows error message
- x on an existing alias marks it for removal
- New alias text is entered in the "Add:" input
- e with no editor configured is a no-op
- e on empty list is a no-op
- Alias removal is committed on save (not immediately)

STATUS: Complete

SPEC CONTEXT: The specification defines a project edit modal as the most complex modal type: "e triggers a modal overlay with the project's name field, alias list, and full edit controls; Enter saves; Esc cancels." The modal system uses lipgloss.Place() for overlay positioning. In command-pending mode, e is not registered (disabled).

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `internal/tui/model.go:57-67` -- ProjectEditor and AliasEditor interfaces
  - `internal/tui/model.go:69-75` -- editField type and constants (editFieldName, editFieldAliases)
  - `internal/tui/model.go:138-145` -- Edit project modal state fields on Model
  - `internal/tui/model.go:330-342` -- WithProjectEditor and WithAliasEditor options
  - `internal/tui/model.go:663-667` -- e key handler in updateProjectsPage (with command-pending guard)
  - `internal/tui/model.go:738-771` -- handleEditProjectKey (opens modal, loads aliases, populates state)
  - `internal/tui/model.go:773-847` -- updateEditProjectModal (Tab, Esc, Enter, Backspace, Up/Down, rune dispatch)
  - `internal/tui/model.go:849-893` -- handleEditProjectConfirm (validation, rename, alias CRUD, save, refresh)
  - `internal/tui/model.go:1134-1178` -- renderEditProjectContent (modal content builder)
  - `internal/tui/modal.go:19` -- modalEditProject constant in modalState enum
  - `internal/tui/modal.go:31-46` -- renderListWithModal shared helper
  - `internal/tui/model.go:987-988` -- updateModal dispatches to updateEditProjectModal
- Notes: Implementation matches all acceptance criteria. The edit modal correctly populates from the selected project, loads matching aliases by path, supports Tab-based field switching, validates empty name on Enter, checks alias collisions against loaded data, defers alias removal to save time, and refreshes the project list after successful save.

TESTS:
- Status: Adequate
- Coverage:
  - "e opens edit modal with project name and aliases" (line 4796) -- verifies Name field, project name, Aliases section, alias display, border styling
  - "Tab switches focus between name and aliases" (line 4835) -- verifies typing goes to name initially, switches to Add input after Tab, returns to name after second Tab
  - "Enter saves name change and refreshes list" (line 4880) -- verifies Rename called with correct args, returns refresh command, command produces ProjectsLoadedMsg
  - "Esc cancels edit without saving" (line 4932) -- verifies Rename not called, nil command, modal dismissed, original name preserved
  - "empty name rejected with error on Enter" (line 4972) -- verifies error message, modal stays open, Rename not called, nil command
  - "alias collision shows error message" (line 5013) -- verifies collision error with existing alias for different project, modal stays open
  - "x removes alias from list in edit mode" (line 5053) -- verifies modal stays open, aliases section visible after removal
  - "new alias is added on save" (line 5088) -- verifies Set called with correct name/path, Save called
  - "alias removal is committed on save" (line 5132) -- verifies Delete called for removed alias, Save called
  - "e with no editor configured is no-op" (line 5174) -- verifies nil command, modal not opened
  - "e on empty project list is no-op" (line 5195) -- verifies nil command, modal not opened
  - "pressing e in command-pending mode does nothing" (line 2561) -- verifies modal not opened in command-pending mode
  - Esc progressive back test (line ~6790) -- verifies Esc in edit modal dismisses modal without quitting
- Notes: All 11 planned tests are present, plus additional tests in other test groups that exercise the edit modal pathway (command-pending guard, Esc progressive back). Tests verify behavior rather than implementation details. Mock infrastructure (mockProjectEditor, mockAliasEditor) is well-structured with call tracking. The "x removes alias" test could be slightly more specific about which alias was removed (it only checks the section is still visible), but the "alias removal is committed on save" test covers the actual Delete call verification, so this is acceptable.

CODE QUALITY:
- Project conventions: Followed. Functional options pattern for DI (WithProjectEditor, WithAliasEditor). Interfaces are small and focused (ProjectEditor has 1 method, AliasEditor has 4 related methods). Table-driven test patterns consistent with the rest of the codebase.
- SOLID principles: Good. Single responsibility -- handleEditProjectKey opens the modal, updateEditProjectModal handles input routing, handleEditProjectConfirm handles save logic. Dependency inversion via interfaces. Interface segregation -- ProjectEditor and AliasEditor are separate despite being used together.
- Complexity: Acceptable. updateEditProjectModal has moderate cyclomatic complexity (handling Tab, Esc, Enter, Backspace, Up, Down, Runes with sub-conditions), but the switch-case structure keeps code paths clear. The confirm handler has a linear flow: validate name -> rename if changed -> remove aliases -> add new alias with collision check -> save -> refresh.
- Modern idioms: Yes. Uses Go 1.21+ range-over-int (`for range len("portal")`), proper error handling throughout.
- Readability: Good. Clear function names, comment headers on sections, the modal content renderer uses a strings.Builder with descriptive format strings.
- Issues: None significant.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The alias list order in handleEditProjectKey (line 761-766) iterates over a map, so alias display order is non-deterministic. For a small alias count this is cosmetically acceptable, but sorting the matching aliases alphabetically would provide consistent display. Very minor.
- The "x removes alias" test (line 5053) verifies the modal stays open and aliases section is visible, but does not explicitly verify which alias was removed from the displayed list. The companion "alias removal is committed on save" test covers the correctness of the Delete call, so this is not a gap in coverage -- just a slightly weaker individual test assertion.
