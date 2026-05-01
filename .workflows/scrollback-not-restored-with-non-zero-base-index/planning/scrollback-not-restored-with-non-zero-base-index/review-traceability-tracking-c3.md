---
status: complete
created: 2026-04-30
cycle: 3
phase: Traceability Review
topic: Scrollback Not Restored With Non-Zero Base Index
---

# Review Tracking: Scrollback Not Restored With Non-Zero Base Index - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification after cycle 2's updates.

### Cycle 2 Update Verification

The two cycle-2 changes mentioned in the orchestrator prompt are present and correctly grounded:

1. **Task 1-2 AC explicitly names `RegisterPortalHooksWithLogger` as the aggregator.** Acceptance criterion: *"A `ShowGlobalHooks` failure causes `migrateHydrationHooks` to return a wrapped error; the caller in `RegisterPortalHooksWithLogger` aggregates it via `errors.Join` alongside any per-event register errors..."* — traces to spec § "Migration mechanics (explicit)" → "Error handling" ("Eviction is best-effort... the install is itself idempotent... Eviction failures must not abort bootstrap; they surface as warnings only"). The named aggregator is the same logger-aware sibling introduced in cycle 1, so no orphan references.

2. **Task 2-2 Tests section names `TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning`.** The named regex unit test verifies the spec-mandated regex from AC #4 (`predicted=.*__\d+\.\d+ live=.*__\d+\.\d+`) and explicitly excludes the preserved `armPanes:202` shape, which is the spec's "coarser but consistent signal" replacement. Both legs trace directly to spec content (AC #4 + Part 2 "Rationale for deletion over repair").

### Direction 1 (Spec → Plan) — completeness

All spec elements remain represented in the plan with sufficient depth:

- **Primary Root Cause** (leading-dash argv parse) → tasks 1-1 and 1-3.
- **Secondary Root Cause** (`PredictLiveIndices` reads wrong tmux option scope) → tasks 2-1 and 2-2.
- **Part 1 — `--` separator + dedupe substring + doc comment** → task 1-1.
- **Part 1 — One-shot bootstrap migration** (eviction API, code location, hook event scope, eviction predicate, ordering, error handling, operator visibility) → task 1-2.
- **Part 2 — Deletion list and pre-deletion verification + test-side audit** → task 2-1.
- **Acceptance Criteria 1–5** → mapped to phase- and task-level acceptance items.
- **Testing Requirements 1–4** → mapped to named tests in tasks 1-1, 1-2, 1-3, 2-2.
- **Testing Constraint — Do Not Restart The Active Tmux Server** → tasks 1-3 and 2-2 edge cases and acceptance criteria.
- **Out of Scope** items → not contradicted by any plan content (task 1-1 explicitly notes the rejection of `DisableFlagParsing` and the intentional non-fix for manual leading-dash invocations).

### Direction 2 (Plan → Spec) — fidelity

Every plan element continues to trace back to the specification:

- `RegisterPortalHooksWithLogger` + `MigrationLogger` (task 1-2) — implementation bridge for spec-mandated INFO/WARN log emissions to `portal.log` while honouring spec-mandated code location (`internal/tmux/hooks_register.go`).
- `errors.Join` aggregation in task 1-2 — implementation detail consistent with spec § "Error handling".
- Bootstrap-adapter wiring referenced in tasks 1-2 and 1-3 — traces to spec § "Production-side reading aid".
- Negative cobra sub-case in task 1-1 (asserting `unknown shorthand flag` without `--`) — regression guard tied to the spec's empirical verification block.
- Defensive fallback in task 1-3 (tmux CLI rejecting leading-dash at argv layer → `--` on the tmux command) — direct extension of the spec's central concern.
- `savedPanePos` struct in task 2-1's grep list — transitive deletion target for `flattenSavedPanePositions`, satisfying the spec's "no dead test scaffolding remains" requirement.
- `TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning` (task 2-2) — verifies the regex shape the spec itself specifies in AC #4; not invented behaviour.

No hallucinated content was introduced by cycle 2's edits. The plan remains traceable in both directions.
