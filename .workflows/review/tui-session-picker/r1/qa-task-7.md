TASK: Page-Switching Skeleton and Help Bar (tick-fe1e6f)

ACCEPTANCE CRITERIA:
- [ ] p on sessions page switches to projects page
- [ ] s on projects page switches to sessions page
- [ ] x toggles between pages from either page
- [ ] Switching pages preserves list state (cursor position, filter state) on the source page
- [ ] Sessions page help bar includes [p] projects
- [ ] Projects stub page help bar includes [s] sessions
- [ ] Empty sessions page shows "No sessions running"
- [ ] Empty projects stub shows "No saved projects"

STATUS: Complete

SPEC CONTEXT: The spec defines a two-page architecture where Sessions and Projects are equal peers. Navigation uses p (go to projects, shown in Sessions help bar), s (go to sessions, shown in Projects help bar), and x (toggle, undocumented power-user shortcut -- not shown in help bars). Empty pages are always reachable and display bubbles/list built-in empty messages. The old viewState enum is replaced by a page type. Each page is a full bubbles/list.Model instance.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:17-27` -- `page` type with `PageSessions`, `PageProjects`, `pageFileBrowser` constants
  - `/Users/leeovery/Code/portal/internal/tui/model.go:117` -- `activePage page` field on Model
  - `/Users/leeovery/Code/portal/internal/tui/model.go:118` -- `projectList list.Model` field on Model
  - `/Users/leeovery/Code/portal/internal/tui/model.go:402-412` -- `newProjectList()` creates stub project list with nil items, ProjectDelegate, and "s" in help keys
  - `/Users/leeovery/Code/portal/internal/tui/model.go:944-949` -- `p` and `x` keys on sessions page set `activePage = PageProjects`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:647-655` -- `s` and `x` keys on projects page set `activePage = PageSessions`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1101-1119` -- `View()` dispatches based on `activePage`
  - `/Users/leeovery/Code/portal/internal/tui/model.go:592-599` -- `Update()` dispatches to active page's update logic
  - `/Users/leeovery/Code/portal/internal/tui/model.go:353-361` -- `sessionHelpKeys()` includes `p` for "projects"
  - `/Users/leeovery/Code/portal/internal/tui/model.go:377-388` -- `projectHelpKeys()` includes `s` for "sessions"
  - `/Users/leeovery/Code/portal/internal/tui/model.go:373` -- `SetStatusBarItemName("session", "sessions running")` for "No sessions running" empty text
  - `/Users/leeovery/Code/portal/internal/tui/model.go:410` -- `SetStatusBarItemName("project", "saved projects")` for "No saved projects" empty text
- Notes:
  - Old `viewState` enum is fully removed -- no traces of `viewSessionList`, `viewProjectPicker` as enum values
  - The `x` key is correctly absent from both `sessionHelpKeys` and `projectHelpKeys`, matching the spec's "undocumented" requirement
  - Page switching is a simple assignment (no teardown/rebuild), which naturally preserves list state

TESTS:
- Status: Adequate
- Coverage:
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:3272` -- "p on sessions page switches to projects page"
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:3294` -- "s on projects page switches to sessions page"
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:3313` -- "x toggles from sessions to projects"
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:3329` -- "x toggles from projects to sessions"
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:3348` -- "switching to projects and back preserves session list state" (verifies cursor position via Enter/Selected)
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:3384` -- "switching to empty stub projects page shows empty message" (checks "No saved projects")
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:4267` -- "empty sessions page shows no sessions running" (checks "No sessions running")
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:4276` -- "projects stub help bar includes s for sessions"
  - `/Users/leeovery/Code/portal/internal/tui/model_test.go:4295` -- "sessions page help bar includes p for projects"
- Notes:
  - All 6 required tests from the task are present, plus 3 additional tests for empty text and help bar assertions
  - Tests verify behavior (page state, rendered output) rather than implementation details
  - The cursor preservation test at line 3348 is well-structured: moves cursor, switches pages, switches back, then verifies cursor via Enter selection
  - No over-testing; each test covers a distinct behavior

CODE QUALITY:
- Project conventions: Followed -- table-driven test style with subtests, functional options pattern, idiomatic Go
- SOLID principles: Good -- page switching is cleanly separated in Update dispatch; each page has its own update function; help keys are configured via functions returning slices (easy to change per page)
- Complexity: Low -- page switching is a simple activePage assignment; View/Update dispatch via switch is straightforward
- Modern idioms: Yes -- uses Go iota for page enum, type-safe page constants, receiver methods
- Readability: Good -- page type and constants are well-documented with comments; the dispatch in Update() and View() is clear
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The `page` type is unexported but `PageSessions` and `PageProjects` are exported constants. This is fine for test access but slightly unusual. The `pageFileBrowser` is kept unexported which is consistent (it is not a top-level page).
