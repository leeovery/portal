TASK: Add Multi-Pane and Graceful No-Op Acceptance Tests

ACCEPTANCE CRITERIA:
- Multi-pane test: 3 panes with independent hooks, all fire correctly
- Graceful no-op test: orphaned structural keys produce no errors
- Hook survival test: empty pane list preserves hooks
- Upgrade path test: old pane-ID entries cleaned by CleanStale when live structural keys exist
- go test ./internal/hooks/... passes
- go test ./... passes (full suite)

STATUS: Complete

SPEC CONTEXT: The specification requires tests for: (1) multi-pane sessions with independent hook entries keyed by distinct structural positions, (2) graceful no-op when structural keys don't match live panes (no-resurrect scenario), (3) hook survival when ListAllPanes returns empty (post-restart guard), and (4) old pane-ID entries cleaned by CleanStale on first run after upgrade. Spec sections: "Behavioral Requirements" (Multi-pane support, Silent operation, Graceful failure without tmux-resurrect) and "Testing Requirements."

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/hooks/executor_test.go:365-415 -- "multi-pane independent hooks fire correctly with structural key targets"
  - internal/hooks/executor_test.go:417-441 -- "orphaned structural keys produce no errors and no send-keys calls"
  - internal/hooks/executor_test.go:646-676 -- "empty pane list preserves hooks for post-restart survival"
  - internal/hooks/store_test.go:589-639 -- "old pane-ID entries cleaned on first run after upgrade"
- Notes:
  - All four tests specified in the acceptance criteria are present.
  - Test 3 ("hook survival") deviates slightly from the plan's "Do" section which specified ListPanes returning a pane for the session (resulting in 1 send-keys call). The actual implementation uses empty ListPanes (0 send-keys). This is acceptable because the existing test at line 615 ("empty pane list skips cleanup and continues hook execution") already covers the scenario where ListAllPanes is empty but ListPanes has panes and hooks still fire. Together, these two tests fully cover the requirement.
  - Test 4 ("upgrade path") was placed in store_test.go rather than executor_test.go (as loosely implied in the Description section). This is architecturally correct since it tests CleanStale directly on the Store, not ExecuteHooks.
  - Test 2 ("no-op") tests the case where ALL hooks are orphaned (0 send-keys), rather than the plan's suggested mix of 1 matching + 2 orphaned. The "matching subset fires" scenario is already well covered by the existing "skips pane not in session" test (line 198).

TESTS:
- Status: Adequate
- Coverage:
  - Multi-pane: 3 panes across 2 windows (0.0, 0.1, 1.0), each with independent hooks. Verifies 3 send-keys calls with correct pane-to-command mapping and 3 volatile markers.
  - Graceful no-op: 3 orphaned structural keys (0.1, 1.0, 2.0) with 1 live pane (0.0). Verifies 0 send-keys and 0 marker-sets.
  - Hook survival: ListAllPanes returns empty, ListPanes returns empty. Verifies CleanStale NOT called and 0 send-keys. Combined with line 615 test, covers the full post-restart scenario.
  - Upgrade path: Mix of old pane-ID keys (%0, %3) and structural key (my-session:0.0). Verifies old entries removed, structural entry preserved.
  - Tests would fail if features broke: changing the empty-pane guard, structural key matching, or CleanStale cross-reference logic would cause failures.
- Notes:
  - Minor overlap between the multi-pane test (line 365) and the pre-existing "executes hooks for multiple qualifying panes" (line 443, from Phase 2 Task 4). Both test 3 panes with independent hooks. The new test adds multi-window coverage and explicit per-pane marker verification. Not a blocking concern -- the new test focuses on the specific structural key cross-window scenario called out in the spec.

CODE QUALITY:
- Project conventions: Followed. Tests use the established mock patterns (noopTmux, noopStore, mockPaneLister, etc.). No t.Parallel() as mandated by CLAUDE.md. Standard Go testing idioms.
- SOLID principles: Good. Tests use the existing interface-based mock system without introducing new coupling.
- Complexity: Low. Each test is straightforward: setup mocks, call ExecuteHooks/CleanStale, assert results.
- Modern idioms: Yes. Uses map-based assertion patterns for order-independent verification of multi-pane results.
- Readability: Good. Test names are descriptive. Comments explain the scenario being tested. Assertion messages include both got and want values.
- Issues: None.

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- The multi-pane test at line 365 has significant overlap with the pre-existing "executes hooks for multiple qualifying panes" at line 443. The key differentiator is multi-window coverage (0.0, 0.1, 1.0 vs 0.0, 0.1, 0.2). If the team finds this redundant, the older test could potentially be removed in favor of the more comprehensive new one. Low priority.
- The no-op test at line 417 tests the "all orphaned" scenario. The plan's "Do" section envisioned a mixed scenario (1 matching + 2 orphaned), but the existing test at line 198 ("skips pane not in session") already covers the mixed case. No additional test is needed.
