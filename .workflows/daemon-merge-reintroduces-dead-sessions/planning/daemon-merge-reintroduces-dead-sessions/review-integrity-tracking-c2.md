---
status: complete
created: 2026-05-09
cycle: 2
phase: Plan Integrity Review
topic: daemon-merge-reintroduces-dead-sessions
---

# Review Tracking: daemon-merge-reintroduces-dead-sessions - Integrity

## Findings

No findings. The plan meets structural quality and implementation-readiness standards.

### Cycle 1 follow-up confirmation

The cycle 1 finding (task 2-1 Tests meta-commentary at `phase-2-tasks.md` line 41) has been applied verbatim. The current Tests section reads as a clean, declarative test list with no authoring stream-of-consciousness.

### Scope of review

Re-evaluated all eight integrity criteria across phases 1 and 2, all 12 tasks (1-1..1-5, 2-1..2-7) including full task detail in `phase-1-tasks.md` and `phase-2-tasks.md`:

1. Task Template Compliance — every task has Problem / Solution / Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference. Acceptance criteria are pass/fail and concrete; Tests sections name primary behaviour plus edge cases.
2. Vertical Slicing — each task is one TDD cycle delivering a verifiable behaviour (a filter level, a regression test, a seam, a guard, an adapter, an end-to-end). No horizontal "all helpers then all wiring" decomposition.
3. Phase Structure — two phases with explicit "Why this order" rationale. Phase 1 establishes the live-truth invariant in the merge layer (resolves the user-visible symptom). Phase 2 builds on that invariant to add cleanup without serialisation against the daemon. Phase boundaries are load-bearing, not arbitrary.
4. Dependencies and Ordering — tasks within each phase are sequential by intent and execute correctly under the natural-id ordering convention. The helper introduced in 1-1 is extended in 1-2 / 1-3; the seam introduced in 2-1 is extended in 2-2..2-4 then wired in 2-5..2-7. No cross-phase convergence requiring explicit dependency edges.
5. Task Self-Containment — each task pulls relevant spec quotations into its Context block, names exact file paths and line ranges, and supplies fixture data inline. An implementer could execute any single task without re-reading the specification.
6. Scope and Granularity — no task exceeds five concrete Do steps in spirit (some are longer because they enumerate orchestrator-doc / Debug-label / test-update edits that must be co-authored with the code change, which is appropriate). No task is mechanical-only.
7. Acceptance Criteria Quality — criteria are pass/fail and reference specific assertion shapes (e.g. `findPane(idx, "old", 1, 2) == nil`, `errors.As(err, &fatal) is false`, `UnsetServerOption` call counts).
8. External Dependencies — N/A for bugfix scope.

### Items considered but not raised

- Task 1-2's "or extend it now" hedge about the helper shape from 1-1 — task 1-1's Do block already mandates the full `map[string]map[int]map[int]struct{}` shape, so 1-2 just consumes the deeper layer. Implementer-clear.
- Task 2-6's adapter-stub-shape guidance ("stub `*tmux.Client` is impractical... introduce a small seam stub OR exercise the adapter by injecting a stub-able interface field") — genuine implementer guidance referencing the existing `FIFOSweeper` `Client state.ServerOptionLister` pattern. Acceptable implementation discretion within a bounded shape.
- Task 2-7's daemon-tick driving options ("Construct a daemonDeps (or invoke captureAndCommit directly via an exported test seam, or via daemon-loop-test style)") — three valid test-author paths; selecting one is a test-implementation call, not a design decision the planner left unresolved.
- Minor fixture name divergence between 2-1 acceptance criteria (`foo`/`bar`) and 2-1 Tests (`stale`/`live`) — both are valid example data; the Tests section is the authoritative narrative for assertions.

None of these rises to blocking implementation, forcing a design decision, or codifying ambiguity that would compromise the work.
