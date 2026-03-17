TASK: Default Page Selection on Launch (tick-2f0ec0)

ACCEPTANCE CRITERIA:
- When sessions exist (post inside-tmux filtering), TUI opens on Sessions page
- When no sessions exist but projects exist, TUI opens on Projects page
- When both are empty, TUI opens on Projects page
- Page switching (p/s/x) still works after default page selection
- Inside-tmux filtering applied before the default page decision
- Default page evaluated only after both SessionsMsg and ProjectsLoadedMsg received

STATUS: Complete

SPEC CONTEXT: The specification (Page Navigation & Defaults section) states: sessions exist -> Sessions page; no sessions, projects exist -> Projects page; both empty -> Projects page. Empty pages remain reachable via p/s navigation. The default page determines which list receives the initial filter (--filter flag). Command-pending mode always locks to Projects page.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:132-135 (tracking booleans: sessionsLoaded, projectsLoaded, defaultPageEvaluated)
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:462-500 (evaluateDefaultPage method)
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:529-542 (Init batches both fetches)
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:559-582 (SessionsMsg/ProjectsLoadedMsg handlers call evaluateDefaultPage)
- Notes: Implementation matches all acceptance criteria. The evaluateDefaultPage method:
  - Guards with defaultPageEvaluated bool to run only once (prevents overriding manual page switches)
  - Waits for both sessionsLoaded and projectsLoaded before executing
  - In command-pending mode, only waits for projectsLoaded (correct optimization since sessions aren't shown)
  - Checks sessionList.Items() (post inside-tmux filtering) for the page decision
  - Also applies initial filter to the default page and consumes it (clears initialFilter)
  - The filtering happens *before* the Items() check since SessionsMsg handler calls filteredSessions() and SetItems() before evaluateDefaultPage()

TESTS:
- Status: Adequate
- Coverage:
  - "defaults to Sessions page when sessions exist" (line 5597): Verifies sessions present -> PageSessions
  - "defaults to Projects page when no sessions exist but projects exist" (line 5628): Verifies no sessions -> PageProjects
  - "defaults to Projects page when both pages are empty" (line 5656): Verifies both empty -> PageProjects
  - "defaults to Projects page when all sessions filtered by inside-tmux exclusion" (line 5679): Verifies inside-tmux filtering applied before decision
  - "page switching works after defaulting to Sessions page" (line 5709): Tests p, s, x keys after Sessions default
  - "page switching works after defaulting to Projects page" (line 5760): Tests s, p, x keys after Projects default
  - "default page waits for both data sources before evaluating" (line 5859): Sessions first, then projects
  - "default page waits for both data sources before evaluating -- projects first" (line 5893): Projects first, then sessions
  - "evaluateDefaultPage only runs once and does not override manual page switch" (line 5809): Ensures subsequent SessionsMsg doesn't re-run evaluation
  - "command-pending sets PageProjects even when sessionList has items" (line 5927): Command-pending always Projects
  - "normal mode still defaults to PageSessions when sessions exist" (line 5960): Baseline normal mode test
- Notes: All 7 required tests from the task are present, plus 4 additional tests covering edge cases (run-once guard, reverse message order, command-pending override, normal mode baseline). Test coverage is thorough without being redundant -- each test verifies a distinct scenario. Tests properly use the message-passing approach to simulate Init() completion rather than calling Init() directly (which returns tea.Cmd that can't be easily inspected for batch contents).

CODE QUALITY:
- Project conventions: Followed. Uses functional options pattern for configuration. Table-driven-style subtests via t.Run. Proper Go naming conventions.
- SOLID principles: Good. evaluateDefaultPage has a single responsibility (determine default page + apply initial filter). The coupling of initial filter application with page evaluation is appropriate since the filter depends on which page is selected.
- Complexity: Low. evaluateDefaultPage is straightforward with clear early returns. The guard pattern (defaultPageEvaluated + loaded booleans) is simple and effective.
- Modern idioms: Yes. Uses Bubble Tea patterns correctly (tea.Batch for concurrent commands, message-based state tracking).
- Readability: Good. Well-documented function with clear comments explaining the logic flow. The three-boolean tracking (sessionsLoaded, projectsLoaded, defaultPageEvaluated) is easy to follow.
- Issues: None identified.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The Init() function in command-pending mode only loads projects (line 530-531), which means sessionsLoaded stays false. The evaluateDefaultPage method handles this correctly with a separate branch (line 472-475) that only checks projectsLoaded. This is good but the asymmetry could be documented inline for future maintainers.
