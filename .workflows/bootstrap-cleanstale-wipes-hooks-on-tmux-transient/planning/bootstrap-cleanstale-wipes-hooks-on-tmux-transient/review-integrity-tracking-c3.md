---
status: complete
created: 2026-05-27
cycle: 3
phase: Plan Integrity Review
topic: Bootstrap CleanStale Wipes Hooks On Tmux Transient
---

# Review Tracking: Bootstrap CleanStale Wipes Hooks On Tmux Transient - Integrity

Cycle 3 — post-fix follow-up after cycle 2's single finding was applied.

## Cycle 2 Fix Verification

Cycle 2 finding (Task 3-2 Do step 6 used the wrong arity destructure `warnings, err := orch.Run(ctx)`) is fixed in place at `phase-3-tasks.md:106`:

```go
orch, _ := buildProductionOrchestrator()
ctx := context.Background()
_, warnings, err := orch.Run(ctx)
```

The explanatory note on line 108 correctly describes the three-value return `(serverStarted bool, warnings []Warning, err error)` and that the `serverStarted` bool is `_`-discarded for the hooks-preservation property under test. Matches the proposed text from cycle 2 verbatim.

## Fresh Integrity Scan

Performed a fresh end-to-end pass across `planning.md`, `phase-1-tasks.md`, `phase-2-tasks.md`, and `phase-3-tasks.md` against all eight review criteria:

1. **Task Template Compliance** — All seven tasks (1-1, 1-2, 2-1 through 2-5, 3-1 through 3-3) carry Problem, Solution, Outcome, Do, Acceptance Criteria, Tests, Edge Cases, Context, and Spec Reference. Task 2-1 has no new test (compile-time-only) but explicitly states this and is covered by Task 2-3's unit suite — acceptable per the wiring-only narrative.
2. **Vertical Slicing** — Every task is a single TDD cycle and verifiable independently within its phase. Phase 2's split (logger plumbing → hazard guard → adapter tests → portal-clean callsite → invert destructive test) follows seam-then-behaviour ordering.
3. **Phase Structure** — Phases follow Foundation (Phase 1: helper contract) → Core (Phase 2: hazard guard at both callsites) → Integration (Phase 3: end-to-end coverage). Each phase has clear acceptance criteria and a "Why this order" justification.
4. **Dependencies and Ordering** — Cross-phase dependencies (Phase 2 depends on Phase 1's `(nil, err)` contract; Phase 3 depends on Phase 2's logging contract for log-line assertions) are captured in the "Why this order" prose. Intra-phase tasks execute in natural ID order — no explicit edges required.
5. **Task Self-Containment** — Each task carries enough spec-pulled context to execute without re-reading the specification. Code samples in Tasks 1-1, 2-1, 2-2, 2-3, 2-4, 3-2 give the implementer the exact shape of the change.
6. **Scope and Granularity** — No task exceeds the "5 concrete Do steps" signal in a way that suggests splitting; no task is mechanical boilerplate.
7. **Acceptance Criteria Quality** — Criteria are pass/fail and cover the actual requirement (e.g., Task 2-3's "hazard-guard subtest explicitly asserts the completion Debug is NOT recorded" is the mutual-exclusivity property, not just code existence). Minor style note: Phase 2 task acceptance criteria use plain `-` bullets while Phase 1 and Phase 3 use `- [ ]` checkboxes; the inconsistency is cosmetic and would not block an implementer.
8. **External Dependencies** — N/A (bugfix, not epic).

## Findings

No findings. The plan meets structural quality standards.

---
