---
status: in-progress
created: 2026-06-18
cycle: 2
phase: Plan Integrity Review
topic: Spectrum TUI Design
---

# Review Tracking: Spectrum TUI Design - Integrity

## Summary

Cycle-2 follow-up after the cycle-1 integrity findings were applied. Read end-to-end again: planning.md (5-phase structure + per-phase goal/why/acceptance/task tables) and all five per-phase task files (phase-1..5-tasks.md — 9/9/9/7/8 = 42 tasks), plus the cycle-1 integrity tracking file to confirm each applied fix.

**Result: clean. No new findings.** The cycle-1 edits integrated without introducing duplication, contradiction, or broken self-containment. The plan remains structurally strong and implementation-ready across every integrity dimension.

### Cycle-1 fixes — verified well-integrated

- **Task 1-9 — §12.3 acceptance criterion (Ctrl+↑/↓ swallow-check + fallback)**: present as the final acceptance criterion (phase-1-tasks.md ~L414). It reads as a validation caveat to confirm the paging chords are delivered (not swallowed by terminal/tmux) and to flag a fallback page key for the descriptor consumers (2-1 / 3-3 / 4-7). No duplicated test/edge-case, no contradiction with 2-1's `Ctrl+↑/↓ page` binding (2-1 binds it; 1-9 validates delivery and provides the fallback escape hatch). Self-contained.
- **Tasks 3-4 / 3-6 / 3-9 — §15.6 light-mode-eyeball acceptance criterion**: present as the final acceptance criterion on each (3-4 ~L195, 3-6 ~L303, 3-9 ~L487). Wording is consistent across all three ("rendered in light mode against `#e1e2e7` … no further Paper mock required per §15.6 … the deferred light eyeball task 1-9 punted to this surface, not a frame compare"). No conflict with the preceding VISUAL VERIFICATION criterion on each task — that one is a dark Paper-frame compare; the new one is a light in-terminal eyeball with no Paper mock, a complementary not contradictory gate. Coherent with 1-9's own scope note that the *foundation* light wiring is confirmed at 1-9 while the *per-modal* light eyeball "lands with those surfaces in later phases."
- **Task 5-7 — orange/warning role + transient/auto-clear lifetime pinned**: the third Do bullet (~L313), the single-slot Edge Case (~L337), and the new acceptance criterion (~L324) all now consistently state the post-load warning notice is the orange/warning role variant and a transient band that auto-clears on the next actionable keypress (reusing `flashGen`/`isActionableKey`), following the 4-1 arbiter's transient hand-off (yields slot to a persistent band on clear). The three statements agree with each other and with the implementer escape-hatch ("if §11 single-slot semantics force a different lifetime, record the deviation"). Coherent with task 4-1's arbiter contract (a transient flash wins the slot then yields) and with 4-2's flash being the canonical transient consumer — 5-7 now matches that class exactly. No contradiction introduced.
- **Task 5-5 — first acceptance criterion split into two pinning the five §10.4 labels in order + matching test**: the split criteria (~L216–L217) and the new test (~L228) pin the step-list to exactly the five §10.4 friendly labels in order — `Started tmux server` · `Registered hooks` · `Restoring sessions (N/M)` · `Replaying scrollback` · `Running resume commands` — "sourced from the task-5-4 mapping (one row per label, no label invented or dropped at the render layer)." The label set/order matches §10.4 and task 5-4's mapping table verbatim. The criterion correctly defers authority to 5-4 (single source of truth) rather than re-asserting an independent label set, so no split-authority risk was introduced. The pre-existing frame-compare criterion remains as backstop.

### Full integrity re-pass — all dimensions hold

- **Task template compliance**: all 42 tasks carry Problem / Solution / Outcome / Do / Acceptance Criteria / Tests / Edge Cases / Context / Spec Reference. Cycle-1 edits added criteria/tests/Do/Edge-Case content within the existing template; none left a task malformed or a field empty.
- **Vertical slicing**: each task remains a single, independently-verifiable increment; no horizontal layering. The cycle-1 additions were criteria/lifetime pins, not scope expansions, so slicing is unchanged.
- **Phase structure**: foundation → consumers ordering intact (Phase 1 tokens/canvas/detect-gate; Phase 2 chrome + Sessions; Phase 3 Projects + modals; Phase 4 preview + edge states; Phase 5 cold-path flip). Phase boundaries are non-arbitrary and each phase has its own acceptance block.
- **Dependencies / ordering**: every cross-phase dependency consumes EARLIER work — 1-7 gate → 5-5; 2-1 descriptor → 2-4/3-3/3-4/4-5/4-7; 2-9 empty-state pattern → 4-5; 3-1 blank-screen → 3-5/3-6/3-7/3-9; 3-4 renderer → 4-7; 4-1 notice band → 4-2/4-3/4-4/5-7; 4-6 chrome → 4-7; 5-1 gate → 5-2..5-8; 5-3 callback → 5-4; 5-4 mapping → 5-5. Natural phase/ID order produces the correct sequence; no forward references, no circular dependencies, no convergence point missing an explicit edge. The cycle-1 5-7 transient-lifetime pin strengthened (did not break) the 4-1↔5-7 edge by making the band's persistence class explicit and therefore testable against the arbiter.
- **Self-containment**: each task still carries the spec context, codebase anchors, and ambiguity notes needed to execute it without reading sibling tasks. The cycle-1 5-7 fix specifically removed the prior open implementer decision (warning-notice lifetime), improving self-containment.
- **Acceptance-criteria quality**: criteria remain pass/fail and cover the actual requirement; the vhs-exemption + behavioural-acceptance pattern is stated on every non-visual/plumbing task. The cycle-1 5-5 split made the label-set/order a direct pass/fail check (previously only frame-compare-implied), and the cycle-1 5-7 criterion made the warning-band lifetime verifiable — both raised criteria quality.
- **Consistency**: the reskin-vs-behaviour-change-vs-new-work classification is applied uniformly (edit modal = the one deliberate behaviour change, banner-flagged on 3-8/3-9; kill/rename/delete/Projects/preview/edge-states = parity reskins; blank-screen layer + ? help + canvas/detection + cold-path flip = new). The light-eyeball criteria added to 3-4/3-6/3-9 use identical phrasing, reinforcing rather than diverging from the established §15.6 convention.
- **Proportionality**: nothing rises to a finding. No nitpicks manufactured.

## Findings

None. The plan meets structural quality and implementation-readiness standards, and the cycle-1 fixes are cleanly integrated with no new inconsistency.
