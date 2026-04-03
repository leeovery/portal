TASK: Update MarkerName Format and Executor Tests to Structural Keys

ACCEPTANCE CRITERIA:
- MarkerName parameter is named key with structural key doc comment
- All interface doc comments use structural key terminology
- All executor tests use structural key format values
- Marker assertions verify @portal-active-session:window.pane format
- "empty pane list" test asserts CleanStale NOT called (Phase 1 guard)
- ExecuteHooks function body has no code changes (only doc comment)
- go test ./internal/hooks/... passes

STATUS: Complete

SPEC CONTEXT: The specification requires volatile marker format to change from @portal-active-%paneID to @portal-active-{structural_key} (e.g., @portal-active-my-project-abc:0.0). Interface doc comments must use "structural key" terminology. The executor function body should not change -- it already iterates opaque string keys. The "no tmux server running" test must assert CleanStale is NOT called when livePanes is empty (the Phase 1 empty-pane guard).

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/hooks/executor.go
- Notes:
  - MarkerName parameter renamed from paneID to key (line 55) with structural key doc comment (lines 52-54) -- correct
  - PaneLister doc comment (lines 5-6): uses "structural keys" -- correct
  - KeySender doc comment (line 11): uses "structural key" -- correct
  - AllPaneLister doc comment (lines 27-28): uses "structural keys" -- correct
  - HookCleaner doc comment (lines 33-34): uses "structural keys" -- correct
  - ExecuteHooks doc comment (lines 59-69): uses "structural keys" terminology -- correct
  - ExecuteHooks function body (lines 70-115): unchanged, still uses paneID loop variable -- correct per plan requirement of no code changes
  - Empty-pane guard at line 72: `len(livePanes) > 0` check present -- correct (from Phase 1)

TESTS:
- Status: Adequate
- Coverage:
  - "executes hook when persistent entry exists and marker absent" (line 151): structural key "my-session:0.0" -- correct
  - "skips pane when volatile marker present" (line 176): marker "@portal-active-my-session:0.0" -- correct
  - "skips pane not in session" (line 198): uses "other-session:0.0" for different-session pane -- correct
  - "skips pane with no on-resume event" (line 221): structural key -- correct
  - "sets volatile marker after executing hook" (line 240): asserts "@portal-active-my-session:0.0" -- correct
  - "continues to next pane when SendKeys fails" (line 265): structural keys "my-session:0.0", "my-session:0.1" -- correct
  - "executes hooks for multiple qualifying panes" (line 443): 3 structural keys -- correct
  - "cleanup calls ListAllPanes and CleanStale before hook execution" (line 486): structural keys -- correct
  - "empty pane list skips cleanup and continues hook execution" (line 615): asserts CleanStale NOT called, hooks still fire -- correct
  - No pane ID format values (%N) remain in any test
  - Additional tests added beyond plan: "multi-pane independent hooks fire correctly with structural key targets" (line 365), "orphaned structural keys produce no errors" (line 417), "empty pane list preserves hooks for post-restart survival" (line 646), "ListAllPanes returns nil skips cleanup gracefully" (line 678) -- these cover legitimate edge cases without redundancy
- Notes:
  - Plan specified "executes hooks for multiple qualifying panes" should use keys across two windows (my-session:0.0, my-session:0.1, my-session:1.0). Actual test uses three panes in same window (0.0, 0.1, 0.2). However, multi-window scenario is covered by separate "multi-pane independent hooks" test at line 365. Intent satisfied.
  - All marker assertions use @portal-active-session:window.pane format

CODE QUALITY:
- Project conventions: Followed -- interface-based DI, mock composition pattern, subtests
- SOLID principles: Good -- small focused interfaces, single responsibility
- Complexity: Low -- MarkerName is a one-liner, no logic changes to ExecuteHooks
- Modern idioms: Yes -- standard Go patterns throughout
- Readability: Good -- doc comments are clear and descriptive
- Issues: None

BLOCKING ISSUES:
(none)

NON-BLOCKING NOTES:
- Test helper struct `keySend` (executor_test.go:30-33) still uses field name `paneID` while storing structural key values. Similarly, `mockHookCleaner.livePanesReceived` (line 101) uses "Panes" in the name. These are private test-only types, so the inconsistency is cosmetic. Consider renaming to `key`/`liveKeysReceived` for terminology consistency if a future task touches this file.
- The `ExecuteHooks` function body retains `paneID` as the loop variable name (line 97) and `panes`/`paneSet` as variable names. This is correct per the plan's explicit "no code changes to function body" requirement. A future cleanup task could align these names with structural key terminology.
