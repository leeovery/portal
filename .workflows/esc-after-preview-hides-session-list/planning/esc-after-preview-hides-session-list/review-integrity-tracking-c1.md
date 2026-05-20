---
status: complete
created: 2026-05-20
cycle: 1
phase: Plan Integrity Review
topic: Esc After Preview Hides Session List
---

# Review Tracking: Esc After Preview Hides Session List - Integrity

## Findings

No findings. The plan meets structural quality and implementation-readiness standards on every reviewed dimension.

## Dimension-by-dimension assessment

1. **Task Template Compliance** — All 6 tasks carry Problem, Solution, Outcome, Do, Acceptance Criteria, Tests, Edge Cases (where relevant), Context (where relevant), and Spec Reference. Problem statements clearly explain why; Solutions describe what; Outcomes are verifiable end states; Acceptance criteria are concrete (slice-equality vs length-only is explicitly called out); Tests include edge cases (nil cmd, unfiltered guard, cursor preservation).

2. **Vertical Slicing** — Each task is an independently verifiable TDD increment: 1-1 (test-harness helper, no-op on nil), 1-2 (assertions, red test), 1-3 (fix, turns 1-2 green), 1-4 (regression test using 1-1's helper), 1-5 (shape-consistency sweep), 1-6 (audit). No horizontal layer split.

3. **Phase Structure** — Single phase, with explicit "Why this order" justification matching bugfix phase-design guidance (singular cause, surgical change, no intermediate state with independent value). Phase Acceptance section is concrete and lock-in-grade.

4. **Dependencies and Ordering** — No explicit `blocked_by` edges in the tick store, which is correct: tasks are sequential intra-phase with creation-date order matching execution order (1-1 → 1-2 → 1-3 → 1-4 → 1-5 → 1-6), and the tick reading.md natural-ordering convention specifies dependencies should only be added when the correct order differs from natural order. Per the integrity criteria, sequential intra-phase tasks in natural order do not need explicit dependencies; no convergence point requires multi-predecessor edges; no cross-phase dependencies exist (single phase).

5. **Task Self-Containment** — Every task pulls in the necessary spec excerpts (Context blocks), absolute file paths with line ranges (e.g. `internal/tui/model.go:660-668`, `internal/tui/pagepreview_refetch_test.go:76-112`), and a Spec Reference. An implementer can pick up any single task without re-reading the spec or sibling tasks.

6. **Scope and Granularity** — Each task is one TDD cycle. Task 1-5 touches two call sites (`WithInsideTmux` and `ProjectsLoadedMsg` handler) but they are shape-identical sweeps of the same lossy pattern; the spec deliberately groups them and they share one set of acceptance criteria. Not too large.

7. **Acceptance Criteria Quality** — All criteria are pass/fail with concrete artefacts (function signatures, file:line, assertion shape, exact behaviour). Edge cases (nil cmd, boot path, real-keystroke path, audit-empty-still-recorded) are codified as separate criteria where they matter.

8. **External Dependencies** — Bugfix work type; criterion not applicable.

## Notes

- Plan file: `.workflows/esc-after-preview-hides-session-list/planning/esc-after-preview-hides-session-list/planning.md`
- Task file: `.workflows/esc-after-preview-hides-session-list/planning/esc-after-preview-hides-session-list/phase-1-tasks.md`
- Tick records inspected: tick-99e7c1 (topic), tick-341b0f (phase), tick-18ed97 / tick-8f489c / tick-ffe008 / tick-87e490 / tick-f63ec9 / tick-f0dead (tasks 1-1 through 1-6).
- Tick descriptions are consistent with the markdown task file content.
