TASK: Guard CleanStale from empty pane list in ExecuteHooks

ACCEPTANCE CRITERIA:
- ExecuteHooks does NOT call CleanStale when ListAllPanes returns empty slice
- ExecuteHooks does NOT call CleanStale when ListAllPanes returns nil
- ExecuteHooks still calls CleanStale when ListAllPanes returns non-empty slice (existing test passes)
- ExecuteHooks still skips CleanStale on ListAllPanes error (existing test passes)
- Hook execution proceeds normally regardless of cleanup skip
- All existing tests pass: go test ./...

STATUS: Complete

SPEC CONTEXT: The specification identifies Problem 1 as "Hook deletion on restart" -- when a tmux server restarts, ListAllPanes() returns an empty slice (no error), which CleanStale interprets as "all hooks are stale" and deletes everything. The fix is an empty-pane guard matching the existing pattern in cmd/clean.go:77-80. This is the minimal surgical fix for Phase 1 before the structural-key changes in Phases 2-3.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/hooks/executor.go:72
- Notes: The fix is a single condition addition: `&& len(livePanes) > 0` appended to the existing `if livePanes, err := tmux.ListAllPanes(); err == nil` check. This matches the guard pattern in cmd/clean.go:77-80. The approach is correct -- Go's `len(nil)` returns 0, so both nil and empty slice are handled by the same condition. The function docstring (lines 64-69) was updated to document the guard behavior, explaining the post-restart scenario clearly.

TESTS:
- Status: Adequate
- Coverage:
  - "empty pane list skips cleanup and continues hook execution" (line 615): Verifies CleanStale NOT called with empty slice, hooks still fire. This is the renamed/updated version of the old "no tmux server running" test.
  - "ListAllPanes returns nil skips cleanup gracefully" (line 678): Verifies nil case explicitly -- CleanStale NOT called, hooks still fire.
  - "empty pane list preserves hooks for post-restart survival" (line 646): Additional test covering full post-restart scenario (empty panes everywhere, multiple hooks preserved). Not in the plan but adds genuine value for the realistic scenario.
  - "cleanup calls ListAllPanes and CleanStale before hook execution" (line 486): Existing test confirming CleanStale IS called with non-empty panes. Unchanged.
  - "ListAllPanes error skips cleanup and continues" (line 525): Existing test confirming error path. Unchanged.
  - "CleanStale error skips cleanup and continues" (line 553): Existing test confirming CleanStale error is tolerated. Unchanged.
- Notes: All six acceptance criteria are covered by tests. The nil vs empty slice distinction is explicitly tested. The additional "preserves hooks for post-restart survival" test is not over-testing -- it covers a distinct realistic scenario (no session panes either) vs the guard-specific test. Tests are focused and verify behavior, not implementation details.

CODE QUALITY:
- Project conventions: Followed. Uses interface-based DI, best-effort error handling pattern consistent with the codebase. No t.Parallel() in tests (per CLAUDE.md). Mock pattern matches existing test helpers.
- SOLID principles: Good. Single-line change to existing function, no new responsibilities added. Interfaces unchanged.
- Complexity: Low. One additional boolean condition in an existing if-statement.
- Modern idioms: Yes. Idiomatic Go -- len(nil)==0 behavior leveraged correctly, compound if-condition is standard Go style.
- Readability: Good. The inline comment "Best-effort cleanup" and the function docstring clearly explain the guard's purpose and the post-restart scenario.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The old test name "no tmux server running skips cleanup gracefully" was renamed to "empty pane list skips cleanup and continues hook execution" which is more precise. Good.
- The "cleanup runs before loader.Load" test (line 579) has a somewhat awkward verification approach with an unused callOrder variable (line 581), but this predates this task and is not a regression.
