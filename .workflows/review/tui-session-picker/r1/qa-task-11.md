TASK: Projects Page with bubbles/list Core (tick-9184b3)

ACCEPTANCE CRITERIA:
- [ ] Projects page renders using bubbles/list.Model.View()
- [ ] ProjectsLoadedMsg populates the list via SetItems()
- [ ] createSession(path) method created; createSessionInCWD delegates to it
- [ ] enter on a project creates a session in the project directory and quits
- [ ] n on the projects page creates a session in cwd and quits
- [ ] q and Ctrl+C quit the TUI from projects page
- [ ] Empty project list shows "No saved projects" built-in empty message
- [ ] Project load error leaves list empty (does not crash)
- [ ] Session creation error is handled gracefully
- [ ] Help bar shows projects-specific keybindings
- [ ] tea.WindowSizeMsg updates projects list dimensions

STATUS: Complete

SPEC CONTEXT: The spec defines a two-page architecture (Sessions and Projects) using bubbles/list. The Projects page should display all saved projects via a custom ItemDelegate showing project name and path. Enter creates a new session in the selected project directory. The help bar should show: [enter] new session [e] edit [d] delete [s] sessions [n] new in cwd [b] browse [/] filter [q] quit. Empty pages show bubbles/list built-in empty message ("No saved projects").

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:96-100` -- ProjectsLoadedMsg struct
  - `/Users/leeovery/Code/portal/internal/tui/model.go:401-412` -- newProjectList() with correct configuration (title "Projects", disabled quit keybindings, project help keys, status bar item name "saved projects")
  - `/Users/leeovery/Code/portal/internal/tui/model.go:502-512` -- loadProjects() tea.Cmd calling CleanStale() then List()
  - `/Users/leeovery/Code/portal/internal/tui/model.go:529-542` -- Init() fires loadProjects in batch with fetchSessions
  - `/Users/leeovery/Code/portal/internal/tui/model.go:575-582` -- ProjectsLoadedMsg handler: converts to list items via ProjectsToListItems, calls SetItems; on error leaves list empty
  - `/Users/leeovery/Code/portal/internal/tui/model.go:602-610` -- createSession(dir string) method
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1087-1089` -- createSessionInCWD delegates to createSession(m.cwd)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:622-678` -- updateProjectsPage handles enter, n, q, Ctrl+C, and other keys
  - `/Users/leeovery/Code/portal/internal/tui/model.go:689-698` -- handleProjectEnter selects project and calls createSession
  - `/Users/leeovery/Code/portal/internal/tui/model.go:547-549` -- WindowSizeMsg updates both session and project list dimensions
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1101-1119` -- View() renders projectList.View() when on PageProjects
  - `/Users/leeovery/Code/portal/internal/tui/model.go:377-398` -- projectHelpKeys with enter/s/e/d/b/n/q
- Notes: All acceptance criteria items are fully implemented. createSessionInCWD properly delegates to createSession. The newProjectList() function sets StatusBarItemName to "project"/"saved projects" which produces the "No saved projects" empty message via bubbles/list internals. The project list empty text is not explicitly set via a separate method but is derived from the status bar item name plural -- the test confirms "No saved projects" appears in the view.

TESTS:
- Status: Adequate
- Coverage:
  - "ProjectsLoadedMsg populates project list items" -- line 3402: verifies 2 items set, correct project names
  - "createSession creates session at given path" -- line 3441: verifies enter on project triggers CreateFromDir with correct path, returns SessionCreatedMsg
  - "createSessionInCWD delegates to createSession with cwd" -- line 3482: verifies n key calls CreateFromDir with cwd path
  - "enter on project creates session and quits" -- line 3511: verifies full flow: enter -> SessionCreatedMsg -> selected set -> quit
  - "n on projects page creates session in cwd" -- line 3570: verifies n key on projects page, full flow including quit
  - "q key quits from projects page" -- line 3625: verifies q produces tea.QuitMsg
  - "Ctrl+C quits from projects page" -- line 3646: verifies Ctrl+C produces tea.QuitMsg
  - "empty project list shows empty message" -- line 3667: verifies "No saved projects" in view
  - "project load error leaves list empty" -- line 3683: verifies 0 items after error, view still renders
  - "session creation error handled gracefully" -- line 3714: verifies error does not crash, selected remains empty, view still renders
  - "WindowSizeMsg updates project list dimensions" -- line 3761: verifies width=120, height=40
  - "projects help bar shows correct keybindings" -- line 3778: verifies "new session", "sessions", "new in cwd" in view
  - "Init fires loadProjects command" -- line 3813: verifies Init returns non-nil command
  - "projects page renders using list View" -- line 3838: verifies project names appear in rendered view
- Notes: All 11 required tests from the task spec are present plus 3 additional tests (Ctrl+C, Init fires loadProjects, renders using list View). Tests are well-structured, testing both the command execution and the full message round-trip where appropriate. Test coverage is thorough without being redundant.

CODE QUALITY:
- Project conventions: Followed -- uses functional options pattern, Go interfaces for testability, table-driven style subtests
- SOLID principles: Good -- ProjectStore interface is minimal (List, CleanStale, Remove), createSession extracted as reusable method, single responsibility maintained between model.go, project_item.go, modal.go
- Complexity: Low -- updateProjectsPage has clear switch-case routing, no deeply nested logic
- Modern idioms: Yes -- uses value receivers on Model (Bubble Tea convention), proper error handling, cmd composition with tea.Batch
- Readability: Good -- helper functions (handleProjectEnter, handleNewInCWD, selectedProjectItem) keep update methods clean and intention-revealing
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The "Init fires loadProjects command" test (line 3813) only verifies the command is non-nil but does not verify the batch actually includes a loadProjects command. This is noted in the test itself as a limitation of batch command inspection. Acceptable given the integration is tested via other paths.
