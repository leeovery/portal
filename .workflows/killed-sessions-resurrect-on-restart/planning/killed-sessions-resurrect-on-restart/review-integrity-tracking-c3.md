---
status: complete
created: 2026-05-10
cycle: 3
phase: Plan Integrity Review
topic: Killed Sessions Resurrect on Restart
---

# Review Tracking: Killed Sessions Resurrect on Restart - Integrity

## Findings

No findings. Plan integrity has converged.

## Convergence Summary

Read end-to-end: `planning.md` (3 phases, 18 tasks across the task tables) and full task detail in `phase-1-tasks.md` (8 tasks), `phase-2-tasks.md` (6 tasks), `phase-3-tasks.md` (4 tasks).

Cycle 2 integrity findings were fixed in-place and verified in this cycle:

1. The planning.md task-table rows for tasks 2-2 and 2-5 now match the phase-2-tasks.md task names and edge-cases (sleep-ownership consolidation propagated end-to-end). Verified:
   - planning.md line 83 (task 2-2 row) reads `comment documents that runHydrate (per task 2-1) owns the 100 ms settle-sleep before exec`.
   - planning.md line 86 (task 2-5 row) reads `Unit test: runHydrate timeout fall-through preserves the 100 ms settle-sleep, marker-unset ordering, and FIFO-unlink tolerance`.
2. The Phase 2 acceptance line on supersession (planning.md line 74) is now unambiguous about the record living in the killed-sessions spec at lines 156–163 and the behavioural deliveries owned by tasks 2-1 and 2-3.

Re-evaluated against all eight integrity criteria for cycle 3:

- **Task Template Compliance**: Every task has Problem, Solution, Outcome, Do, Acceptance Criteria, Tests. Doc-only tasks (1-7, 3-2) and the observational task (3-4) explicitly note the absence of new test cases with regression-posture justification, which is acceptable per task-design.md.
- **Vertical Slicing**: Each task is independently testable. The seam → orchestrator → adapter chain across 1-3/1-4/1-5 uses NoOp placeholders so each step verifies on its own. Test-first flips in 2-1/2-3 keep each TDD cycle self-contained.
- **Phase Structure**: Phase 1 (root cause) → Phase 2 (timeout-path corrections built on Phase 1 steady state) → Phase 3 (independent wrapper drop). Rationale captured under each phase's "Why this order".
- **Dependencies and Ordering**: Sequential intra-phase tasks rely on natural ordering. Cross-task references are explicit where they matter (1-2 depends on 1-1; 1-4 depends on 1-3 NoOp scaffolding; 1-5 replaces the placeholder; 2-2 depends on 2-1 having inserted the sleep; 1-8 extends 1-6's fixture). No circular dependencies.
- **Task Self-Containment**: Tasks pull spec context inline (Context blocks, code-line references, Spec Reference paths). Implementer can pick up any task without reading siblings.
- **Scope and Granularity**: Tasks are sized for one TDD cycle. Task 2-1 is the largest unit (test-flip + handler change + sleep insertion in `runHydrate` + `TestHydrate_TimeoutDoesNotSleep100ms` rename) but the components are tightly coupled around the marker-unset behaviour and cannot be split without leaving an intermediate state where the test pins the wrong invariant. Acceptable as a single coherent TDD increment.
- **Acceptance Criteria Quality**: All criteria are pass/fail with concrete shapes (argv slices, log substrings, file existence, polled timeouts, line-position assertions). No interpretation required.
- **External Dependencies**: N/A — this is a feature/bugfix unit, not an epic.

No new structural issues, no template gaps, no scope drift, no acceptance ambiguity. Plan is implementation-ready.
