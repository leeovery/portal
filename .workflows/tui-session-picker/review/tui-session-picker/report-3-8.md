TASK: Guard evaluateDefaultPage against command-pending page selection (tick-a87527)

ACCEPTANCE CRITERIA:
- When commandPending is true, evaluateDefaultPage always sets activePage to PageProjects regardless of session list contents.
- Existing non-command-pending behavior is unchanged.

STATUS: Complete

SPEC CONTEXT: The specification states command-pending mode locks the TUI to the Projects page. The default page logic (sessions exist -> Sessions page; no sessions -> Projects page) must be overridden when command-pending is active. This task hardens evaluateDefaultPage so it never accidentally selects PageSessions in command-pending mode, even if a SessionsMsg arrives and populates the session list.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:468-500
- Notes: The implementation matches the task specification exactly. The method has an explicit command-pending branch at line 480 that unconditionally sets `m.activePage = PageProjects`, bypassing the `sessionList.Items()` check. Additionally, the readiness gate at lines 472-478 correctly handles command-pending mode by only requiring `projectsLoaded` (not `sessionsLoaded`), since `Init()` skips session fetching in command-pending mode. The doc comment at lines 462-467 accurately describes the behavior. The resulting structure matches the task's prescribed code shape.

TESTS:
- Status: Adequate
- Coverage:
  - "command-pending sets PageProjects even when sessionList has items" (model_test.go:5927-5958): Creates a model with WithCommand, sends a SessionsMsg with real sessions to populate the list, then sends ProjectsLoadedMsg. Verifies activePage is PageProjects. This directly tests the acceptance criterion -- sessions in the list are ignored.
  - "normal mode still defaults to PageSessions when sessions exist" (model_test.go:5960-5988): Confirms normal (non-command-pending) behavior is unchanged -- sessions present leads to PageSessions.
- Notes: Both acceptance criteria have direct, focused tests. The command-pending test properly exercises the exact edge case described in the task (SessionsMsg arrives and populates session list before evaluateDefaultPage makes its final determination). No over-testing observed.

CODE QUALITY:
- Project conventions: Followed -- table-driven-compatible test structure, idiomatic Go patterns.
- SOLID principles: Good -- evaluateDefaultPage has single responsibility (determine default page), the command-pending invariant is self-contained within the method rather than relying on Init not fetching sessions.
- Complexity: Low -- the branching logic is a simple if/else-if/else with clear intent.
- Modern idioms: Yes -- standard Go patterns throughout.
- Readability: Good -- the doc comment explains the three modes clearly, and the code structure makes the command-pending override immediately visible.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
