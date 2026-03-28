TASK: Hook Executor Core Logic (resume-sessions-after-reboot-2-2)

ACCEPTANCE CRITERIA:
- Two-condition check: executes only when persistent entry exists AND GetServerOption returns error (marker absent)
- Iterates over hook store entries (not tmux pane list) per spec ordering requirement
- After executing, SetServerOption called with @portal-active-{paneID}
- Panes with existing markers skipped
- Panes with no hook entry skipped
- SendKeys failure for one pane does not block others
- Hook store Load error causes silent return
- ListPanes error causes silent return
- All tests pass: go test ./internal/hooks/...

STATUS: Issues Found

SPEC CONTEXT: The "Execution Mechanics" section defines the two-condition execution check: "persistent entry exists AND volatile marker absent." After execution, the volatile marker is set to prevent re-execution. send-keys failures are silently ignored. Multiple panes are executed sequentially. Scope is the target session's panes only. The spec also says: "The volatile marker is still set for the failing pane to prevent retry loops" (from the plan edge case documentation for task 2-2).

IMPLEMENTATION:
- Status: Implemented (with expected post-plan evolution)
- Location: /Users/leeovery/Code/portal/internal/hooks/executor.go
- Notes:
  - All 8 acceptance criteria are correctly implemented in the code.
  - Line 91: Iterates over `hookMap` (the hook store entries), not the pane list. Correct per spec.
  - Lines 92-94: Checks if pane is in the session's pane set; skips if not. Correct.
  - Lines 96-99: Checks for `on-resume` event; skips if absent. Correct.
  - Lines 101-104: Two-condition check -- GetServerOption returning nil error means marker exists, skip. Error means marker absent, proceed. Correct.
  - Line 106: `_ = tmux.SendKeys(...)` discards error, ensuring one pane's failure does not block others. Correct.
  - Line 107: `_ = tmux.SetServerOption(markerName, "1")` called regardless of SendKeys result. Correct per spec ("marker still set to prevent retry loops").
  - Lines 70-73: Load error causes silent return. Correct.
  - Lines 78-80: ListPanes error causes silent return. Correct.
  - The function signature evolved from 7 individual interface parameters to 3 parameters (sessionName, TmuxOperator, HookRepository) as part of Phase 4 task 4-4. This is a planned refactor and is correct.
  - Stale cleanup logic (lines 65-68) was added by Phase 3 task 3-3. This is additive and does not affect the core 2-2 logic.
  - `MarkerName` helper (line 53) was extracted by Phase 4 task 4-3. Correct centralization.
  - The small interfaces (PaneLister, KeySender, OptionChecker, HookLoader) are all defined and preserved alongside the composed interfaces. Good ISP compliance.

TESTS:
- Status: Under-tested (minor gaps)
- Coverage: The following planned test scenarios have dedicated tests:
  - "executes hook when persistent entry exists and marker absent" (line 151) -- YES
  - "skips pane when volatile marker present" (line 176) -- YES
  - "skips pane not in session" (line 198) -- YES
  - "skips pane with no on-resume event" (line 221) -- YES
  - "sets volatile marker after executing hook" (line 240) -- YES
  - "continues to next pane when SendKeys fails" (line 265) -- YES
  - "silent return when hook store Load fails" (line 292) -- YES
  - "silent return when ListPanes fails" (line 310) -- YES
  - "no-op when hook store is empty" (line 329) -- YES
  - "no-op when session has no panes" (line 346) -- YES (bonus)
  - "executes hooks for multiple qualifying panes" (line 365) -- YES
  - Cleanup tests in TestExecuteHooks_Cleanup (lines 407-569) -- YES (Phase 3 addition)

  Missing from plan test list:
  1. "sets volatile marker even when send-keys fails" -- MISSING. The "continues to next pane when SendKeys fails" test only verifies that %7 got SendKeys, but does NOT assert SetServerOption was called for the failed pane %3. The implementation is correct (line 107 always runs), but there is no test proving the marker is set for a pane whose SendKeys failed.
  2. "mixed panes some execute some skip" -- MISSING as a dedicated test. The "skips pane not in session" test has two panes where one is in session and one is not, but there is no test with a pane that has a marker (skip) alongside one without (execute) alongside one with no hook entry (skip) -- i.e., all three skip reasons in a single test.
  3. "all panes already have volatile markers skips all" -- MISSING. No test where all panes in the session have existing markers and the expected result is zero SendKeys calls.

- Notes: The existing tests would fail if the feature broke -- they cover the core happy path and major error conditions well. The gaps are specific edge case scenarios that are mostly implicitly covered by the logic of other tests but not explicitly verified. Gap #1 is the most significant since it tests a specific behavior that the spec and plan both call out.

CODE QUALITY:
- Project conventions: Followed. Uses the same DI pattern as the rest of the codebase (small interfaces, test mocks via embedding). Uses `_test` package suffix for black-box testing. No `t.Parallel()` (per CLAUDE.md). File location in `internal/hooks/` is appropriate.
- SOLID principles: Good. Small single-method interfaces (PaneLister, KeySender, HookLoader) composed into TmuxOperator and HookRepository. Single responsibility -- ExecuteHooks does one thing. Open/closed -- new event types could be added without modifying the core loop structure.
- Complexity: Low. Linear flow with early returns. No nesting deeper than 2 levels. The main loop body is straightforward: check membership, check event, check marker, send, set marker.
- Modern idioms: Yes. Uses `struct{}` for set values. Uses composed interfaces. Uses `range` over maps. Error handling follows Go conventions.
- Readability: Good. Function is well-documented with a clear godoc comment. Variable names are clear (hookMap, paneSet, markerName). The flow reads top to bottom with no jumps.
- Issues: None found.

BLOCKING ISSUES:
- None. All acceptance criteria are met in the implementation. The test gaps are non-blocking because (a) the implementation is correct, (b) the missing scenarios are partially covered by existing tests, and (c) the missing tests would be improvements rather than requirements.

NON-BLOCKING NOTES:
- Missing test: "sets volatile marker even when send-keys fails". Add a test that sets up a pane with a failing SendKeys, then asserts that SetServerOption was still called for that pane. This is explicitly called out in the plan and is a specific spec behavior ("The volatile marker is still set for the failing pane to prevent retry loops").
- Missing test: "all panes already have volatile markers skips all". Would be a clean scenario test verifying normal reattach behavior (not post-reboot).
- Missing test: "mixed panes (some execute, some skip)". A comprehensive test combining marker-present, no-hook-entry, and marker-absent panes in one scenario would provide stronger regression protection.
- The `for paneID, events := range hookMap` iteration (line 91) uses Go map iteration which is non-deterministic. The plan explicitly acknowledges this as acceptable. However, the "continues to next pane when SendKeys fails" test (line 265) implicitly depends on both %3 and %7 being visited -- which is guaranteed since both are in the map. This is fine but worth noting.
