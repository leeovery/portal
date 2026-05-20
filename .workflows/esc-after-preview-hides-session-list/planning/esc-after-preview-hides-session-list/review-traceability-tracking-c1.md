---
status: complete
created: 2026-05-20
cycle: 1
phase: Traceability Review
topic: Esc After Preview Hides Session List
---

# Review Tracking: Esc After Preview Hides Session List - Traceability

## Findings

No findings. The plan is a faithful, complete translation of the specification.

### Direction 1 — Specification → Plan (completeness)

Every spec element has plan coverage:

- **Root Cause** (`applySessions` discards cmd; two-phase `SetItems` contract; `KeyMap.ClearFilter` second-Esc path) → task 1-3 Problem section preserves the mechanism in full.
- **Fix Approach → Primary change** (signature change + propagation at `SessionsMsg` and `previewSessionsRefreshedMsg` handlers; `previewAttachBailMsg` covered transitively) → task 1-3 Do steps and ACs.
- **Fix Approach → Secondary sweep** (`Model.WithInsideTmux` panic-guard; `ProjectsLoadedMsg` propagation; no `tea.Batch`; no test for the projects site) → task 1-5.
- **Scope** sibling mutator audit (`SetItem`/`InsertItem`/`RemoveItem` against `m.sessionList`/`m.projectList`; record outcome in PR description) → task 1-6.
- **Test Coverage → Lock in at the wrong-axis miss site** (`VisibleItems`/`visibleSessionNames` slice-equality + cursor-index assertions) → task 1-2.
- **Test Coverage → Cover the latent variant** (kill-refresh test via real keystrokes; `SessionKiller` + `SessionLister` seams; slice-equality assertion) → task 1-4.
- **Test Coverage → Test harness must drain the propagated refilter cmd** (extend `pressSpaceThenEscWithRefresh`; reusable `drainRefilterCmd` helper) → task 1-1.
- **Test scope discipline** (no separate tests for rename/bail/projects variants) → encoded as explicit "Do not" steps in tasks 1-4 and 1-5.
- **Acceptance Criteria #1–#7** → phase-level Acceptance list mirrors each, plus the harness-extension detail from Test Coverage.
- **Out of scope** (cursor reanchoring on externally-killed-during-preview filtered branch) → correctly absent from the plan.
- **Alternatives Considered** (rejected) → correctly absent from the plan (informational only).
- **Risk** → correctly absent (informational only; no testable behaviour).
- **"Existing test left unchanged"** (`TestPreviewEscPreservesCommittedFilter`) → no-op directive; correctly produces no task.

### Direction 2 — Plan → Specification (fidelity)

Every task traces verbatim to specific spec sections:

- Task 1-1 → spec "Test Coverage → Test harness must drain the propagated refilter cmd" (quoted in task Context).
- Task 1-2 → spec "Test Coverage → Lock in the fix at the wrong-axis miss site" + AC #1, #6 (quoted in task Context).
- Task 1-3 → spec "Root Cause" + "Fix Approach → Primary change" + Implementation Notes + AC #1–#4 (quoted in task Context).
- Task 1-4 → spec "Test Coverage → Cover the latent variant" + "Test scope — one representative latent-variant test is sufficient" + AC #2, #6 (quoted in task Context).
- Task 1-5 → spec "Fix Approach → Secondary sweep" + Implementation Notes + AC #5 (quoted in task Context).
- Task 1-6 → spec "Scope" sibling-mutator audit clause + AC #5 (quoted in task Context).

No hallucinated content. No invented technical approaches. No acceptance criteria testing things the spec does not require. No edge cases not grounded in the spec.

**Resolution**: Clean — no changes required.
