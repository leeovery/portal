TASK: Command-Pending Esc and Quit Behavior (tick-5c4639)

ACCEPTANCE CRITERIA:
- Esc with nothing active in command-pending mode exits TUI
- Esc with filter active clears filter first (does not exit)
- Second Esc after clearing filter exits TUI
- Esc with modal active dismisses modal first
- Esc in file browser during command-pending returns to Projects page
- q exits from any state in command-pending mode
- In normal mode, Esc on Projects page with nothing active exits TUI (same as command-pending)

STATUS: Complete

SPEC CONTEXT: The spec defines Esc as a progressive "back" key that unwinds one layer at a time: (1) modal active -> dismiss, (2) filter active -> clear filter, (3) file browser active -> return to Projects page, (4) nothing active -> exit TUI. This applies consistently across normal and command-pending modes. The spec also states `q`/`Esc` cancels entirely in command-pending mode (exits TUI without creating a session), with Esc first clearing the filter if active.

IMPLEMENTATION:
- Status: Implemented
- Location: /Users/leeovery/Code/portal/internal/tui/model.go:622-678 (updateProjectsPage)
- Notes: The progressive Esc logic is correctly implemented via layered checks:
  1. Modal check at line 624 intercepts all input when a modal is active; each modal handler dismisses on Esc (lines 728-730, 780-782, 1007-1010, 1057-1060)
  2. Filter-active check at line 633 (`SettingFilter()`) lets bubbles/list handle Esc during active filter input
  3. Filter-applied check at line 638 breaks to delegate Esc to bubbles/list for filter clearing
  4. No-state Esc at line 641 returns `tea.Quit`
  5. `q` at line 642-643 always returns `tea.Quit` regardless of state
  6. File browser cancel at line 556-558 (cross-view handler) sets `activePage = PageProjects`
  The implementation does not differentiate between normal and command-pending mode for Esc handling, which is correct per spec -- the behavior is identical.

TESTS:
- Status: Adequate
- Coverage: All 7 acceptance criteria have dedicated tests in `TestCommandPendingEscAndQuit` at /Users/leeovery/Code/portal/internal/tui/model_test.go:6668-6964:
  - "Esc with nothing active in command-pending mode exits TUI" (line 6669)
  - "Esc with filter active clears filter first in command-pending mode" (line 6699)
  - "two Esc presses: clear filter then exit in command-pending mode" (line 6744)
  - "Esc with modal active dismisses modal in command-pending mode" (line 6786)
  - "Esc in file browser returns to Projects page in command-pending mode" (line 6849)
  - "q exits from any state in command-pending mode" (line 6898)
  - "Esc on Projects page in normal mode with nothing active exits TUI" (line 6929)
- Notes:
  - The modal test (line 6786) pragmatically tests in normal mode since e/d keys are disabled in command-pending mode. This is valid because the `updateProjectsPage` code path for modal-first Esc handling is shared. The test includes a clear comment explaining this decision.
  - There is a minor overlap with an earlier test in `TestCommandPendingMode` at line 2302 ("esc in command-pending mode quits TUI") from the Command-Pending Mode Core task (tick-310db8). This is acceptable -- the earlier test is a smoke test for the core mode, while the task-22 test is the comprehensive dedicated test.
  - Edge case "Esc with filter active needs two presses" is covered by the "two Esc presses" test.
  - Edge case "Esc with modal active dismisses modal first" is covered.
  - Tests would fail if the feature broke (they verify exact quit/non-quit behavior).

CODE QUALITY:
- Project conventions: Followed. Table-driven-style subtests within a parent test function. Mocks follow existing patterns. Go idiomatic error handling.
- SOLID principles: Good. The progressive Esc logic is implemented as layered checks with clear single responsibilities per layer. Modal handling is delegated to a unified `updateModal` dispatcher. No violations detected.
- Complexity: Low. The `updateProjectsPage` function has straightforward branching: modal check -> filter-setting check -> Esc/key switch. Each branch is a simple guard clause. Cyclomatic complexity is reasonable.
- Modern idioms: Yes. Uses bubbles/list filter state API correctly. The `break` to delegate to bubbles/list for filter clearing is idiomatic for the Bubble Tea framework.
- Readability: Good. The layered Esc logic reads top-to-bottom in priority order (modal -> setting filter -> applied filter -> quit). Comments explain the progressive back intent.
- Issues: None identified.

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The duplicate test coverage between `TestCommandPendingMode/"esc in command-pending mode quits TUI"` (line 2302) and `TestCommandPendingEscAndQuit/"Esc with nothing active in command-pending mode exits TUI"` (line 6669) is minor redundancy. Could consolidate, but not worth the churn since they belong to different task scopes.
