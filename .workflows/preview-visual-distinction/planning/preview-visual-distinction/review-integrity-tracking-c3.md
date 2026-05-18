---
status: complete
created: 2026-05-18
cycle: 3
phase: Plan Integrity Review
topic: Preview Visual Distinction
---

# Review Tracking: Preview Visual Distinction - Integrity

Cycle 3 focuses on verifying that the cycle-2 width-convention fix in task 1-4 (switching from spec-flavoured outer-width threshold rows to function-argument-convention threshold rows with output = arg + 2) is internally consistent across the live tick database and that no sibling task picked up collateral inconsistency.

Verification performed:
- **Task 1-4 (`tick-c90448`)** — live description now uses arg convention throughout: `Do` test thresholds (arg 200/60/40/25/13/2/1/0/-1), Acceptance Criteria ("Tier-4 args 0, 1, 2 each produce a valid output ('╭╮', '╭─╮', '╭──╮')"), Test names ("arg 0 tier 4 returns two-cell top edge", etc.), Edge Cases ("Tier-4 args 0, 1, 2 — output must be valid and width-correct"), `composeChromeLineParts` byte-for-byte property test enumerated args (200, 60, 40, 25, 13, plus 8/7-cell boundary plus degenerate args 0/1/2). All internally consistent with the function contract `outer = arg + 2`.
- **Task 1-5 (`tick-f842e2`)** — chrome-row invariant test enumerates widths {200, 80, 60, 40, 25, 15, 10, 4, 3, 2, 0} passed directly as `composeChromeLine(w, ...)` arguments. Under the function-arg convention these are inner widths; the test only asserts `strings.Count(got, '\n') == 0` so robustness is convention-agnostic. No drift.
- **Task 1-7 (`tick-674c91`)** — Edge Cases reference `msg.Width/Height` of 0 or 1, which are outer terminal widths from `tea.WindowSizeMsg` (always outer regardless of convention). The `max(0, msg.Width-previewFrameOverhead)` arithmetic correctly clamps to 0. No drift.
- **Task 1-8 (`tick-5f158b`)** — `View()` calls `composeChromeLineParts(m.width-previewFrameOverhead, …)`, correctly passing inner width as the function arg. Degenerate-width acceptance test uses `width=2` (outer, via `tea.WindowSizeMsg`), translating to `composeChromeLineParts(0, …)` — the canonical tier-4 arg-0 case from task 1-4. Consistent.
- **Task 1-9 (`tick-86f581`)** — e2e test rows use `tea.WindowSizeMsg{Width: w}` with w ∈ {200, 60, 40, 25, 15}. These are outer terminal widths; `View()` does the `-previewFrameOverhead` conversion before calling the pure function. Width 15 outer → inner arg 13 → outer 15 cells = `╭` + 13 × `─` + `╮`, matching task 1-4's "arg 13 tier 4" row. Consistent.
- **Phase 1 acceptance bullets** — the composeChromeLine contract bullet was updated in cycle 2 to `width >= 0` with explicit args 0 and 1 narration; the e2e bullet enumerates outer widths 200/60/40/25/15 matching task 1-9. Consistent.

One residual inconsistency found: `planning.md`'s task table Edge Cases column for task 1-4 (line 41) still carries the pre-cycle-2 outer-width values "tier-4 degenerate widths 2/3/4" that contradict the live task description's "Tier-4 args 0, 1, 2". This is the single collateral artefact the cycle-2 fix did not propagate to.

## Findings

### 1. `planning.md` task-table Edge Cases column for task 1-4 still uses pre-cycle-2 outer-width values

**Severity**: Minor
**Plan Reference**: `planning.md` line 41 (Phase 1 task table, task 1-4 Edge Cases column)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
Cycle 2's width-convention fix updated task 1-4's live tick description (Do, Acceptance Criteria, Tests, Edge Cases) from "tier-4 degenerate widths 2/3/4" to "tier-4 args 0/1/2" — codifying that the function-argument convention is used throughout the task. The `planning.md` Phase 1 task-table Edge Cases column was not updated in lockstep and still reads "tier-4 degenerate widths 2/3/4".

Under the function-arg convention now codified in the live task: arg 2 produces a 4-cell tier-4 frame `╭──╮`, not a 2-cell `╭╮`. The genuinely degenerate args (where the outer frame leaves zero room for any chrome content) are 0, 1, and 2 — corresponding to outer widths 2, 3, 4.

The mismatch is small but the `planning.md` task table is the orchestrator-facing index that summarises each task's edge cases. A reader navigating from the plan summary down to task 1-4's live description would see two different sets of numbers ("2/3/4" in the table vs "0/1/2" in the task body) and need to reconcile them. Aligning the table with the live task body removes the discrepancy and matches the cycle-2 finding-1 resolution.

The same row also still uses the phrase "width + 2 exact output width invariant" — under the new convention this would more precisely read "arg + 2 exact output width invariant" (mirroring the Test name "output width invariant equals arg plus two for all args" in the live tick). Folding both updates into one edit keeps the table consistent with the live task vocabulary.

**Current**:
```
| preview-visual-distinction-1-4 | Implement composeChromeLine cascade tiers 1-4 | tier-2 entry at 8-cell minimum boundary, tier-4 degenerate widths 2/3/4, width < 0 returns empty string, width + 2 exact output width invariant |
```

**Proposed**:
```
| preview-visual-distinction-1-4 | Implement composeChromeLine cascade tiers 1-4 | tier-2 entry at 8-cell minimum boundary, tier-4 degenerate args 0/1/2 (outer widths 2/3/4), arg < 0 returns empty string, arg + 2 exact output width invariant |
```

**Resolution**: Fixed
**Notes**: This is the only collateral artefact found from the cycle-2 width-convention switch. All other downstream tasks (1-5, 1-7, 1-8, 1-9) and the phase-level acceptance bullets are internally consistent.

---
