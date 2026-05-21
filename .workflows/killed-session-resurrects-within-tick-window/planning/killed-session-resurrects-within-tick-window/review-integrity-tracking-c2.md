---
status: complete
created: 2026-05-21
cycle: 2
phase: Plan Integrity Review
topic: killed-session-resurrects-within-tick-window
---

# Review Tracking: Killed Session Resurrects Within Tick Window - Integrity

## Findings

No findings. The plan meets structural quality standards and is implementation-ready.

## Review Summary

Reviewed the plan end-to-end against all eight integrity criteria.

- **Task Template Compliance**: Every task carries Problem, Solution, Outcome, Do, Acceptance Criteria, Tests, Edge Cases, Context, and Spec Reference. All Problem statements explain WHY; Solutions describe WHAT; Outcomes describe success state. Acceptance criteria are concrete and pass/fail.
- **Vertical Slicing**: The phase is a single deliberate vertical slice (sync commit-now subcommand + hook migration shipping together). Within the phase, Tasks 1-1/1-2/1-3 layer behaviour onto the same file but each delivers a verifiable increment (happy path; marker short-circuit; failure-path discipline). Tasks 1-4..1-7 are independently testable.
- **Phase Structure**: Single phase is appropriate — the bug has one root cause and one structural fix. Phase Goal, Why, and Acceptance are well-articulated.
- **Dependencies and Ordering**: Intra-phase tasks execute in natural ID order with no cross-phase dependencies. No convergence points lack explicit edges (1-5/1-6/1-7 follow 1-1..1-4 in natural order). No circular dependencies. Per criteria rule, missing explicit deps for sequential intra-phase tasks are not flagged.
- **Task Self-Containment**: Each task pulls relevant spec quotations into its Context block; the Do sections cite specific files (`cmd/state_commit_now.go`, `internal/tmux/hooks_register.go`) and specific primitives (`state.IsRestoringSet`, `state.CaptureStructure`, `state.Commit`, `ShowGlobalHooks`, `AppendGlobalHook`, `UnsetGlobalHookAt`).
- **Scope and Granularity**: All seven tasks fit one TDD cycle. Task 1-4 is the largest but remains structurally cohesive (single seam: `RegisterPortalHooks` for `session-closed`). No task is mechanical boilerplate.
- **Acceptance Criteria Quality**: All criteria are objectively verifiable. The cycle-1 fix on Task 1-5 successfully restated the diagnostic requirement as a code-inspection criterion on the timeout branch's `t.Fatalf` payload.
- **External Dependencies**: N/A (bugfix, not epic).

## Cycle 1 Fixes Verified

- **Task 1-2** — Do bullet now reads "call `state.IsRestoringSet(client)` (the canonical accessor used by the daemon's `tick()` body) … Wire it through `commitNowDeps`". The ambiguous "or call `IsRestoringSet` directly" alternative is removed; the implementer has a single concrete instruction.
- **Task 1-5** — The aspirational test case `"it fails with a deadlock-diagnostic message rather than a silent timeout"` is removed. The diagnostic requirement is preserved as an Acceptance Criterion phrased as code-inspection of the timeout branch's `t.Fatalf` arguments (state-dir contents, live tmux session/pane list, elapsed wall time). This is verifiable without an artificial hang injection.

## Deliberate Spec Gap (not a finding)

Task 1-3 retains an explicit `[needs-info]` block in the Do section covering the `IsRestoringSet` query failure mode (fail-open vs fail-closed when the `@portal-restoring` read itself errors). The user has confirmed this is a deliberate spec gap to be resolved at the specification level before this task is implemented. The plan correctly flags this rather than speculating, so no finding is raised.
