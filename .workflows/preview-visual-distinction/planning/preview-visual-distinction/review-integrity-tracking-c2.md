---
status: in-progress
created: 2026-05-18
cycle: 2
phase: Plan Integrity Review
topic: Preview Visual Distinction
---

# Review Tracking: Preview Visual Distinction - Integrity

Cycle 1 integrity fixes (7 findings) were re-verified against the live `tick show` output for each affected task — all proposed cycle-1 replacements are present in the tick database. The findings below are net-new on cycle 2.

The disputed cycle-1-c2 width-convention finding was re-examined carefully against spec § Width cascade > Unit of measure and § Top edge composition > Degenerate widths. The spec uses `width` with two distinct meanings: (a) the function parameter (inner width, `terminalWidth − 2`) in § Unit of measure; (b) the **outer terminal width** in § Top edge composition > Column layout, explicitly flagged at line 223 ("The column layout below uses `width` for the outer terminal width"). The § Degenerate widths section immediately follows the Column layout paragraph and inherits the outer-width convention — so "width 2: `╭╮`" in the spec is internally consistent as an outer-terminal-width statement (i.e. it describes the function output, not the function input).

Task 1-4's test threshold rows are **inputs to the pure function under test** — by definition the leftmost column is the function argument. The function contract (codified in the same task as `outer := width + 2` and asserted via "For every `width >= 0` the returned string has `lipgloss.Width == width + 2`") means the row `width 2 → ╭╮` is internally inconsistent: the function's output for argument 2 must be 4 cells (`╭──╮`), not 2 cells. The rows that *look* charitable when re-interpreted under outer-width convention break the function-parameter contract the same task asserts elsewhere. An implementer following the rows verbatim writes failing tests; an implementer following the contract writes tests that contradict the rows. The finding stands as Critical.

## Findings

### 1. Task 1-4 boundary test rows confuse function-parameter convention with outer-width convention — expected outputs cannot satisfy both

**Severity**: Critical
**Plan Reference**: preview-visual-distinction-1-4 (`tick-c90448`) — Do (test thresholds), Tests, Acceptance Criteria, Edge Cases
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-4 documents its function contract three ways, all stating `output_cells = arg + 2`:
1. Algorithm code: `outer := width + 2`.
2. Acceptance Criteria: "For every `width >= 0` the returned string has `lipgloss.Width == width + 2`".
3. Test row: "output width invariant equals width plus two for all widths".

But the task's threshold rows describe expected outputs that contradict that contract:

| Row in task 1-4                                | Function arg | Output cells (row says) | Output cells (contract requires) |
|-----------------------------------------------|--------------|-------------------------|----------------------------------|
| width 15 → "╭" + 13 × "─" + "╮"               | 15           | 15                      | 17                               |
| width 4 → "╭──╮"                              | 4            | 4                       | 6                                |
| width 3 → "╭─╮"                               | 3            | 3                       | 5                                |
| width 2 → "╭╮"                                | 2            | 2                       | 4                                |
| width 0 → outer == 2, returns "╭╮"            | 0            | 2                       | 2 ✓ (coincidence)                |
| width -1 → ""                                 | -1           | 0                       | 0 ✓                              |

The rows phrase their first column under the spec's *outer-width* convention (where "width 15" means the outer terminal width, producing a 15-cell top edge `╭{─×13}╮`), but they are simultaneously being used as **inputs to the function under test**. The function takes its argument as inner-width. An implementer running `composeChromeLine(15, ...)` and expecting a 15-cell output is wrong — they would get a 17-cell output and the "width plus two" invariant test would also fail at that same input. An implementer running `composeChromeLine(13, ...)` (the correct argument for a 15-cell outer top edge) is fine but then the row's leftmost column is misleading.

The Acceptance Criterion "Tier-4 degenerate widths 2, 3, 4 each produce a valid output ('╭╮', '╭─╮', '╭──╮')" codifies the row-side error and the Test names "width 2 returns two-cell top edge" / "width 3 returns three-cell top edge" / "width 4 returns four-cell top edge" name it explicitly.

The remedy is to disambiguate by adopting one convention consistently. Since the leftmost column drives a test call (function input), the natural convention is function-parameter and the row outputs must be `arg + 2` cells. This also aligns the task 1-4 rows with task 1-9's e2e rows (which use `tea.WindowSizeMsg{Width: w}` — outer width — at the integration level, with `View()` doing the `-previewFrameOverhead` conversion before calling the function).

Note on the user-hint counter-reading: if the rows were re-interpreted as outer-width inputs (with the test setup expected to convert via `arg - 2` before calling the function), this would relocate the bug to the description rather than the math — but the same task's "output width invariant equals width plus two for all widths" test would still fail in this reading at any non-zero argument since it presumably feeds `width` directly. The two conventions cannot coexist in a single task without explicit per-row labelling. The cleanest fix is to keep the function-parameter convention everywhere in task 1-4 and let the row outputs grow accordingly.

**Current**:

Do (test thresholds block):
```
    - width 200 → tier 1, full name present.
    - width 60 → tier 1, truncated name with '…'.
    - boundary: smallest width where nameBudget == minWindowNameCells (8) — assert tier 1 still active and '…' present.
    - boundary: width where nameBudget == 7 — assert tier 2 selected.
    - width 40 → tier 2.
    - width 25 → tier 3 (verify by strings.Contains(got, compactKeymap) && !strings.Contains(got, 'next pane')).
    - width 15 → tier 4 (top edge is '╭' + 13 × '─' + '╮').
    - width 4 → tier 4, output is '╭──╮'.
    - width 3 → tier 4, output is '╭─╮'.
    - width 2 → tier 4, output is '╭╮'.
    - width 0 → outer == 2, returns '╭╮'.
    - width -1 → returns ''.
```

Acceptance Criteria entry:
```
  - Tier-4 degenerate widths 2, 3, 4 each produce a valid output ('╭╮', '╭─╮', '╭──╮').
```

Tests entries:
```
  - 'width 4 returns four-cell top edge'
  - 'width 3 returns three-cell top edge'
  - 'width 2 returns two-cell top edge'
```

Edge Cases entry:
```
  - Tier-4 degenerate widths 2, 3, 4 — output must be valid and width-correct without panic.
```

**Proposed**:

Do (test thresholds block) — adopt the function-parameter convention consistently (leftmost column is the function argument; output cells = arg + 2):
```
    - width 200 → tier 1, full name present (output 202 cells).
    - width 60 → tier 1, truncated name with '…' (output 62 cells).
    - boundary: smallest width where nameBudget == minWindowNameCells (8) — assert tier 1 still active and '…' present.
    - boundary: width where nameBudget == 7 — assert tier 2 selected.
    - width 40 → tier 2 (output 42 cells).
    - width 25 → tier 3 (output 27 cells; verify by strings.Contains(got, compactKeymap) && !strings.Contains(got, 'next pane')).
    - width 13 → tier 4, output is '╭' + 13 × '─' + '╮' (15 cells, outer = arg + 2 = 15).
    - width 2 → tier 4, output is '╭────╮' (6 cells, outer = arg + 2 = 6).
    - width 1 → tier 4, output is '╭───╮' (5 cells, outer = arg + 2 = 5).
    - width 0 → tier 4, output is '╭──╮' (4 cells, outer = arg + 2 = 4) — wait, this contradicts the existing 'width 0 → outer == 2, returns ╭╮' row. Resolve: when arg = 0, outer = 2, which leaves 0 cells in the middle range [2, width − 3]. The assemble helper's tier-4 branch is `tl + strings.Repeat(h, outer-2) + tr` = `╭` + 0 × `─` + `╮` = `╭╮`. So arg 0 → `╭╮` (2 cells). Arg 1 → outer = 3 → `╭─╮` (3 cells). Arg 2 → outer = 4 → `╭──╮` (4 cells). The genuinely degenerate args are 0 and 1.
    - width 0 → tier 4, output is '╭╮' (2 cells, outer = 2; minimal frame, no filler).
    - width -1 → returns ''.
```

(Implementer note for the Do block: the 'width 0' row above corrects the previous calculation — at arg 0, outer is 2, which produces the two-corner frame. The threshold table should list args 0, 1, 2, 13 for the tier-4 boundary cases plus the existing wider rows.)

Acceptance Criteria entry (replacement):
```
  - Tier-4 args 0, 1, 2 each produce a valid output ('╭╮', '╭─╮', '╭──╮') — outer width = arg + 2 = 2, 3, 4 cells respectively. These are the genuinely degenerate cases where the outer frame leaves no room for any chrome content.
```

Tests entries (replacement):
```
  - 'arg 2 tier 4 returns six-cell top edge (corners and four filler)'
  - 'arg 1 tier 4 returns three-cell top edge (corners and one filler)'
  - 'arg 0 tier 4 returns two-cell top edge (corners only)'
  - 'arg 13 tier 4 returns fifteen-cell top edge (corners and thirteen filler)'
```

Edge Cases entry (replacement):
```
  - Tier-4 args 0, 1, 2 — output must be valid and width-correct (outer = arg + 2 = 2, 3, 4 cells respectively) without panic. These are the genuinely degenerate function-argument cases where outer ≤ 4 leaves no room for any chrome content.
  - The previous rows that read 'width 2 → ╭╮' / 'width 3 → ╭─╮' / 'width 4 → ╭──╮' were using the spec's outer-width convention (from § Top edge composition > Degenerate widths, where 'width' means outer terminal width) but the leftmost column of the threshold table is the function argument (inner width). Under the function-argument convention, those outer-width fixtures correspond to args 0, 1, 2.
```

**Resolution**: Pending
**Notes**: Verified against `tick show tick-c90448` (live description) and spec lines 132-134 (Unit of measure: function arg = inner width, returned cells = width + 2) and 240-246 (Degenerate widths: "width 2: ╭╮" uses outer-terminal-width convention per the immediately-preceding Column layout paragraph at line 223).

---

### 2. Phase acceptance lower bound on composeChromeLine contract excludes valid args 0 and 1

**Severity**: Minor
**Plan Reference**: Phase 1 acceptance, second checkbox bullet (`planning.md` line 16)
**Category**: Acceptance Criteria Quality

**Change Type**: update-task

**Details**:
The phase-level acceptance bullet specifies the contract `width + 2` "for every `width >= 2`" and "returns the empty string for `width < 0`". This leaves args 0 and 1 unspecified at the phase level. The task-1-4 algorithm (and the live task 1-4 description's Acceptance Criteria line "For every width >= 0 the returned string has lipgloss.Width == width + 2") handles both: arg 0 → outer=2 → `╭╮`; arg 1 → outer=3 → `╭─╮`. Both are valid tier-4 outputs.

Tightening the phase bullet to `width >= 0` removes the gap and matches task 1-4's actual contract, preventing a future reader of phase acceptance from concluding args 0 and 1 have undefined behaviour.

**Current**:
```
- [ ] `composeChromeLine` exists as a pure function in `internal/tui/pagepreview.go`, returns a single-row top-edge string (no embedded newlines) of display-cell width `width + 2` for every `width >= 2`, and returns the empty string for `width < 0`.
```

**Proposed**:
```
- [ ] `composeChromeLine` exists as a pure function in `internal/tui/pagepreview.go`, returns a single-row top-edge string (no embedded newlines) of display-cell width `width + 2` for every `width >= 0` (args 0 and 1 produce the minimum tier-4 frames `╭╮` and `╭─╮` respectively), and returns the empty string for `width < 0`. The `width` parameter is the inner frame width (`terminalWidth − 2`); the returned string has display-cell width `width + 2` (the outer terminal width).
```

**Resolution**: Pending
**Notes**:

---

### 3. phase-1-tasks.md mirror file is stale relative to the live tick database

**Severity**: Minor
**Plan Reference**: `.workflows/preview-visual-distinction/planning/preview-visual-distinction/phase-1-tasks.md` (entire file)
**Category**: Task Self-Containment (informational — not strictly a plan-integrity criterion)
**Change Type**: (no plan change — flag for orchestrator)

**Details**:
The `phase-1-tasks.md` file contains the cycle-0 task content and was not updated when cycle-1 integrity findings were applied to the tick database. Per `reading.md`, implementers locate tasks via `tick show <id>` — so the live source is authoritative and this mirror file is informational only. However, the file's existence and stale state could mislead a casual reader. Notably the mirror still carries the pragmatic-interpretation NOTE block from before cycle 1 fix #3 was applied (the live tick description has been correctly cleaned).

No action required for plan integrity; flagged so the orchestrator can decide whether to refresh or remove the mirror at conclusion.

**Resolution**: Pending (informational — no plan content change proposed)
**Notes**: Out of scope for cycle-2 integrity remediation per the "Task scope only — check the plan as built" rule. Mentioned for completeness. Same finding as cycle 1-c2 finding 4 (carried forward unchanged).

---
