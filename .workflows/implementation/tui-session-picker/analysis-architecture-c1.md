AGENT: architecture
FINDINGS:
- FINDING: Duplicated modal dispatch functions should be unified
  SEVERITY: medium
  FILES: internal/tui/model.go:704, internal/tui/model.go:980
  DESCRIPTION: `updateModal` (sessions page) and `updateProjectModal` (projects page) are structurally identical -- both check Ctrl+C, then switch on modal state to delegate. Having two dispatch points means a new modal type requires edits in two places, and the Ctrl+C guard is duplicated. Since `modalState` is a single enum shared across both pages, there is no page-specific reason for separate dispatch.
  RECOMMENDATION: Merge into a single `updateModal` method that handles all modal states (kill, rename, deleteProject, editProject). Both `updateSessionList` and `updateProjectsPage` already check `m.modal != modalNone` before delegating -- they can both call the same method.

- FINDING: Model struct accumulates all modal state as flat fields
  SEVERITY: medium
  FILES: internal/tui/model.go:103-146
  DESCRIPTION: The Model struct carries 15+ fields for modal state (pendingKillName, renameInput, renameTarget, pendingDeletePath, pendingDeleteName, editProject, editName, editAliases, editRemoved, editNewAlias, editFocus, editAliasCursor, editError). These fields are only meaningful when their corresponding modal is active, but they are always present on the struct. This makes it easy to accidentally read stale state from a previous modal activation and makes it harder to reason about invariants. The field count will grow with each new modal type.
  RECOMMENDATION: Group modal-specific state into sub-structs (e.g., `killModalState`, `renameModalState`, `editProjectState`) and store only the active one, either via an interface or a pointer that is nil when the modal is inactive. This contains modal state lifetime and prevents stale-field bugs.

- FINDING: Overly broad public API surface on Model -- test-only accessors exported
  SEVERITY: low
  FILES: internal/tui/model.go:159-224
  DESCRIPTION: Model exports roughly 15 accessor methods documented "for testing" (SessionListItems, SessionListTitle, SessionListSize, SessionListFilterState, SessionListVisibleItems, SessionListFilterValue, SetSessionListFilter, ProjectListFilterValue, ProjectListItems, ProjectListSize, ProjectListFilterState, ProjectListVisibleItems, SetProjectListFilter, etc.). These inflate the public API surface of the `tui` package. Since the test file is in `tui_test` (external test package), these accessors are necessary for the current test approach, but they expose internal list state to any caller.
  RECOMMENDATION: Consider moving detailed list-state tests into an `_internal_test.go` file (package `tui`, not `tui_test`) where fields are directly accessible, keeping the exported surface limited to what `cmd/open.go` actually needs: `Selected()`, `InitialFilter()`, `CommandPending()`, `InsideTmux()`, `CurrentSession()`, `SessionListTitle()`, `CWD()`, `ActivePage()`, `Command()`. Alternatively, accept the trade-off if external-package testing is a deliberate project convention.

- FINDING: placeOverlay is ANSI-unaware and will misalign with styled content
  SEVERITY: medium
  FILES: internal/tui/modal.go:37-67
  DESCRIPTION: `placeOverlay` operates on raw runes, treating each rune as one column. But lipgloss-styled content contains ANSI escape sequences that are zero-width on screen but occupy runes in the string. When the background (`listView`) contains lipgloss-styled text (bold session names, colored attached badges), the overlay will miscount column positions and produce visual misalignment. The modal border itself is styled via `modalStyle`, but the background lines it is composited onto are all styled. This means the overlay will shift right or left depending on how many ANSI codes appear on each background line.
  RECOMMENDATION: Either use lipgloss's own `lipgloss.Place()` for centering (which is ANSI-aware), or strip ANSI sequences from background lines when computing offsets and re-inject them after compositing. The specification explicitly mentions using `lipgloss.Place()` for modal positioning.

SUMMARY: The implementation composes well overall -- clean interface boundaries, proper use of functional options, and thorough test coverage. The main structural concerns are: the custom overlay function is ANSI-unaware and will produce visual artifacts with styled backgrounds; the two modal dispatch functions should be unified to prevent divergence; and the flat modal-state fields on Model will scale poorly as modal types grow.
