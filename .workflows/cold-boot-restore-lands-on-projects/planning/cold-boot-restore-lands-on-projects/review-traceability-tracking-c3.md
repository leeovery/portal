---
status: complete
created: 2026-06-26
cycle: 3
phase: Traceability Review
topic: Cold-Boot Restore Lands on Projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Traceability

## Findings

No findings. The plan remains a faithful, complete bidirectional translation of the specification.

This is cycle 3 — a follow-up after cycle 1 (two traceability findings applied) and cycle 2 (traceability clean; one integrity finding fixed — Task 1-4 Problem statement realigned to "Three invariants"). Both cycle-1 fixes and the cycle-2 integrity fix were re-verified present and consistent across **both** the markdown task file (`phase-1-tasks.md`) and the tick store (`tick-9b305e` / `tick-28a91d` / `tick-7f37e3` / `tick-6fee61`). No new gap was introduced, and no remaining gap was found.

## Re-verification of prior fixes

- **Cycle-1 finding 1 (AC6 / `commandPending` coverage in Task 1-3)** — present in `phase-1-tasks.md` (Do line 137, Acceptance lines 144-145, Tests lines 152-153, Edge Cases line 159, Spec Reference line 166) and in tick `tick-7f37e3`'s description (TestCommandPending_LandsOnProjects_NoInterimFlash, the AC6 acceptance pair, both test names, the non-intersection edge case, and the amended spec reference). Consistent across both stores.
- **Cycle-1 finding 2 (failing-refetch-quit coverage in Task 1-4)** — present in `phase-1-tasks.md` (Do line 183, Acceptance line 189, Test line 196, Edge Cases line 202, Spec Reference line 211) and in tick `tick-6fee61`'s description (TestColdBoot_RefetchError_QuitsWithoutStrandingInterim, the failing-refetch acceptance criterion, the test name, the degrades-to-quit edge case, and the amended spec reference). Consistent across both stores.
- **Cycle-2 integrity fix (Task 1-4 Problem "Three invariants")** — `phase-1-tasks.md` line 172 and tick `tick-6fee61` both state the Problem covers three invariants: (a) valid interim page, (b) `ProjectsLoadedMsg` order independence, (c) failing-refetch quit. Aligned with the task's Do / Acceptance / Tests / Edge Cases (which carry all three).

## Verification Summary

### Direction 1: Specification → Plan (completeness)

Every spec element is represented in the plan at adequate implementer-grade depth:

- **Acceptance Criteria** — AC1 → Task 1-1; AC2 → Task 1-2; AC3 (filter co-defect, incl. filter applied to session list, project-filter-untouched, `InitialFilter()` zeroed/consumed) → Task 1-2; AC4 → Task 1-3; AC5 → Task 1-3; AC6 (commandPending → Projects, no interim flash) → Task 1-3; AC7 → Task 1-4. All seven required rows are covered.
- **Testing Requirements** — the mandatory "deliver `ProjectsLoadedMsg` before the loading-page transition" rule is carried in Tasks 1-1, 1-2, and 1-4 (and is the explicit defence against the old test's "passes for the wrong reason" blind spot). Case 1 → 1-1; cases 2 & 3 → 1-2; case 4 (warm parity + `refetchSessionsAfterRestore()` nil/non-nil) → 1-3; cases 5 & 6 → 1-4. "Each test asserts the active page, not just list contents" is honoured in every cold/warm test.
- **Fix Approach mechanics** — cold-route interim `PageSessions` + no `sessionsLoaded` + no `evaluateDefaultPage()` (1-1); warm-route synchronous decision unchanged (1-1/1-3); refetch coupled in the same handler return as the transition (1-1); ordering contract / `ProjectsLoadedMsg`-order independence (1-4); filter co-defect resolved by the single deferral with no extra code (1-2); one-shot `defaultPageEvaluated` latch untouched (1-1).
- **Constraints & Invariants** — warm/CLI/direct-path untouched (1-3); no over-correction (1-2); valid interim page (1-4); latch preserved (1-1); decision always resolves on the cold route (1-4); canonical `progressReceiver != nil` predicate with no alternative probe — no `serverStarted` / `tmux has-server` / `shouldRunConcurrentBootstrap` re-probe (1-1/1-3); interim render content not special-cased (1-4); `commandPending` branch preserved + does not intersect the deferral (1-3); failing refetch degrades to today's quit (1-4); scope confined to `internal/tui/model.go` (1-1).
- **Interim-window input "accepted as-is / out of scope"** — correctly captured as a do-not-test edge case in Task 1-4 (a non-requirement, not a deliverable).
- **Out of Scope items** — the plan respects every exclusion (no orchestrator/restore/scrollback/enumeration changes; latch and decision-logic untouched; no interim-render polish; `_portal-saver` `_`-prefix filter untouched; no severity/release handling).

### Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every task's Problem, Solution, Outcome, Do steps, Acceptance Criteria, Tests, and Edge Cases traces to a specific spec section. No hallucinated requirements, invented edge cases, or untraceable technical approaches were found. All key identifiers used in the plan (`progressReceiver`, `defaultPageEvaluated`, `sessionsLoaded`, `projectsLoaded`, `loadingPadTick`, `shouldRunConcurrentBootstrap`, `commandPending`, `initialFilter`, `transitionFromLoading`, `refetchSessionsAfterRestore`, `evaluateDefaultPage`) appear verbatim in the spec. Source line/file references (e.g. `model.go:632-639`, `~line 1828`) are implementation-locating aids, not invented scope.

## Notes on items deliberately not raised (carried forward from cycle 2)

- **Test-case-6 decision-point wording.** The spec contains an internal tension: §Constraints (line 76) says the `ProjectsLoadedMsg` handler is the decision point under the late-`ProjectsLoadedMsg` ordering, while §Testing Requirements case 6 specifies an ordering where `ProjectsLoadedMsg` is delivered *first* and the refetch `SessionsMsg` lands *second* — under which the `SessionsMsg` handler is the decision point (per §Fix Approach line 48, "whichever lands second"). Task 1-4 follows the concrete, operative testing-requirement ordering and names the `SessionsMsg` handler as the decision point; it also documents the order-independence explicitly. This is faithful to the actionable spec text (the deferral is order-independent regardless), so it is not a plan defect. Flagged again for transparency only — a spec-internal wording nuance, not a plan/spec divergence.
- **Demo harness (`demo/portal-cold.tape`, 12 sessions / 10 projects).** Described in the spec's Context as the pre-existing end-to-end manual-verification path that *complements* the programmatic regression tests. It is descriptive reproduction context, not a deliverable the spec asks to build or modify; the spec's actionable test surface is the six programmatic cases, all of which the plan covers. Correctly absent from the plan as tasks.
