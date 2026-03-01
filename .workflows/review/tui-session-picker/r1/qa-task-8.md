TASK: Esc Progressive Back Behavior (tick-f7693a)

ACCEPTANCE CRITERIA:
- Esc during kill modal dismisses the modal (does not quit)
- Esc during rename modal dismisses the modal (does not quit)
- Esc during active filtering clears the filter (does not quit)
- Esc with applied filter clears the filter (does not quit)
- Esc with no modal, no filter, on a page -> quits TUI
- Multiple Esc presses unwind layers in order: modal -> filter -> quit
- Ctrl+C always force-quits regardless of state

STATUS: Complete

SPEC CONTEXT: The spec defines Esc as a progressive "back" key, unwinding one layer at a time: (1) modal active -> dismiss, (2) filter active -> clear filter (bubbles/list handles this), (3) file browser active -> return to Projects page, (4) sessions or projects page with nothing active -> exit TUI. This applies consistently across normal and command-pending modes.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - `/Users/leeovery/Code/portal/internal/tui/model.go:913-959` (updateSessionList — sessions page Esc handling)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:622-678` (updateProjectsPage — projects page Esc handling)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:974-992` (updateModal — centralized modal dispatch with Ctrl+C force-quit)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:994-1014` (updateKillConfirmModal — Esc dismisses kill modal)
  - `/Users/leeovery/Code/portal/internal/tui/model.go:1044-1068` (updateRenameModal — Esc dismisses rename modal)
- Notes: The implementation follows the planned priority order correctly:
  1. Modal check comes first (line 915 for sessions, line 624 for projects) — if modal is active, all input routes to modal handler which catches Esc
  2. Ctrl+C is checked early in both page handlers (lines 921, 630) and in updateModal (line 976)
  3. SettingFilter() check (lines 925, 633) — when actively typing filter, breaks to let list handle Esc (cancels filter)
  4. FilterApplied check (lines 932, 638) — when filter is applied, breaks to let list handle Esc (clears filter via ClearFilter keybinding)
  5. Default Esc -> tea.Quit (lines 935, 641) — when nothing is active, quits

  The old direct `Esc -> tea.Quit` from the default handler has been replaced with the progressive layered approach. No drift from plan.

TESTS:
- Status: Adequate
- Coverage: All 7 planned test cases are implemented in `TestEscProgressiveBack` at `/Users/leeovery/Code/portal/internal/tui/model_test.go:4311-4710`:
  1. "Esc with no modal or filter quits TUI" (line 4312) — verifies tea.QuitMsg
  2. "Esc during kill modal dismisses modal" (line 4329) — verifies no quit, modal dismissed, sessions visible
  3. "Esc during rename modal dismisses modal" (line 4372) — verifies no quit, modal dismissed, renamer not called
  4. "Esc with filter active clears filter" (line 4415) — verifies no quit, filter state Unfiltered, all items visible
  5. "Esc during SettingFilter cancels filter without quitting" (line 4466) — bonus test covering the actively-typing filter state
  6. "Ctrl+C force-quits from any state" (line 4505) — comprehensive: tests Ctrl+C from normal, with filter, during kill modal, during rename modal, during active filtering
  7. "Esc during rename then Esc again quits TUI" (line 4594) — multi-step unwinding: modal -> quit
  8. "Esc clears filter then second Esc quits" (line 4634) — multi-step unwinding: filter -> quit
  9. "Esc on projects page with no filter quits TUI" (line 4685) — verifies projects page Esc behavior
- Additional coverage in other test suites:
  - `TestCommandPendingEscAndQuit` (line 6668) covers command-pending mode Esc behavior
  - Browser Esc tests (line 5320, 5343) cover file browser layer
- Notes: Tests are well-structured with precondition verification before testing the actual behavior. Each test verifies both positive (expected action happened) and negative (unexpected action did not happen) outcomes. The Ctrl+C test is thorough, covering 5 distinct states. The multi-step tests (rename->quit, filter->quit) directly verify the "progressive" nature of the unwinding. No over-testing observed — each test covers a distinct state/behavior combination.

CODE QUALITY:
- Project conventions: Followed — table-driven style is not used here but subtests are properly used within the test function; the implementation uses functional options and small interfaces per the project's patterns
- SOLID principles: Good — updateModal acts as a single dispatch point (SRP), individual modal handlers each own their Esc behavior, the page handlers have clear separation of concerns
- Complexity: Low — the Esc handling in each page handler follows a simple if-chain priority: modal -> SettingFilter -> FilterApplied -> quit. No nested complexity.
- Modern idioms: Yes — uses bubbles/list FilterState() and SettingFilter() APIs correctly; leverages the list's built-in Esc handling for filter states by falling through (break) rather than reimplementing
- Readability: Good — the priority order is clear in the code flow, comments explain the "progressive back" logic (line 930-931), the pattern is consistent between sessions and projects page handlers
- Issues: None

BLOCKING ISSUES:
- (none)

NON-BLOCKING NOTES:
- The Esc handling logic in `updateSessionList` (lines 925-935) and `updateProjectsPage` (lines 633-641) is nearly identical. This duplication is noted but acceptable given that each page may diverge in the future (e.g., file browser layer only applies to projects page). A later analysis phase could consider extracting a shared `handleEscProgressive` helper if the pattern remains identical.
