---
status: complete
created: 2026-05-19
cycle: 2
phase: Plan Integrity Review
topic: saver-kill-respawn-loop-leaks-daemons
---

# Review Tracking: Saver Kill-Respawn Loop Leaks Daemons - Integrity

## Findings

No findings.

## Review Notes

Cycle 1 surfaced one Minor finding — Task 1-1 (tick-c911bf) was missing the canonical `Tests` field. That finding was applied via `tick update` (Tests block added between Acceptance Criteria and Edge Cases, eight test names enumerated). Cycle 2 re-read all twelve tasks from scratch against the eight integrity criteria.

**Task Template Compliance**: All twelve tasks now carry Problem, Solution, Outcome, Do, Acceptance Criteria, Tests, Edge Cases, and Spec Reference. Task 1-5 explicitly documents `Tests: None — this is a comment-only edit. Verification is by code review against the new contract.` — appropriate for a documentation-only task and follows the spirit of the template (the field is present and the rationale for omission is recorded). Every other task lists concrete test names including edge cases (not just happy paths).

**Vertical Slicing**: Each task delivers complete, independently testable functionality. Phase 1 tasks 1-1 through 1-6 each pair implementation with verification; Phase 2 tasks 2-2/2-3/2-4 split the three ctx.Done() observation points into three TDD cycles, each with its own unit test, and 2-5/2-6 add real-tmux integration tests.

**Phase Structure**: Two phases with clear Foundation → Refinement progression. Phase 1 ships the user-visible fix (alive-check gating + defensive write + breadcrumb). Phase 2 ships the structural responsiveness fix (ctx threading + three observation points + integration regression guards). "Why this order" rationales are explicit and load-bearing.

**Dependencies and Ordering**: Tasks within each phase follow natural execution order by internal ID. Per the review criteria's intra-phase rule, missing explicit dependencies are not flagged when natural order produces the correct sequence. No cross-phase convergence points exist. No circular dependencies.

**Task Self-Containment**: Each task contains file paths (e.g. `internal/tmux/portal_saver.go:249`, `cmd/state_daemon.go:133`), function names, fake-injection guidance, and Spec References. An implementer could pick up any single task in isolation.

**Scope and Granularity**: Each task is one TDD cycle. The largest tasks (1-3, 2-1) involve signature changes touching ~3 call sites, well within the "5 concrete steps" threshold. The smallest task (1-5) is a comment edit, justified by being load-bearing for future readers (it documents the inverted invariant from Tasks 1-3/1-4).

**Acceptance Criteria Quality**: All criteria are pass/fail. Concrete byte strings are pinned (`'daemon.version write:'`, `'no such session'`, `'exit status 1'`). Numeric thresholds are anchored to fresh measurements (Task 2-5) or unchanged invariants (5s `killBarrierTimeout`). Call-ordering assertions specify the recorder mechanism (call-order recorders on injected fakes).

**External Dependencies**: N/A — this is a bugfix, not an epic.

Plan meets structural quality standards. Ready for implementation.
