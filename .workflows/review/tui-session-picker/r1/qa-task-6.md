TASK: Built-in Filtering and Initial Filter (tick-b00429)

ACCEPTANCE CRITERIA:
- / activates the built-in filter mode
- Filtering fuzzy-matches on session names (via FilterValue())
- Esc while filtering clears the filter (built-in behavior)
- --filter flag pre-applies filter text on launch after items load
- Hand-rolled filter code is removed (filterMode, filterText, updateFilter, filterMatchedSessions, displaySessions)
- Initial filter with no matches shows the list's built-in "no matches" state
- Empty initial filter string is a no-op (no filter applied)

STATUS: Complete

SPEC CONTEXT: The spec states "Independent filters per page. Each bubbles/list manages its own filter state." and "Apply SetFilterText() and SetFilterState(list.FilterApplied) on whichever page is the default." The / key is the default filter activation key in the list's KeyMap.Filter binding. Filtering should fuzzy-match on session names via FilterValue().

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/internal/tui/model.go:370 — `SetFilteringEnabled(true)` on session list
  - /Users/leeovery/Code/portal/internal/tui/model.go:407 — `SetFilteringEnabled(true)` on project list
  - /Users/leeovery/Code/portal/internal/tui/model.go:468-500 — `evaluateDefaultPage()` applies initial filter via `SetFilterText()` and `SetFilterState(list.FilterApplied)` to whichever page is the default, then clears `m.initialFilter`
  - /Users/leeovery/Code/portal/internal/tui/model.go:256-261 — `WithInitialFilter()` stores the filter string
  - /Users/leeovery/Code/portal/internal/tui/session_item.go:35-37 — `FilterValue()` returns session name
  - /Users/leeovery/Code/portal/internal/tui/model.go:913-958 — `updateSessionList()` does NOT intercept `/`, letting bubbles/list handle it natively via the `m.sessionList.Update(msg)` fallthrough at line 957
  - /Users/leeovery/Code/portal/internal/tui/model.go:929-935 — Esc progressive back: when filter is active (`FilterApplied`), breaks out to let list handle clearing; otherwise quits
- Notes:
  - No hand-rolled filter code exists: grep for `filterMode`, `filterText`, `updateFilter`, `filterMatchedSessions`, `displaySessions` returns zero matches in the tui package
  - No `internal/fuzzy` import in the tui package
  - Empty initial filter skipped at line 488: `if m.initialFilter == "" { return }`
  - Initial filter consumed (cleared to "") at line 499 after first application, preventing re-application on subsequent data loads

TESTS:
- Status: Adequate
- Coverage:
  - `TestBuiltInFiltering` at model_test.go:3096:
    - "initial filter pre-applies filter text after items load" (line 3097) — verifies FilterApplied state, filter value "myapp", 2 visible items (matching), 3 total items
    - "initial filter with no matches shows empty filtered state" (line 3149) — verifies FilterApplied state with 0 visible items for non-matching filter
    - "empty initial filter is no-op" (line 3183) — verifies Unfiltered state, all items visible
    - "list handles filter activation via slash key" (line 3208) — verifies pressing / sets state to Filtering
    - "Esc clears active filter" (line 3226) — verifies Esc on FilterApplied returns to Unfiltered with all items visible
  - `TestInitialFilter` at model_test.go:1031:
    - "model stores initial filter text" — verifies WithInitialFilter stores value
    - "initial filter defaults to empty" — verifies default is ""
    - "evaluateDefaultPage applies initial filter via built-in list filtering" — overlapping with TestBuiltInFiltering but uses evaluateDefaultPage path
    - "initial filter consumed on first load only" — verifies re-load does not re-apply filter
    - "command-pending mode applies initial filter to project list" — verifies filter applied to project list in command-pending mode
  - `TestInitialFilterAppliedToDefaultPage` at model_test.go:6966:
    - "initial filter applied to Sessions page when sessions exist" — full path with both sessions and projects loaded
    - "initial filter applied to Projects page when no sessions exist" — filter applied to projects when no sessions
    - "initial filter applied to Projects page in command-pending mode" — command-pending filter application
    - "initial filter with no matches shows empty filtered state" — edge case
    - "empty initial filter is no-op" — edge case
  - `TestFilterMode` at model_test.go:2079:
    - "q does not quit while filtering is active" — verifies keys are routed to filter during Filtering state
    - "shortcut keys functional after filter mode exit" — verifies keys work again after exiting filter
- Notes:
  - All 5 specified test cases from the task are covered
  - Both edge cases (no matches, empty initial filter) are tested
  - Some overlap between TestInitialFilter, TestBuiltInFiltering, and TestInitialFilterAppliedToDefaultPage — the same scenarios appear multiple times (e.g., initial filter pre-applies is tested 3 times with slight variations). This is minor redundancy but not blocking.

CODE QUALITY:
- Project conventions: Followed — table-driven test style not applicable here (behavioral Bubble Tea tests), but tests use clear subtests with t.Run
- SOLID principles: Good — filtering is delegated entirely to bubbles/list (single responsibility); WithInitialFilter is a clean functional builder
- Complexity: Low — the evaluateDefaultPage method has clear conditional logic; initial filter application is ~12 lines
- Modern idioms: Yes — uses bubbles/list API correctly (SetFilterText, SetFilterState, FilterApplied, SettingFilter)
- Readability: Good — the evaluateDefaultPage method documents its purpose clearly; the guard clause pattern (early return for empty filter) is clean
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- There is some test duplication between TestInitialFilter (line 1049), TestBuiltInFiltering (line 3097), and TestInitialFilterAppliedToDefaultPage (line 6967) — all three test "initial filter pre-applies filter text after items load" with nearly identical setups. Consider consolidating to reduce maintenance burden.
