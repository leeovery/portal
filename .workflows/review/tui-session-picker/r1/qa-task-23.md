TASK: Initial Filter Applied to Default Page (tick-bd640d)

ACCEPTANCE CRITERIA:
- Initial filter applied to Sessions page when sessions exist
- Initial filter applied to Projects page when no sessions
- Initial filter applied to Projects page in command-pending mode
- Filter text visible in the bubbles/list filter bar
- Only matching items shown after filter applied
- Empty initial filter is a no-op
- Filter consumed after first application (not re-applied on refresh)
- Old initial filter code removed from SessionsMsg handler

STATUS: Complete

SPEC CONTEXT: The specification states that the initial filter (--filter flag) should be applied to the default page during initialization using SetFilterText() and SetFilterState(list.FilterApplied) on whichever page is the default (sessions if they exist, otherwise projects). Independent filters per page is the default bubbles/list behavior.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:462-500 (evaluateDefaultPage method)
- Notes: The implementation correctly:
  1. Waits for both sessionsLoaded and projectsLoaded before evaluating (or just projectsLoaded in command-pending mode) — lines 472-478
  2. Uses defaultPageEvaluated guard to ensure one-time execution — lines 469-471
  3. Determines active page: command-pending -> PageProjects, sessions exist -> PageSessions, else -> PageProjects — lines 480-486
  4. If initialFilter is non-empty, applies SetFilterText() and SetFilterState(list.FilterApplied) to the correct page's list — lines 488-498
  5. Consumes initialFilter by setting it to "" — line 499
  6. SessionsMsg handler (line 559-574) does NOT apply initial filter directly; it only calls evaluateDefaultPage()
  7. The old initial filter code that was in the SessionsMsg handler (from task 1-6) has been moved to evaluateDefaultPage as specified

TESTS:
- Status: Adequate
- Coverage: All 7 specified tests are present in TestInitialFilterAppliedToDefaultPage (lines 6966-7273):
  1. "initial filter applied to Sessions page when sessions exist" (line 6967) — verifies page=Sessions, filter state=FilterApplied, filter value="myapp", visible items filtered correctly, filter consumed
  2. "initial filter applied to Projects page when no sessions exist" (line 7022) — verifies page=Projects, filter applied to project list, visible items filtered, filter consumed
  3. "initial filter applied to Projects page in command-pending mode" (line 7074) — verifies command-pending mode Init() flow, filter applied to project list, filter consumed
  4. "initial filter with no matches shows empty filtered state" (line 7128) — verifies filter applied even when no matches, visible items = 0
  5. "empty initial filter is no-op" (line 7168) — verifies filter state remains Unfiltered, all items visible
  6. "filter consumed after first application" (line 7203) — verifies filter consumed, then after Esc + second SessionsMsg, filter is NOT re-applied
  7. "SessionsMsg handler no longer applies initial filter" (line 7246) — verifies that sending only SessionsMsg (without ProjectsLoadedMsg) does NOT apply filter, proving the filter logic is in evaluateDefaultPage, not SessionsMsg handler
- Notes: Tests are well-structured, each focused on a distinct acceptance criterion. There is some overlap with earlier TestInitialFilter tests (lines 1031-1159) from task 1-6 that also test initial filter behavior, but those are from a prior task and cover the older code path. The duplication is minor and acceptable since the task 1-6 tests were already present before this task restructured the logic.

CODE QUALITY:
- Project conventions: Followed — table-driven subtests pattern used, functional options for model config, value receiver with mutation via return
- SOLID principles: Good — evaluateDefaultPage has clear single responsibility (determine page + apply filter), the guard pattern with defaultPageEvaluated prevents re-entry cleanly
- Complexity: Low — the evaluateDefaultPage method is straightforward with a clear flow: guard check -> wait for data -> determine page -> apply filter -> consume
- Modern idioms: Yes — uses bubbles/list SetFilterText/SetFilterState API correctly as specified
- Readability: Good — method is well-documented with a comprehensive comment explaining its behavior, the conditional logic for command-pending vs normal mode is clear
- Issues: None

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The earlier TestInitialFilter (lines 1049-1093, 1095-1159) from task 1-6 partially overlaps with TestInitialFilterAppliedToDefaultPage tests. These could potentially be consolidated, but it is not worth the churn since they validate slightly different code paths from different task perspectives.
