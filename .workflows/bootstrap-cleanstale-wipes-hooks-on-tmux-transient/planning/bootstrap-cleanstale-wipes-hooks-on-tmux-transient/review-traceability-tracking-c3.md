---
status: complete
created: 2026-05-27
cycle: 3
phase: Traceability Review
topic: Bootstrap CleanStale Wipes Hooks On Tmux Transient
---

# Review Tracking: Bootstrap CleanStale Wipes Hooks On Tmux Transient - Traceability (Cycle 3)

## Summary

Cycle-3 bi-directional trace complete against the post-cycle-2 plan. No new findings. Cycle-2 was already clean; cycle-2's only edit was the integrity-tracked three-value `Run` destructure fix in Task 3-2 (Do step 6, lines 105-108) — verified present and correctly destructures `(serverStarted, warnings, err)` matching `(*bootstrap.Orchestrator).Run` at `cmd/bootstrap/bootstrap.go:248`.

## Direction 1 (Spec → Plan) — Completeness

Every specification element has plan coverage:

| Spec element | Plan coverage |
|---|---|
| Problem / Defect / Expected Behavior | Phase 1 + Phase 2 goals |
| Failure Mode (a) — exit ≠ 0 | Task 1-1 (helper propagates), Tasks 2-2 / 2-4 (adapter Warn + return) |
| Failure Mode (b) — exit 0 empty stdout | Task 1-2 (legitimate-empty pin), Tasks 2-2 / 2-4 (hazard guard) |
| Defect Class Scope audit (2 callsites) | Tasks 2-2 (bootstrap), 2-4 (portal clean) |
| Audited Reference Set (step-9 prior art) | Tasks 2-1, 2-2, 2-4 Context blocks |
| Change 1 (repurpose `ListAllPanes` + docstring + return-value contract) | Task 1-1 |
| Change 2 (no `parseLivePaneSet` promotion) | Honoured — Task 1-1 reuses `parsePaneOutput`, no parser promotion task introduced |
| Change 3 (hazard guard, Logger plumbing, comment lift, docstring rewrite, soft-warning surfacing) | Tasks 2-1, 2-2, 2-4 |
| Change 4 (entry-point Debug + mutually-exclusive terminal lines + portal-clean early-exit Debug) | Tasks 2-2, 2-4 |
| Bootstrap Posture Preserved | Task 3-2 acceptance + Task 2-2 spec ref |
| Closing Both Failure Modes table | Task 1-1 + Tasks 2-2 / 2-4 |
| Test Requirements §New File (4 subtests) | Task 2-3 |
| Test Requirements §Inverted Subtest | Task 2-5 |
| Test Requirements §Deterministic Repro Mechanism | Task 3-1 |
| Integration — Tmux Transient Simulation | Task 3-2 |
| Integration — `portal clean` Analogue | Task 3-3 |
| Regression — non-empty live sets | Phase 1 acceptance, Task 2-3 legitimate-stale-removal subtest, Tasks 3-2 / 3-3 normal-path subtests |
| Coverage Matrix (7 rows) | All rows mapped (Task 2-3 × 4 rows, Task 2-5 × 1, Task 3-2 × 1, Task 3-3 × 1) |
| Acceptance Criteria 1-6 | Mapped via phase acceptance bullets + per-task acceptance criteria |
| Out of Scope items | Correctly absent from plan |

Depth of coverage is sufficient — each task carries Problem / Solution / Outcome plus a Context block lifting the relevant spec excerpts so an implementer need not page back to the spec for routine work.

## Direction 2 (Plan → Spec) — Fidelity

Every task's Problem, Solution, Outcome, Do steps, Acceptance Criteria, Tests, and Edge Cases trace to a specific spec section:

- **Task 1-1** — Change 1 (verbatim repurpose, docstring rewrite, return-contract narrowing).
- **Task 1-2** — Failure Modes Covered §(b) + Closing Both Failure Modes + Change 1 return-contract. The whitespace-only-stdout subtest is defensive coverage explicitly accepted in Cycle 1 Finding 2.
- **Task 2-1** — Change 3 Logger plumbing (bootstrap adapter) + Audited Reference Set (`MarkerCleanupCore` as canonical prior-art).
- **Task 2-2** — Change 3 hazard-guard algorithm + `Load()` error handling + adapter docstring rewrite + load-bearing comment lift; Change 4 terminal-line shapes. The six-branch enumeration is a structural decomposition of Change 3 + Change 4 (5 spec-named branches plus the implied `Save`-error fall-through); every branch is traceable.
- **Task 2-3** — Test Requirements §New File (four named subtests). Test-local `cleanStaleAdapterT` shape verified post-Cycle 1 Finding 1; "No production refactor" explicit.
- **Task 2-4** — Change 3 hazard-guard at `portal clean` + Logger plumbing for `portal clean` + soft-warning `RunE`-returns-nil; Change 4 portal-clean early-exit special case.
- **Task 2-5** — Test Requirements §Inverted Subtest + structural-preserve-flip-assert shape from commit `7e33c04b`.
- **Task 3-1** — Test Requirements §Deterministic Repro Mechanism + Integration headings. Helper signatures derived from spec-driven needs (Commander interception, hooks-json seed, log tailing) — all required for Tasks 3-2 / 3-3.
- **Task 3-2** — Test Requirements §Integration — Tmux Transient Simulation + Acceptance Criteria items 1, 2, 4, 5 + Coverage Matrix row "Tmux transient end-to-end" + Change 4 distinguishability. The `commanderFactory` production seam was approved in Cycle 1 Integrity Finding 1 (matches CLAUDE.md `cleanDeps` / `bootstrapDeps` convention). The cycle-2 three-value `Run` destructure fix is verified at Do step 6 (lines 105-108): `(serverStarted, warnings, err)` with the `_`-discard rationale documented. The "normal_path_legitimate_stale_removal_still_works" subtest is grounded in §Regression non-empty live sets + Coverage Matrix.
- **Task 3-3** — Test Requirements §Integration — `portal clean` Analogue + Coverage Matrix row "portal clean transient end-to-end" + Change 3 soft-warning contract + Change 4 portal-clean early-exit. The "persisted_empty_early_exit_emits_breadcrumb" subtest is grounded in Change 4 early-exit special case. Option A (in-process) selected over Option B (subprocess) with rationale documented.

No hallucinated content remains. No invented edge cases. No acceptance criteria testing un-spec'd behaviour.

## Findings

No new traceability findings in cycle 3. The plan is a faithful, complete translation of the specification.

Status: **clean**.
