---
status: complete
created: 2026-05-21
cycle: 2
phase: Traceability Review
topic: Killed Session Resurrects Within Tick Window
---

# Review Tracking: Killed Session Resurrects Within Tick Window - Traceability

## Findings

No findings. The plan is a faithful, complete translation of the specification.

### Direction 1 (Spec → Plan) — Complete

Every specification element is represented:

- Problem Statement / Required Behavior → Phase Acceptance items 1-3, Task 1-1 Problem
- Fix Approach / Mechanism (steps 1-4, PrevIndex fallback) → Task 1-1
- Entry-Point Design Decision (`portal state commit-now` sibling) → Task 1-1
- Hook Registration Migration (commitNowCommand, Replace strategy, exact-string algorithm, idempotency) → Task 1-4
- `@portal-restoring` Defence → Task 1-2
- Logging Discipline (state logger, daemon Component constant) → Tasks 1-1, 1-2, 1-3
- `_portal-saver` Self-Kill (Timeline 1 marker set, Timeline 2 marker clear, underscore filter) → Task 1-6 sub-tests 2 and 3
- Hook Re-entrancy (real-tmux gate, fail-loudly, spec-pivot signal) → Task 1-5
- Daemon Merge Interaction (verified safe; semantic-set invariant) → Task 1-7 regression test 1
- `commit-now` Failure Behaviour → Task 1-3
- `save.requested` Discipline (touch on every exit path except success) → Tasks 1-1, 1-2, 1-3
- Exit Code Summary → Tasks 1-2, 1-3
- `save.requested` Touch Failure Handling → Tasks 1-2, 1-3
- Acceptance Criteria 1-13 → Phase Acceptance bullets
- Testing Requirements (unit, integration, regression) → Tasks 1-1 through 1-7
- Daemon vs Hook narrow race / Concurrent invocations / Scrollback .bin ownership / Consumer-side untouched → spec-level documentation, correctly require no code or are covered by explicit "do not touch" clauses

### Direction 2 (Plan → Spec) — Clean

Every plan element traces back to the specification. No hallucinated content found.

- Task 1-1 mechanism, deps struct, PrevIndex fallback, no-bin-write/no-marker-touch → spec § Mechanism, § Logging Discipline, § save.requested Discipline
- Task 1-2 IsRestoringSet read, INFO log, best-effort save.requested touch → spec § @portal-restoring Defence, § save.requested Discipline, § save.requested Touch Failure Handling
- Task 1-3 ERROR log, save.requested fallback, non-zero exit, no panic, `[needs-info]` on IsRestoringSet query failure → spec § commit-now Failure Behaviour, § save.requested Touch Failure Handling, § Exit Code Summary
- Task 1-4 const literal, scan-and-remove, exact-string match, highest-index-first, idempotency, six-events untouched → spec § Hook Registration Migration > Migration Algorithm, § Registration Strategy
- Task 1-5 real-tmux fixture, 1.5s bound, fail-loudly diagnostic, spec-pivot signal → spec § Hook Re-entrancy
- Task 1-6 canonical symptom + _portal-saver marker-clear/marker-set sub-tests + marker-clear continuation → spec § Testing Requirements > Integration Tests, § _portal-saver Self-Kill, § Why session-closed Is The Right Hook
- Task 1-7 semantic-set invariant on daemon next-tick + six-event eventual-consistency assertions → spec § Daemon Merge Interaction, § Testing Requirements > Regression Tests, § Acceptance Criteria items 9-11, 13

### Cycle 1 Follow-Up Verification

- Task 1-6 sub-test 3 marker-clear continuation (steps 7-9 plus matching acceptance criterion and named assertion) → present and traces to spec § Testing Requirements > Integration Tests bullet "`@portal-restoring` defence under real tmux" ("Clear the marker, fire `session-closed` again, assert the file updates correctly").
- Task 1-3 IsRestoringSet query failure mode demoted to `[needs-info]` with explicit "Resolve in spec before implementation" prompt and matching edge-case entry → present and correctly identifies the spec gap rather than inventing fail-open/fail-closed behaviour.
