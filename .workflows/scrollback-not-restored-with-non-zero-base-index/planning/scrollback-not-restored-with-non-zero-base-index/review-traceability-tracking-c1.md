---
status: complete
created: 2026-04-30
cycle: 1
phase: Traceability Review
topic: Scrollback Not Restored With Non-Zero Base Index
---

# Review Tracking: Scrollback Not Restored With Non-Zero Base Index - Traceability

## Summary

Bidirectional traceability analysis (spec ↔ plan) found **zero findings**. The plan is a faithful, complete translation of the specification.

### Direction 1: Spec → Plan (completeness)

Every spec element has plan coverage:

- **Part 1 — `--` separator on `signalHydrateCommand`** → Task 1-1
- **Tightened `signalHydrateSubstring`** → Task 1-1
- **One-shot bootstrap migration (full mechanics: eviction API, code location, event scope, predicate, ordering, error handling, operator visibility)** → Task 1-2
- **Part 2 — Delete `PredictLiveIndices`, `flattenSavedPanePositions`, `warnOnPaneKeyDrift`, call site, `readIndexOption` if unused** → Task 2-1
- **Pre-deletion verification (repo-wide grep)** → Task 2-1
- **Test-side audit (`_test.go` sweep across the repo)** → Task 2-1
- **Acceptance Criteria #1 (hydration succeeds for leading-dash)** → Task 1-3
- **Acceptance Criteria #2 (cobra-level argv parse test)** → Task 1-1
- **Acceptance Criteria #3 (migration invariant: exactly 1 entry, idempotent)** → Task 1-2
- **Acceptance Criteria #4 (no predicted-vs-live WARN under non-zero base-index)** → Task 2-2
- **Acceptance Criteria #5 (no regression for non-dash names; `armPanes:202` preserved)** → Tasks 1-3, 2-1
- **Testing Requirements 1 (cobra-level argv parse test)** → Task 1-1
- **Testing Requirements 2 (reboot round-trip with leading-dash, real tmux)** → Task 1-3
- **Testing Requirements 3 (hook content unit test)** → Task 1-1
- **Testing Requirements 4 (migration test, real-tmux fixture preferred)** → Task 1-2
- **Testing Constraint (do not restart active tmux server; isolated socket only)** → Tasks 1-3, 2-2

### Direction 2: Plan → Spec (fidelity)

Every plan element traces back to the spec:

- All Phase 1 / Phase 2 goals tie directly to "Part 1" and "Part 2" of the spec's Fix Scope.
- All acceptance criteria map to spec ACs and Testing Requirements.
- Task implementation details (e.g., `MigrationLogger` interface, Option A/B for logger threading) are faithful elaborations of the spec's mandate to wire bootstrap-step `*state.Logger` to the migration function — no invented requirements.
- Out-of-Scope items (`SanitiseProjectName` rename, env-var session passing, `DisableFlagParsing`, repairing `PredictLiveIndices`) are correctly excluded; Task 1-1 even pins the `DisableFlagParsing` exclusion explicitly.
- Inclusion of `savedPanePos` struct in Task 2-1's deletion list is a reasonable transitive cleanup (it is the helper type for `flattenSavedPanePositions`, whose deletion is mandated).

## Findings

None.

## Resolution

No fixes required. Traceability is sound.
