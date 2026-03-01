TASK: Eliminate duplicated window-label pluralization in SessionDelegate.Render (tick-dad932)

ACCEPTANCE CRITERIA:
- Pluralization logic ("%d windows" with special case for 1) appears exactly once
- Description() and Render() both use the shared helper
- All existing session item and delegate tests pass

STATUS: Complete

SPEC CONTEXT: The spec requires session items to show "window count" via a custom ItemDelegate. The pluralization (singular "window" vs plural "windows") is a display detail. The task is a Phase 4 analysis finding to eliminate code duplication between Description() and Render().

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/session_item.go:21-26 (windowLabel helper), :47 (Description() calls windowLabel), :85 (Render() calls windowLabel)
- Notes: The shared `windowLabel(count int) string` function is defined once at lines 21-26. Both `Description()` (line 47) and `Render()` (line 85) call it. A grep for `%d window` across the entire `internal/tui` directory confirms the format string appears exactly once. The deduplication is clean and complete.

TESTS:
- Status: Adequate
- Coverage: `window_label_test.go` directly tests the `windowLabel()` helper with a table-driven test covering singular (1), plural (3), zero (0), and large count (100). Existing `session_item_test.go` tests for `Description()` cover singular/plural with and without attached badge. Delegate `Render()` tests cover singular, plural, attached, and detached cases.
- Notes: The `windowLabel` test is internal (package `tui`) which is correct since the function is unexported. Test coverage is thorough without being redundant -- the unit test on `windowLabel` verifies the helper in isolation, while `Description` and `Render` tests verify integration.

CODE QUALITY:
- Project conventions: Followed. Table-driven tests per golang-pro skill. Idiomatic Go.
- SOLID principles: Good. Single responsibility -- windowLabel does one thing. DRY principle satisfied (the whole point of this task).
- Complexity: Low. Simple conditional with one branch.
- Modern idioms: Yes. Clean function extraction, fmt.Sprintf usage.
- Readability: Good. Function name is self-documenting, godoc comment is clear.
- Issues: None.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- (none)
