---
status: complete
created: 2026-05-10
cycle: 3
phase: Traceability Review
topic: Killed Sessions Resurrect on Restart
---

# Review Tracking: Killed Sessions Resurrect on Restart - Traceability

## Summary

Cycle 3 traceability review verifies convergence after cycle 2 fixes:

- **Removed task 2-7** (planning-side supersession artifact): Phase 2 acceptance now correctly states "Spec supersession is recorded in the killed-sessions spec (lines 156–163) ... No in-place edit of the original spec; no separate planning-side artifact required." — confirmed in `planning.md` lines 73-74 and `phase-2-tasks.md` line 280.
- **Pre-flight notes honest framing** (deferred to implementer): `planning.md` lines 5-17 explicitly states "The planning agent has **deferred this check to the implementer** because the planning environment lacks a real tmux + Portal cold-start fixture" and carries both branches as conditional plan scope. Confirmed.
- **Phase 2 acceptance AC6 reworded as behavioural prerequisite**: `planning.md` line 73 reads "the behavioural prerequisites for AC6 are met ... AC6's observational verification gate is owned by task 3-4 (Manual Verification Protocol step 2); this phase does not close AC6 on its own." — confirmed.
- **Integrity sync (table rows for 2-2 / 2-5)**: Phase 2 table rows match phase detail content; both reference the `runHydrate`-owned settle-sleep. Confirmed.
- **Phase 2 supersession acceptance line clarified**: Task 2-1 supersedes built-in-session-resurrection line 838; task 2-3 supersedes line 873. Both attributions match the killed-sessions spec § "Spec Supersession" (lines 156–163). Confirmed.

## Direction 1 — Specification → Plan (completeness)

Every spec element has plan coverage:

| Spec element | Plan coverage |
|---|---|
| Fix 1 — Bootstrap Eager-Signaling Step (Behaviour, Placement/Ordering Invariant, Pane Enumeration, Write Primitive, Race-Free Ordering, Failure Posture, Hook-Driven Signaling Relationship, Step Numbering Update, Adapter Wiring) | Phase 1 tasks 1-1 through 1-8 |
| Fix 2 — Timeout-Path Corrections (Specific Changes 1-4, Spec Supersession, Hook-Firing Safety, Logging) | Phase 2 tasks 2-1 through 2-6 |
| Fix 3 — Wrapper Drop in `buildHydrateCommand` (Behaviour, Why Removable, Inner Wrapper Untouched, Side Effects, Argument Quoting, Defect-D Closure) | Phase 3 tasks 3-1 through 3-3 |
| AC1 (markers cleared within 2s) | Task 1-6 |
| AC2 (hooks fire on non-attached session) | Task 2-6 |
| AC3 (killed sessions stay killed) | Pre-Flight Notes branch logic; existing companion daemon-merge fix coverage referenced |
| AC4 (scrollback save resumes) | Task 1-8 |
| AC5 (`exit` closes pane on first invocation) | Task 3-3 |
| AC6 (WARN volume drops to zero) | Phase 2 acceptance prerequisite + task 3-4 (observational gate) |
| AC7 (existing happy-path invariants preserved) | Phase 1 final acceptance line + Phase 3 acceptance line ("All existing happy-path resurrection integration tests remain green") |
| AC8 (daemon suppression intact) | Phase 1 acceptance + task 1-4 placement before Clear `@portal-restoring` |
| DoD items 1-5 | `planning.md` Definition of Done section maps each item to a task or covered scope |
| Empirical Reconfirmation Before Implementation Starts | Pre-Flight Notes section with branch-conditional scope |
| Manual Verification Protocol (6-step + 2 defect-D checks) | Task 3-4 |
| Manual Workaround | Informational in spec; no plan task required |
| Test Plan unit/integration/regression coverage | Distributed across tasks 1-3, 1-6, 1-8, 2-1, 2-3, 2-4, 2-5, 2-6, 3-1, 3-3 |

Depth check: each task contains enough detail (Problem, Solution, Outcome, Do, AC, Tests, Edge Cases, Context, Spec Reference) for an implementer to proceed without re-reading the spec for routine items. Spec text quoted into Context blocks where load-bearing.

## Direction 2 — Plan → Specification (fidelity)

Every plan element traces back to the spec:

- **Tasks 1-1 to 1-7**: every Do/AC/Test/Edge Case quotes from or paraphrases spec § "Fix 1: Bootstrap Eager-Signaling Step" subsections (Write Primitive, Pane Enumeration and FIFO Resolution, Failure Posture, Adapter Wiring, Bootstrap Step Numbering Update). No invented requirements.
- **Task 1-1 introduces `state.OpenFIFOForSignal`**: this is a thin shared production-opener export needed so both cmd and bootstrapadapter can share the same FIFO opener. Traces to spec § "Sharing mechanism" intent of single source of truth, even though the spec only names `writeFIFOSignal` and `signalHydrateRetryDelays` explicitly. Justified as engineering necessity for the explicit "shared internal package" goal — not a hallucinated requirement.
- **Task 1-8 introduces `state.RunCaptureOnce` test seam**: required to drive a deterministic capture tick for AC4 verification. The spec mandates AC4 verification but does not explicitly enumerate this seam. The task scopes the addition narrowly ("thin exported test seam ... runs exactly one iteration of the existing daemon capture-step body. No other new exports") and frames it as conditional on whether a callable primitive already exists. Justified for AC4 traceability and consistent with the spec's test-plan posture.
- **Tasks 2-1 to 2-5**: every change traces to Fix 2 specific changes 1-4 and the spec's tests-first TDD framing. The "set-option -su argv observed exactly once per timeout" assertion is a defensive sharpening of spec § "Fix 2 → Specific Changes → 1" — reasonable interpretation, not invention.
- **Task 2-2 (comment-only)**: traces to spec § "Fix 2 → Specific Changes → 3" (replace line-262 comment).
- **Task 2-6**: traces to spec § "Test Plan → Integration → End-to-end hook firing on cold-start" and AC2.
- **Task 3-1**: traces to spec § "Fix 3" → "Behaviour" and "Argument Quoting".
- **Task 3-2 (doc comment refresh)**: not enumerated as its own line item in the spec, but the spec § "Argument Quoting" and § "Inner Hook-Firing Wrapper Is Untouched" mandate the doc-comment boundary be reflected accurately. Justified as a sub-task of Fix 3 deliverable.
- **Task 3-3**: traces to spec § "Side Effects" and AC5; sub-test 3 (`pgrep` no rows) traces to "Manual Verification Protocol additional checks".
- **Task 3-4**: traces directly to spec § "Manual Verification Protocol", AC6, and DoD item 3.
- **Pre-Flight Notes**: traces directly to spec § "Empirical Reconfirmation Before Implementation Starts", with both branches preserved per spec contract.

## Findings

None. Plan converges on a faithful translation of the specification.

- All cycle-1 and cycle-2 fixes remain in effect.
- No new gaps surfaced.
- No hallucinated content detected.
- All borderline engineering inferences (`OpenFIFOForSignal` export, `RunCaptureOnce` test seam) are scoped narrowly and justified by spec-stated goals (sharing mechanism, AC4 verification).

## Resolution

The traceability surface is clean for cycle 3. No findings to apply.
