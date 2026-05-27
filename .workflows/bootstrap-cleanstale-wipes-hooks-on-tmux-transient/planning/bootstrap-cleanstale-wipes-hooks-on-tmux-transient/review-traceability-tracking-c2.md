---
status: complete
created: 2026-05-27
cycle: 2
phase: Traceability Review
topic: Bootstrap CleanStale Wipes Hooks On Tmux Transient
---

# Review Tracking: Bootstrap CleanStale Wipes Hooks On Tmux Transient - Traceability (Cycle 2)

## Summary

Cycle-2 bi-directional trace complete. Cycle-1 findings have been applied to the plan:

- **Cycle-1 Finding 1** (Task 2-3 `runCleanStale` free-function refactor narrowed to test-local `cleanStaleAdapterT`) — verified at `phase-2-tasks.md` Task 2-3 Solution paragraph (line 174) and Do step 5 (line 204). The plan now explicitly states "No production refactor" and "Do not refactor the production adapter — the test-local type is the seam."
- **Cycle-1 Finding 2** (whitespace-only stdout subtest kept as defensive coverage) — verified at `phase-1-tasks.md` Task 1.2 Do step 2 (line 83-86). Retained as recommended; the empty-stdout subtest carries the mode-(a)-vs-mode-(b) distinguishability annotation that grounds the defensive variant.
- **Cycle-1 Integrity Finding 1** (Task 3-2 `commanderFactory` seam mandate) — verified at `phase-3-tasks.md` Task 3-2 Do step 6 (lines 89-106). The plan now explicitly mandates the commanderFactory test-only mutable package-level var following the `cleanDeps` / `bootstrapDeps` pattern; Option B (subprocess) rejected; wiring caveat documented.

## Direction 1 (Spec → Plan) — Completeness

Every specification element has plan coverage:

| Spec element | Plan coverage |
|---|---|
| Problem / Defect / Expected Behavior | Phase 1 (mode a) + Phase 2 (mode b) goals |
| Failure Mode (a) — exit ≠ 0 | Task 1-1 (helper propagates), Task 2-2/2-4 (adapter Warn) |
| Failure Mode (b) — exit 0 empty stdout | Task 1-2 (legitimate-empty pin), Task 2-2/2-4 (hazard guard) |
| Defect Class Scope audit (2 callsites) | Tasks 2-2 (bootstrap), 2-4 (portal clean) |
| Audited Reference Set (step-9 prior art) | Tasks 2-2, 2-4 Context blocks |
| Change 1 (repurpose ListAllPanes + docstring + return contract) | Task 1-1 |
| Change 2 (no parseLivePaneSet promotion) | Honoured implicitly — Task 1-1 reuses `parsePaneOutput` |
| Change 3 (hazard guard, Logger plumbing, comment lift, docstring rewrite, soft-warning) | Tasks 2-1, 2-2, 2-4 |
| Change 4 (entry-point Debug + mutually-exclusive terminal lines + portal-clean early-exit) | Tasks 2-2, 2-4 |
| Bootstrap Posture Preserved | Task 3-2 acceptance + Task 2-2 spec ref |
| Closing Both Failure Modes table | Task 1-1 + Tasks 2-2/2-4 |
| Test Requirements §New File (4 subtests) | Task 2-3 |
| Test Requirements §Inverted Subtest | Task 2-5 |
| Test Requirements §Deterministic Repro Mechanism | Task 3-1 |
| Integration — Tmux Transient Simulation | Task 3-2 |
| Integration — portal clean Analogue | Task 3-3 |
| Regression — non-empty live sets | Phase 1 acceptance, Task 2-3 legitimate-stale-removal subtest, Tasks 3-2/3-3 normal-path subtests |
| Coverage Matrix (7 rows) | All rows mapped to tasks (2-3 × 4 rows, 2-5 × 1, 3-2 × 1, 3-3 × 1) |
| Acceptance Criteria 1-6 | Mapped via phase acceptance bullets + per-task acceptance criteria |
| Out of Scope items | Correctly absent from plan |

Depth of coverage is sufficient — each task carries Problem/Solution/Outcome plus a Context block lifting the relevant spec excerpts so an implementer need not page back to the spec for routine work.

## Direction 2 (Plan → Spec) — Fidelity

Every task's Problem, Solution, Outcome, Do steps, Acceptance Criteria, Tests, and Edge Cases trace to a specific spec section:

- Task 1-1 — Change 1 (verbatim).
- Task 1-2 — Failure Modes Covered + Closing Both Failure Modes + Change 1 return-contract. The whitespace-only-stdout subtest is defensive coverage explicitly accepted in Cycle 1 Finding 2.
- Task 2-1 — Change 3 Logger plumbing (bootstrap adapter) + Audited Reference Set.
- Task 2-2 — Change 3 hazard-guard algorithm + Load() error handling + adapter docstring rewrite + load-bearing comment lift; Change 4 terminal-line shapes. The "six-branch" enumeration in the plan is a structural decomposition of Change 3 + Change 4 (5 spec-named branches plus the implied Save-error fall-through); every branch is traceable.
- Task 2-3 — Test Requirements §New File (four named subtests); Cycle-1 Finding 1 application verified.
- Task 2-4 — Change 3 hazard-guard at portal clean + Logger plumbing for portal clean + soft-warning RunE-returns-nil; Change 4 portal-clean early-exit special case.
- Task 2-5 — Test Requirements §Inverted Subtest + structural-preserve-flip-assert shape from commit `7e33c04b`.
- Task 3-1 — Test Requirements §Deterministic Repro Mechanism + Integration headings. Helper signatures are derived from spec-driven needs (Commander interception, hooks-json seed, log tailing) — all explicitly required for Tasks 3-2/3-3.
- Task 3-2 — Test Requirements §Integration — Tmux Transient Simulation + Acceptance Criteria items 1, 2, 4, 5 + Coverage Matrix row "Tmux transient end-to-end" + Change 4 distinguishability. The `commanderFactory` production seam is a plan-introduced test seam; it was explicitly mandated in Cycle 1 Integrity Finding 1 (matches CLAUDE.md `cleanDeps`/`bootstrapDeps` convention) and the user approved it. The "normal_path_legitimate_stale_removal_still_works" subtest is grounded in §Regression non-empty live sets + Coverage Matrix.
- Task 3-3 — Test Requirements §Integration — portal clean Analogue + Coverage Matrix row "portal clean transient end-to-end" + Change 3 soft-warning contract + Change 4 portal-clean early-exit. The "persisted_empty_early_exit_emits_breadcrumb" subtest is grounded in Change 4 early-exit special case. Option A (in-process) selected over Option B (subprocess) with rationale documented.

No hallucinated content remains. No invented edge cases. No acceptance criteria testing un-spec'd behaviour.

## Findings

No new traceability findings in cycle 2. The plan is a faithful, complete translation of the specification.

Status: **clean**.
