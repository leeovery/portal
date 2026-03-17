AGENT: architecture
FINDINGS:
- FINDING: Large Model struct accumulates all modal state as flat fields
  SEVERITY: low
  FILES: internal/tui/model.go:103-146
  DESCRIPTION: The Model struct holds 25+ fields, with edit-project modal state (editProject, editName, editAliases, editRemoved, editNewAlias, editFocus, editAliasCursor, editError) living as flat fields alongside kill-confirmation state (pendingKillName), rename state (renameInput, renameTarget), and delete state (pendingDeletePath, pendingDeleteName). Each modal's state is logically independent but shares the same lifetime and namespace. This is not a bug today -- the modal state machine (modalState enum + updateModal dispatcher) correctly routes to the right handler. But as modals grow or new ones are added, the risk of accidental cross-contamination increases. Currently proportional to the feature set, but approaching the threshold where grouping modal state into sub-structs (e.g., editModalState, killModalState) would improve clarity.
  RECOMMENDATION: No immediate action required. If a future cycle adds more modal types or fields, extract each modal's state into a dedicated sub-struct within Model. The existing modalState enum + dispatcher pattern is sound and would compose well with sub-structs.

- FINDING: Exported test-only accessors bloating the public API surface
  SEVERITY: low
  FILES: internal/tui/model.go:159-228
  DESCRIPTION: The Model type exports 14 accessor methods annotated "for testing" (SessionListItems, SessionListTitle, SessionListSize, SessionListFilterState, SessionListVisibleItems, SessionListFilterValue, SetSessionListFilter, ProjectListFilterValue, ProjectListItems, ProjectListSize, ProjectListFilterState, ProjectListVisibleItems, SetProjectListFilter, ActivePage). These exist because tests are in tui_test (external test package) and need to inspect internal state. While each is small, they collectively double the public method count and expose implementation details (e.g., list.FilterState). The internal/tui package is not consumed by external callers outside this project, so the blast radius is contained, but these methods create a maintenance surface: any refactor to the underlying list models requires updating both the accessors and the tests.
  RECOMMENDATION: This is acceptable as-is given the internal package boundary. If the accessor count continues growing, consider moving some tests into the tui package (internal test file) to access fields directly, or adding a single TestModel type that wraps Model and provides the accessors.

SUMMARY: The C3 edit-project wiring finding has been resolved. The architecture is sound: clean two-page model with modal overlay pattern, well-defined interface boundaries between tui/ui/cmd, proper functional options for dependency injection, and good seam quality between the file browser (internal/ui) and the TUI model (internal/tui). Two low-severity observations noted for future awareness but neither requires action.
