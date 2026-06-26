---
status: complete
created: 2026-06-26
cycle: 2
phase: Traceability Review
topic: Cold-Boot Restore Lands on Projects
---

# Review Tracking: Cold-Boot Restore Lands on Projects - Traceability

## Findings

No findings. The plan is a faithful, complete bidirectional translation of the specification.

This is cycle 2 — a follow-up after cycle 1's two findings were applied (AC6 commandPending coverage added to Task 1-3; failing-refetch-quit coverage added to Task 1-4). Both fixes were verified present and consistent across **both** the markdown task file (`phase-1-tasks.md`) and the tick store (`tick-7f37e3` / `tick-6fee61`). Neither fix introduced a new gap.

## Verification Summary

### Direction 1: Specification → Plan (completeness)

Every spec element is represented in the plan at adequate implementer-grade depth:

- **Acceptance Criteria** — AC1 → Task 1-1; AC2 → Task 1-2; AC3 (filter co-defect, incl. `InitialFilter()` zeroed/consumed + project-filter-untouched assertions) → Task 1-2; AC4 → Task 1-3; AC5 → Task 1-3; AC6 (commandPending → Projects, no interim flash) → Task 1-3 (cycle-1 fix, confirmed present); AC7 → Task 1-4.
- **Testing Requirements** — the mandatory "deliver `ProjectsLoadedMsg` before the transition" rule is carried in Tasks 1-1, 1-2, and 1-4; case 1 → 1-1; cases 2 & 3 → 1-2; case 4 (warm parity + `refetchSessionsAfterRestore()` nil/non-nil) → 1-3; cases 5 & 6 → 1-4. "Each test asserts the active page, not just list contents" is honoured in every cold/warm test.
- **Fix Approach mechanics** — cold-route interim `PageSessions` + no `sessionsLoaded` + no `evaluateDefaultPage()` (1-1); warm-route synchronous decision unchanged (1-1/1-3); refetch coupled in the same handler return (1-1); ordering contract / `ProjectsLoadedMsg`-order independence (1-4); filter co-defect resolved by the single deferral (1-2); latch untouched (1-1).
- **Constraints & Invariants** — warm/CLI/direct-path untouched (1-3); no over-correction (1-2); valid interim page (1-4); latch preserved (1-1); decision always resolves on the cold route (1-4); canonical `progressReceiver != nil` predicate with no alternative probe (1-1/1-3); interim render content not special-cased (1-4); `commandPending` does not intersect the deferral (1-3, cycle-1 fix); failing refetch degrades to today's quit (1-4, cycle-1 fix); scope confined to `internal/tui/model.go` (1-1).
- **Interim-window input "accepted as-is / out of scope"** — correctly captured as a do-not-test edge case in Task 1-4 (a non-requirement, not a deliverable).
- **Out of Scope items** — the plan respects every exclusion (no orchestrator/restore/enumeration changes, latch/decision-logic untouched, no interim-render polish, `_portal-saver` filter untouched, no severity/release handling).

### Direction 2: Plan → Specification (fidelity / anti-hallucination)

Every task's Problem, Solution, Outcome, Do steps, Acceptance Criteria, Tests, and Edge Cases trace to a specific spec section. No hallucinated requirements, invented edge cases, or untraceable technical approaches were found. All key identifiers used in the plan (`progressReceiver`, `defaultPageEvaluated`, `sessionsLoaded`, `projectsLoaded`, `loadingPadTick`, `shouldRunConcurrentBootstrap`) appear verbatim in the spec. Source line/file references (e.g. `model.go:632-639`) are implementation-locating aids, not invented scope.

## Notes on items deliberately not raised

- **Test-case-6 decision-point wording.** The spec contains an internal tension: §Constraints line 76 says the `ProjectsLoadedMsg` handler is the decision point under the late-`ProjectsLoadedMsg` ordering, while §Testing Requirements case 6 specifies an ordering where `ProjectsLoadedMsg` is delivered *first* and the refetch `SessionsMsg` lands *second* — under which the `SessionsMsg` handler is the decision point (per §Fix Approach line 48, "whichever lands second"). Task 1-4 follows the concrete, operative testing-requirement ordering and names the `SessionsMsg` handler as the decision point. This is faithful to the actionable spec text (and the deferral is order-independent regardless), so it is not a plan defect. Flagged here for transparency; it is a spec-internal wording nuance, not a plan/spec divergence to fix in the plan.
- **Demo harness (`demo/portal-cold.tape`, 12 sessions / 10 projects).** Described in the spec's Context as the pre-existing end-to-end manual-verification path that *complements* the programmatic regression tests. It is descriptive reproduction context, not a deliverable the spec asks to build or modify; the spec's actionable test surface is the six programmatic cases, all of which the plan covers. Correctly absent from the plan as tasks.
