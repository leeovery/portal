---
status: complete
created: 2026-04-30
cycle: 2
phase: Traceability Review
topic: Scrollback Not Restored With Non-Zero Base Index
---

# Review Tracking: Scrollback Not Restored With Non-Zero Base Index - Traceability

## Findings

No findings. The plan is a faithful, complete translation of the specification.

### Direction 1 (Spec → Plan) — completeness

Every spec element is represented in the plan with sufficient depth:

- **Primary Root Cause** (leading-dash argv parse) → task 1-1 (constant edit + cobra Execute test) and task 1-3 (end-to-end reboot round-trip).
- **Secondary Root Cause** (PredictLiveIndices wrong scope, diagnostic-only) → task 2-1 (excise) and task 2-2 (runtime regression assertion).
- **Part 1 — `--` separator** → task 1-1, with the constant change, the `signalHydrateSubstring` tightening, and the doc-comment update covered.
- **Part 1 — One-shot bootstrap migration mechanics** (eviction API, code location, hook event scope, eviction predicate, ordering, error handling, operator visibility) → task 1-2, all six sub-bullets reflected in the Do/Acceptance Criteria.
- **Part 2 — Deletion list** (`PredictLiveIndices`, `flattenSavedPanePositions`, `readIndexOption` if unused, `warnOnPaneKeyDrift`, call site, test scaffolding) → task 2-1, with explicit grep step and pre-deletion verification.
- **Acceptance Criteria 1–5** all map to phase- or task-level acceptance items.
- **Testing Requirements 1–4** all map to named tests in tasks 1-1, 1-2, 1-3, 2-2.
- **Testing Constraint** (do not kill primary tmux server) → reflected in tasks 1-3 and 2-2 edge cases.

### Direction 2 (Plan → Spec) — fidelity

Every plan element traces back to the specification:

- The `RegisterPortalHooksWithLogger` sibling and small `MigrationLogger` interface introduced in task 1-2 are an implementation bridge required to satisfy the spec's mandated INFO/WARN log emissions to `portal.log` while keeping `internal/tmux` free of an `internal/state` import dependency. The spec mandates both the log lines and the code location (`internal/tmux/hooks_register.go`); the sibling shape is the minimal mechanism that honours both.
- The bootstrapadapter wiring referenced in tasks 1-2 and 1-3 traces to spec § "Production-side reading aid".
- The `savedPanePos` struct added to Phase 2's grep acceptance is a transitive deletion target: it is used only by `flattenSavedPanePositions` (which the spec deletes), and including it in the grep ensures the "no dead test scaffolding remains" outcome the spec explicitly requires.
- The `TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning` unit test in task 2-2 verifies the correctness of the regex the spec mandates in AC #4. It does not invent new behaviour — it validates the regex shape the spec itself specifies.
- The `errors.Join` aggregation choice for `ShowGlobalHooks` failure in task 1-2 is an implementation detail consistent with the spec's "the caller decides whether to surface as warning or fatal" guidance.
- Defensive concerns in task 1-3 step 6 (tmux CLI rejecting leading-dash names at the argv layer, falling back to `--` on the tmux command) are a direct extension of the spec's central concern (leading-dash names breaking argv parsers); not invented behaviour.

### Cycle 1 Fix Verification

The five integrity findings applied since cycle 1 were re-examined for new traceability issues:

1. **Task 1-2 logger seam (`RegisterPortalHooksWithLogger` sibling + `MigrationLogger` interface)** — bridges spec-mandated logging (INFO/WARN to `portal.log`) without violating spec-mandated code location. No new traceability gap.
2. **Task 1-3 production-adapter clarification** — explicitly references the new sibling and `internal/bootstrapadapter` wiring; both trace to spec § "Production-side reading aid".
3. **Task 2-2 regex unit test** — verifies the regex from spec AC #4. Defensive but spec-grounded.
4. **Phase 2 AC `savedPanePos` addition** — transitive deletion target tied to `flattenSavedPanePositions`. Spec-consistent.
5. **Task 1-3 step 11 wording referencing `RegisterPortalHooksWithLogger`** — flows from cycle-1 fix #1; no orphaned reference.

No new content was introduced that lacks specification grounding.
