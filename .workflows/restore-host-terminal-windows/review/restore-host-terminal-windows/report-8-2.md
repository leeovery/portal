TASK: Suppress n (new-session-in-cwd) while in multi-select mode (restore-host-terminal-windows-8-2 / tick-407482)

ACCEPTANCE CRITERIA:
- Pressing `n` while in multi-select mode is a no-op: no session created, picker does not quit, marked set preserved.
- `n` outside multi-select mode continues to create a session in the cwd and quit as before.
- Live-set (Space, /, s) and suppressed set (k, x, r, and now n) match the spec's key-coexistence rule.

STATUS: Complete

SPEC CONTEXT:
Spec §"Key coexistence within the mode" (specification.md:113-116) fixes a closed live-set — "Live in mode: Space (preview), / (filter), s (regroup)" — and states "Suppressed in mode: k (kill), x (page-toggle), r (rename), and other row actions." `n` (new-session-in-cwd) is not in the live-set and, uniquely among the suppressed keys, would createSessionInCWD → quit, silently discarding the marked set. Task closes that divergence by gating `n` like its k/r/x siblings.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/model.go:3411-3420 — the `case isRuneKey(msg, "n")` arm now leads with `if m.multiSelectMode { return m, nil }` before `return m.handleNewInCWD()`, mirroring the k (3394-3402), r (3403-3410), and x (3436-3444) guards. Arm kept PRESENT (gated, not deleted) so keymap_dispatch_guard_test.go's default-mode dispatch-parity probe stays green.
- Notes:
  - Do #2 satisfied: the only other route to handleNewInCWD is the Projects-page arm (model.go:2634), which is unreachable while multiSelectMode is true — multi-select is a Sessions-only mode and the `x` page-toggle is itself suppressed in the mode (3440-3442), so there is no path from multi-select to the Projects page. No other caller of handleNewInCWD / createSessionInCWD exists (grep confirmed).
  - Do #3 satisfied: out-of-mode `n` still calls m.handleNewInCWD() unchanged (3420).
  - Comment on the arm is accurate and explains the create-and-quit hazard.

TESTS:
- Status: Adequate
- Coverage:
  - TestMultiSelectSuppressesNewInCWD (multi_select_keymap_test.go:113-146) — enters mode, marks 1 session (precondition asserts count==1), sends `n` via updateSessionList; asserts (a) no command dispatched (and if a cmd leaks, runs it and fails if it produces a msg — surfaces any create/quit), (b) creator.dir=="" (no CreateFromDir), (c) SelectedSessionCount==1 (marked set preserved), (d) still MultiSelectActive, (e) activePage==PageSessions (no quit/page-leave). Directly covers all three in-mode acceptance sub-clauses.
  - TestOutOfModeNewInCWDUnchanged (151-181) — out of mode, `n` dispatches createSessionInCWD, produces a SessionCreatedMsg created in the cwd (/home/user/mydir), and feeding that msg back quits with the created session selected. Covers the regression criterion.
  - Live-set / suppressed-set coexistence: TestMultiSelectKeepsCoexistingKeysLive (Space//s live, 185-227) and TestMultiSelectSuppressesRowActions (k/x/r suppressed, 60-104) together with the two new tests cover the full spec key-coexistence rule.
- Notes: Assertions are distinct (each maps to a separate acceptance clause) — no redundancy, no over-testing. The leaked-cmd-execution check (129-133) is a precise behavioural guard rather than an implementation-detail assertion. Would fail if the gate were removed (n would produce a SessionCreatedMsg / quit). Test double recordingCreator is the shared existing mock — no unnecessary new scaffolding.

CODE QUALITY:
- Project conventions: Followed. Gate mirrors the established sibling pattern; DI seam (SessionCreator) untouched; no t.Parallel; tests read model state rather than execute real commands.
- SOLID principles: Good. Single-responsibility guard, no new surface area.
- Complexity: Low. One added conditional, identical shape to three neighbours.
- Modern idioms: Yes.
- Readability: Good. Comment states intent and the specific create-and-quit hazard.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
