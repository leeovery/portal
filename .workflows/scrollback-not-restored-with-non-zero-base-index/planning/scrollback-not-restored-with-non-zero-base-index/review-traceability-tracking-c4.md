---
status: complete
created: 2026-04-30
cycle: 4
phase: Traceability Review
topic: Scrollback Not Restored With Non-Zero Base Index
---

# Review Tracking: Scrollback Not Restored With Non-Zero Base Index - Traceability

## Findings

No findings. The plan remains a faithful, complete translation of the specification after cycle 3's propagation fixes.

### Cycle 3 Update Verification

The two cycle-3 propagation fixes in task 1-2's Tests section are present and correctly grounded:

1. **`TestMigrateHydrationHooks_EvictsUnSeparatedThenInstallsFixed`** now invokes `RegisterPortalHooksWithLogger(c, capturingLogger)` (not the no-op-logger wrapper) and explicitly notes "The no-op-logger wrapper `RegisterPortalHooks` is exercised separately by the bootstrap-adapter wiring; this test must use the logger-aware sibling so the captured-output assertions are reachable." This propagates cleanly from cycle 1's introduction of the logger-aware sibling and traces to spec § "Operator visibility" (INFO/WARN must reach `portal.log`).

2. **`TestMigrateHydrationHooks_IdempotentNoOpOnSecondBootstrap` and `TestMigrateHydrationHooks_ZeroPreExistingEntriesIsSilentNoOp`** also now invoke `RegisterPortalHooksWithLogger(c, capturingLogger)` rather than the no-op wrapper. Same propagation logic; same spec grounding (the "silent no-op" behaviour the spec requires for the steady-state path is only assertable through a capturing logger, which the no-op wrapper deliberately discards).

Both fixes are pure consistency propagation — they do not introduce any new behaviour or content that lacks specification grounding.

### Direction 1 (Spec → Plan) — completeness

Every spec element remains represented in the plan with sufficient depth:

- **Problem & Root Cause — Observed Symptom + Reproduction** → tasks 1-1 (problem framing), 1-3 (deterministic reproduction via leading-dash session name on non-zero base indices).
- **Primary Root Cause** (leading-dash argv parse) → tasks 1-1 and 1-3.
- **Secondary Root Cause** (`PredictLiveIndices` wrong scope, diagnostic-only) → tasks 2-1 and 2-2.
- **Why the End-to-End Path Otherwise Works** → preserved as the implicit non-regression contract in task 1-3's "non-dash session names still hydrate" check and task 2-1's `armPanes:202` preservation.
- **Blast Radius** → task 1-1 acknowledges the manual leading-dash CLI invocation case is intentionally out of scope (spec's stated position), no plan content silently widens scope.
- **Part 1 — `--` separator + dedupe substring + doc comment** → task 1-1.
- **Part 1 — One-shot bootstrap migration mechanics** (eviction API, code location, hook event scope, eviction predicate, ordering, error handling, operator visibility) → task 1-2.
- **Part 2 — Deletion list, pre-deletion verification, test-side audit, rationale for deletion over repair** → task 2-1.
- **Out of Scope** items (rename `SanitiseProjectName`, env-var dispatch, `DisableFlagParsing`, repair `PredictLiveIndices`) → task 1-1 explicitly notes rejection of `DisableFlagParsing`; task 2-1 reflects "delete over repair"; no plan content contradicts the other excluded approaches.
- **Acceptance Criteria 1–5** → mapped to phase- and task-level acceptance items.
- **Testing Requirements 1–4** → mapped to named tests across tasks 1-1, 1-2, 1-3, 2-2.
- **Testing Constraint — Do Not Restart The Active Tmux Server** → reflected in tasks 1-3 and 2-2 edge cases and acceptance criteria.

### Direction 2 (Plan → Spec) — fidelity

Every plan element continues to trace back to the specification:

- `RegisterPortalHooksWithLogger` + `MigrationLogger` interface (task 1-2) — implementation bridge for spec-mandated INFO/WARN log emissions while honouring spec-mandated code location.
- `errors.Join` aggregation in task 1-2 — consistent with spec § "Error handling" (best-effort eviction, install proceeds regardless).
- Bootstrap-adapter wiring referenced in tasks 1-2 and 1-3 — traces to spec § "Production-side reading aid".
- Negative cobra sub-case in task 1-1 (asserting `unknown shorthand flag` without `--`) — regression guard tied to the spec's empirical verification block.
- Defensive fallback in task 1-3 (tmux CLI rejecting leading-dash session names at the argv layer → `--` on the tmux command) — direct extension of the spec's central concern about argv parsers and leading-dash tokens.
- `savedPanePos` struct in task 2-1's grep list — transitive deletion target for `flattenSavedPanePositions`; satisfies the spec's "no dead test scaffolding remains" requirement.
- `TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning` (task 2-2) — verifies the regex shape the spec itself specifies in AC #4 and confirms it does not collide with the preserved `armPanes:202` shape.
- The cycle-3 propagation fixes (see Cycle 3 Update Verification above) — pure consistency edits; no invented behaviour.

No hallucinated content was introduced by cycle 3's edits. The plan remains traceable in both directions and is ready for implementation.
