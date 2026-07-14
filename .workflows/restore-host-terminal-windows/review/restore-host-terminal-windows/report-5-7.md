TASK: restore-host-terminal-windows-5-7 — N=0 / N=1 Enter commit boundary (N≥2 no-op stub → Phase-6 burst)

ACCEPTANCE CRITERIA:
- N=0 Enter sets MultiSelectActive()==false, empties the set, does NOT quit (Selected()=="", no tea.Quit), opens nothing (same effect as Esc).
- N=1 Enter sets Selected() to the one marked session's name and returns tea.Quit (drives the existing single-attach connector — no adapter, no special-casing).
- N≥2 Enter leaves the mode + selection intact, Selected() stays empty, no tea.Quit/spawn issued (authored as a Phase-6 stub; Phase 6 has since wired the burst).
- The commit ignores the cursor/highlight: with a highlighted-but-unmarked row and one OTHER session marked, N=1 Enter opens the MARKED session, not the highlighted one.

STATUS: Complete

SPEC CONTEXT:
Spec "Multi-Select Mode → N=0 / N=1 boundary": N=1 (one marked) degenerates to a plain single attach in the current window (no special-casing); N=0 is a no-op that exits multi-select mode, Portal stays open, nothing opens (same effect as Esc). Spec "Trigger-Context Matrix → Enter opens the marked set only": the cursor/highlight at Enter is irrelevant — marking is `m`, not Enter; Enter always commits the m-marked set. Scope boundary: N≥2 in-process spawn burst, detection gate, pre-flight, cancellation are all Phase 6; task 5.7 authored the N≥2 arm as an explicit stub, with Phase 6 to replace it with the internal/spawn burst.

IMPLEMENTATION:
- Status: Implemented (N≥2 arm has correctly converged to the Phase-6 wiring)
- Location: internal/tui/model.go:3560-3574 (handleMultiSelectEnter); dispatch at model.go:3445-3454 (Enter mode-branch); exitMultiSelect at model.go:3536-3543; handleSessionListEnter at model.go:3734-3741 (the N=1 parity reference).
- Notes:
  - N=0 arm (model.go:3562-3564): `m = m.exitMultiSelect(); return m, nil`. exitMultiSelect clears multiSelectMode, nils selectedSessions, and refreshes the delegate so markers/banner clear. No quit. Matches AC exactly.
  - N=1 arm (model.go:3565-3570): extracts the sole key via `for name = range m.selectedSessions {}`, sets `m.selected = name`, returns `tea.Quit`. This is byte-identical in effect to handleSessionListEnter (set m.selected; tea.Quit), differing only in that the name comes from the marked set rather than the cursor — exactly the required "no special-casing, no adapter" single attach.
  - N≥2 arm (model.go:3571-3572): now `return m.beginBurst(m.orderedMarkedSessions())`. The task authored this as a documented no-op stub, but this task lives inside a now-complete feature and Phase 6 (burst_progress.go) has since replaced the stub with the real burst dispatch, exactly as the task's own scope-boundary note anticipated ("Phase 6 wires the N≥2 spawn burst here"). This is intended convergence, not drift. In the detection-UNWIRED configuration (the default model), beginBurst still defers to a no-op (pendingBurstEnter set, maybeDispatchDetectionCmd returns nil → returns m,nil), preserving the original stub's observable behaviour (mode + selection intact, no quit, nothing opened).
  - Dispatch routing (model.go:3445-3454) correctly gates on `m.multiSelectMode`; the SettingFilter break at model.go:3337 ensures a focused-filter Enter is delegated to the list, so handleMultiSelectEnter is reached only when the filter is not focused — matches the task requirement.

TESTS:
- Status: Adequate
- Location: internal/tui/multi_select_enter_test.go
- Coverage:
  - TestMultiSelectEnterN0 (lines 22-49): exits mode, count 0, Selected()=="", not a quit — full N=0 AC.
  - TestMultiSelectEnterN1 (lines 54-75): one marked (alpha), Selected()=="alpha", isQuitCmd true — full N=1 AC.
  - TestMultiSelectEnterN1IgnoresCursor (lines 80-108): marks bravo, moves cursor back to unmarked alpha, asserts Selected()=="bravo" + quit — the cursor-irrelevance AC.
  - TestMultiSelectEnterN2DetectionUnwired (lines 116-148): two marked, detection unwired → !BurstPending(), mode intact, count still 2, Selected()=="", not a quit — the 5.7 N≥2 AC preserved under the Phase-6 wiring's unwired-defer path.
- Notes:
  - Tests assert observable behaviour through public accessors (MultiSelectActive/SelectedSessionCount/Selected/IsSessionSelected) and the quit-vs-noop distinction via isQuitCmd — behaviour, not implementation details. Not over-tested; each test targets a distinct boundary with no redundant assertions.
  - The N≥2 test is the evolved successor of the original stub test; the supported-terminal burst-dispatch path is (correctly) covered separately in burst_dispatch_test.go, out of 5.7's scope, so this file stays focused on the N-count boundary without a live tmux/detector seam.
  - Would fail if broken: swapping N=1 to use the cursor (handleSessionListEnter directly) would fail TestMultiSelectEnterN1IgnoresCursor; making N=0 quit or N≥2 quit/clear would fail the respective tests.

CODE QUALITY:
- Project conventions: Followed. Small value-receiver handlers returning (tea.Model, tea.Cmd) consistent with the surrounding dispatch arms; delegate refresh via pointer-receiver helpers matches the file's idiom; exitMultiSelect reuse avoids inline duplication.
- SOLID principles: Good. handleMultiSelectEnter has a single responsibility (the N-count branch); the N≥2 spawn concern is delegated to beginBurst, keeping the boundary logic thin.
- Complexity: Low — a three-arm switch on len().
- Modern idioms: Good. The empty-body `for name = range m.selectedSessions {}` is the idiomatic single-key extraction from a one-element map.
- Readability: Good. The function doc block enumerates all three arms against the spec section.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [do-now] internal/tui/model.go:3567 — Add a one-line inline comment on the empty-body `for name = range m.selectedSessions {}` (e.g. "sole key of the guaranteed one-element map") so a reader landing on the loop without the doc block immediately sees why the empty body is intentional. Zero logic impact.
