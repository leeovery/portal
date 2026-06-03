---
status: in-progress
created: 2026-06-03
cycle: 1
phase: Traceability Review
topic: State Notify Cascade on Binary Upgrade
---

# Review Tracking: State Notify Cascade on Binary Upgrade - Traceability

## Summary

Bidirectional traceability analysis of the plan against its specification.

**Direction 1 (Spec → Plan, completeness):** Every spec element traces to at
least one task with adequate implementer-level depth — Solution Strategy, the
per-event convergence algorithm (all four steps), the per-event parameter table
(with the `session-closed` union-fingerprint nuance and the hydration `--`
form), hook body shapes, the user-hook coexistence guarantee, the
logging/ordering/failure semantics, Migration-Helper Consolidation (including
the documented substring-vs-exact behavioural change and the deliberate
non-consolidation of `migrate-rename`), the Teardown Rewrite, all five Testing
Requirements, and seven of the eight Acceptance Criteria. One acceptance
criterion (AC 8, "Cascade eliminated") is present only by Spec-Reference
citation in Task 1-6 and is not enumerated as a verifiable phase-acceptance line
— recorded as Finding 1.

**Direction 2 (Plan → Spec, fidelity):** No hallucinated content found. Every
task's Problem / Solution / acceptance criteria / tests / edge cases trace back
to a specific spec section. Implementation pointers in the tasks (file paths,
line numbers, fixture names such as `countSignalHydrateEntries`,
`verifyHydrationHookEntries`, the `ShowGlobalHooksOrWarn` re-export) were
verified against the live codebase and are accurate — they serve ambiguity
resolution, not invention.

## Findings

### 1. AC 8 ("Cascade eliminated") is cited but not enumerated as a verifiable acceptance criterion

**Type**: Incomplete coverage
**Spec Reference**: Acceptance Criteria — item 8 ("Cascade eliminated. After the fix, a single tmux event that triggers a managed hook (e.g. a session-switch firing `pane-focus-out`) spawns exactly one `portal state notify`, not N.")
**Plan Reference**: Phase 1 "Acceptance" list (planning.md); Task 1-6 (`state-notify-cascade-on-binary-upgrade-1-6`, tick-9a1086)
**Change Type**: add-to-task

**Details**:
The specification lists eight Acceptance Criteria. The plan's Phase 1 acceptance
list and Task 1-6 together cover ACs 1–7 as explicit verifiable bullets, but AC
8 ("a single managed event spawns exactly one `portal state notify`, not N")
appears only as a citation in Task 1-6's `SPEC REFERENCE` line ("Acceptance
Criteria (1, 8)"). No phase-acceptance bullet and no task acceptance criterion
states the cascade-elimination outcome itself.

AC 8 is the behavioural downstream of AC 1 (no growth → array converges to one →
only one process fires per event), and the spec's Testing Requirements
deliberately provide no separate process-count test for it (TR 1, the no-growth
guard, is "the direct regression guard for the bug"). Task 1-6's no-growth test
therefore proves the structural precondition. The gap is one of enumeration, not
of underlying coverage: a reader auditing the plan against the eight acceptance
criteria finds seven enumerated and the eighth present only by reference. The
fix records AC 8 explicitly and ties it to the no-growth guard so the acceptance
set is complete and 1:1 with the spec.

This is recorded against Task 1-6 (the no-growth/blind-spot guard task), whose
no-growth assertion is the structural proof that the per-event array stays at
exactly one Portal entry — from which "exactly one process fires per event"
follows.

**Current** (Task 1-6 acceptance criteria, last bullet):
```
- No t.Parallel(); tests pass under go test ./internal/tmux/... with tmux available.
```

**Proposed** (Task 1-6 acceptance criteria — replace the last bullet with the two bullets below):
```
- No t.Parallel(); tests pass under go test ./internal/tmux/... with tmux available.
- Cascade eliminated (Acceptance Criterion 8): because the no-growth test proves every managed event converges to exactly one Portal entry, a single tmux event that fires a managed hook (e.g. a session-switch firing pane-focus-out) now dispatches exactly one portal state notify rather than N. The no-growth assertion (per-event Portal entry count == 1 across N>=2 bootstraps) is the structural guard for this outcome — no separate process-count test is required, consistent with the spec's Testing Requirements which name the no-growth test as the direct regression guard for the cascade.
```

**Resolution**: Fixed
**Notes**: Applied verbatim to Task 1-6 (tick-9a1086) — AC 8 enumerated as an explicit acceptance bullet tied to the no-growth structural guard.

---
