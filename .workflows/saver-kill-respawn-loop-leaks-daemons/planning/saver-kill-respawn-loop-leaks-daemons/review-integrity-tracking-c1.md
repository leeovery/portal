---
status: in-progress
created: 2026-05-19
cycle: 1
phase: Plan Integrity Review
topic: saver-kill-respawn-loop-leaks-daemons
---

# Review Tracking: Saver Kill-Respawn Loop Leaks Daemons - Integrity

## Findings

### 1. Task 1-1 is missing the required `Tests` field

**Severity**: Minor
**Plan Reference**: Phase 1, Task `saver-kill-respawn-loop-leaks-daemons-1-1` (tick-c911bf)
**Category**: Task Template Compliance
**Change Type**: add-to-task

**Details**:
Per `task-design.md`, every task must include a `Tests` field listing test names ("At least one test name; include edge cases, not just happy path"). Task 1-1's description contains Problem, Solution, Outcome, Do, Acceptance Criteria, Edge Cases, and Spec Reference, but has no explicit `Tests:` block.

The six predicate cases are spelled out inside `Do` as a numbered case list, so the test content is effectively present — but the canonical `Tests` field is absent. Every other task in both phases (1-2, 1-3, 1-4, 1-6, 2-1, 2-2, 2-3, 2-4, 2-5, 2-6) carries an explicit `Tests:` block; only Task 1-5 omits it, and 1-5 documents the omission ("Tests: None — this is a comment-only edit"). Task 1-1 should follow the same explicit pattern.

Impact is low because the test cases are recoverable from `Do`, but adding the explicit `Tests` field brings the task into template compliance and makes the test surface greppable alongside the rest of the plan.

**Current**:
> (Task 1-1 description, after the Acceptance Criteria block — the `Edge Cases` section immediately follows Acceptance Criteria with no intervening `Tests:` section.)
>
> Acceptance Criteria:
> - The renamed test exists in internal/tmux/portal_saver_test.go and runs under go test ./internal/tmux/...
> - All six cases above are present, each with a distinct case name, and each pins the expected boolean.
> - The leading comment block does not assert 'absent counts as version mismatch' as a load-bearing contract; instead it documents the predicate-as-one-input framing.
> - go test ./internal/tmux/... is green.
> - No production code in internal/tmux/portal_saver.go is modified by this task.
>
> Edge Cases:
> - stored="" paired with ErrVersionFileAbsent vs stored="" paired with non-absent I/O error must be distinguishable cases — do not collapse them.
> - Dev cases must assert true even though they superficially 'match' the absent-case behaviour; the reason differs (dev short-circuit vs unreadable file), and case names should reflect that.
> - The reframed comment must not introduce ambiguity: if a reader's first question is 'why does absent return true if it's no longer load-bearing?', the comment should answer it (predicate's verdict is consumed by an alive-check gate in EnsurePortalSaverVersion).

**Proposed**:
> Acceptance Criteria:
> - The renamed test exists in internal/tmux/portal_saver_test.go and runs under go test ./internal/tmux/...
> - All six cases above are present, each with a distinct case name, and each pins the expected boolean.
> - The leading comment block does not assert 'absent counts as version mismatch' as a load-bearing contract; instead it documents the predicate-as-one-input framing.
> - go test ./internal/tmux/... is green.
> - No production code in internal/tmux/portal_saver.go is modified by this task.
>
> Tests:
> - 'predicate returns false when stored and current versions match (neither dev)'
> - 'predicate returns true when stored != current and neither side is dev (real mismatch)'
> - 'predicate returns true when read returns ErrVersionFileAbsent (predicate-layer only; alive-check gates kill in caller)'
> - 'predicate returns true when read returns a non-absent I/O error (e.g. fs.ErrPermission)'
> - 'predicate returns true when stored == "dev" (dev short-circuit preserved)'
> - 'predicate returns true when current == "dev" (dev short-circuit preserved)'
> - 'case names distinguish absent_neither_dev_predicate_layer_only from non_absent_io_read_error so the two are not collapsed'
> - 'leading comment block documents predicate-as-one-input framing and does not assert absent-counts-as-mismatch as load-bearing'
>
> Edge Cases:
> - stored="" paired with ErrVersionFileAbsent vs stored="" paired with non-absent I/O error must be distinguishable cases — do not collapse them.
> - Dev cases must assert true even though they superficially 'match' the absent-case behaviour; the reason differs (dev short-circuit vs unreadable file), and case names should reflect that.
> - The reframed comment must not introduce ambiguity: if a reader's first question is 'why does absent return true if it's no longer load-bearing?', the comment should answer it (predicate's verdict is consumed by an alive-check gate in EnsurePortalSaverVersion).

**Resolution**: Pending
**Notes**:

---
