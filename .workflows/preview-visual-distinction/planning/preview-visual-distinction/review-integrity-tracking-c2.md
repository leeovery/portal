---
status: in-progress
created: 2026-05-18
cycle: 2
phase: Plan Integrity Review
topic: Preview Visual Distinction
---

# Review Tracking: Preview Visual Distinction - Integrity

Cycle 1 integrity fixes (7 findings, all applied) were re-verified via `tick show` for every task — all proposed replacements are now live in the task database. The findings below are net-new on cycle 2.

## Findings

### 1. Task 1-4 boundary tests use the wrong width convention — expected outputs are off by 2 cells

**Severity**: Critical
**Plan Reference**: preview-visual-distinction-1-4 (`tick-c90448`) — Do (test thresholds), Tests, Acceptance Criteria, Edge Cases
**Category**: Acceptance Criteria Quality / Task Self-Containment
**Change Type**: update-task

**Details**:
Task 1-4 documents its function contract as `output = arg + 2` (the function parameter is the *inner* width `terminalWidth − 2`; the returned string has display-cell width `width + 2`). The algorithm code `outer := width + 2` enforces this, and task 1-8 confirms it at the consumer (`composeChromeLine(m.width-previewFrameOverhead, ...)` with assertion that the rendered top row has `lipgloss.Width == 80` when `m.width == 80` — i.e. arg=78 → output=80). Task 1-9 also confirms it at the e2e level (terminal width 15 → `View()` passes 13 to `composeChromeLine` → expected top edge `╭` + 13 × `─` + `╮` = 15 cells = arg+2).

However, task 1-4's own boundary test rows expect `output = arg` (not `arg + 2`):

- `width 4 → tier 4, output is ╭──╮` (4 cells) — should be `╭────╮` (6 cells = arg+2).
- `width 3 → tier 4, output is ╭─╮` (3 cells) — should be `╭───╮` (5 cells = arg+2).
- `width 2 → tier 4, output is ╭╮` (2 cells) — should be `╭──╮` (4 cells = arg+2).
- `width 15 → tier 4 (top edge is ╭─────────────╮ = corners + 13 filler)` (15 cells) — should be `╭` + 15 × `─` + `╮` (17 cells = arg+2).
- `width 0 → outer == 2, returns ╭╮` — coincidentally correct (arg+2 = 2).
- `width -1 → returns ""` — correct.

The acceptance criterion `Tier-4 degenerate widths 2, 3, 4 each produce a valid output (╭╮, ╭─╮, ╭──╮)` codifies the wrong outputs, and the test names `"width 2 returns two-cell top edge"`, `"width 3 returns three-cell top edge"`, `"width 4 returns four-cell top edge"` lock in the off-by-two error.

An implementer following this task verbatim will write the algorithm correctly (the code is unambiguous), then write tests that fail because the expected fixtures contradict the algorithm. They'd be forced to either weaken the algorithm to `outer = width` (breaking the task-1-8 contract and the entire frame math) or rewrite the test expectations themselves (becoming a design decision they shouldn't have to make). The task-1-9 e2e test would then fail against whichever direction they chose.

The actual "degenerate" widths for tier-4 are widths 0 and 1 (which produce outer=2 → `╭╮` and outer=3 → `╭─╮`). Width 2 produces outer=4 → `╭──╮`, which is no longer degenerate.

**Current**:
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

Do (test thresholds):
```
    - width 200 → tier 1, full name present.
    - width 60 → tier 1, truncated name with '…'.
    - boundary: smallest width where nameBudget == minWindowNameCells (8) — assert tier 1 still active and '…' present.
    - boundary: width where nameBudget == 7 — assert tier 2 selected.
    - width 40 → tier 2.
    - width 25 → tier 3 (verify by strings.Contains(got, compactKeymap) && !strings.Contains(got, 'next pane')).
    - width 13 → tier 4 (output is '╭' + 13 × '─' + '╮' = 15 cells; outer = width + 2 = 15).
    - width 4 → tier 4, output is '╭────╮' (6 cells, outer = 6).
    - width 2 → tier 4, output is '╭──╮' (4 cells, outer = 4).
    - width 1 → tier 4, output is '╭─╮' (3 cells, outer = 3).
    - width 0 → tier 4, output is '╭╮' (2 cells, outer = 2; minimal frame).
    - width -1 → returns ''.
```

Acceptance Criteria entry (replacement):
```
  - Tier-4 widths 0, 1, 2 each produce a valid output ('╭╮', '╭─╮', '╭──╮') — the truly degenerate end of the cascade where outer ≤ 4 leaves no room for chrome.
```

Tests entries (replacement):
```
  - 'width 2 returns four-cell top edge corners and two filler'
  - 'width 1 returns three-cell top edge corners and one filler'
  - 'width 0 returns two-cell top edge corners only'
```

Edge Cases entry (replacement):
```
  - Tier-4 widths 0, 1, 2 — output must be valid and width-correct (outer = width + 2 = 2, 3, 4 cells respectively) without panic. These are the genuinely degenerate cases where the frame has no room for any chrome content.
```

**Resolution**: Pending
**Notes**:

---

### 2. Phase acceptance lower bound on composeChromeLine contract excludes valid widths 0 and 1

**Severity**: Minor
**Plan Reference**: Phase 1 acceptance, second checkbox bullet (`planning.md` line 16)
**Category**: Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
The phase-level acceptance bullet specifies the contract `width + 2` "for every `width >= 2`" and "returns the empty string for `width < 0`". This leaves widths 0 and 1 unspecified at the phase level. The task-1-4 contract (and the algorithm as written) handles both: width 0 → outer=2 → `╭╮`; width 1 → outer=3 → `╭─╮`. Both are valid tier-4 outputs satisfying `output = width + 2`.

Tightening the phase bullet to `width >= 0` removes the gap and matches task 1-4's actual contract (after finding 1 is applied), preventing a future reader of phase acceptance from concluding widths 0 and 1 have undefined behaviour.

**Current**:
```
- [ ] `composeChromeLine` exists as a pure function in `internal/tui/pagepreview.go`, returns a single-row top-edge string (no embedded newlines) of display-cell width `width + 2` for every `width >= 2`, and returns the empty string for `width < 0`.
```

**Proposed**:
```
- [ ] `composeChromeLine` exists as a pure function in `internal/tui/pagepreview.go`, returns a single-row top-edge string (no embedded newlines) of display-cell width `width + 2` for every `width >= 0` (widths 0 and 1 produce the minimum tier-4 frames `╭╮` and `╭─╮` respectively), and returns the empty string for `width < 0`.
```

**Resolution**: Pending
**Notes**:

---

### 3. Task 1-8 Do step mismatches its own acceptance — top-edge styling is not the "two stylings concatenated" form the Do bullet outlines

**Severity**: Important
**Plan Reference**: preview-visual-distinction-1-8 (`tick-5f158b`) — Do (Rewrite View() outline) vs Context (NOTE block from cycle 1 c1)
**Category**: Task Self-Containment / Acceptance Criteria Quality
**Change Type**: update-task

**Details**:
After cycle 1 finding 2/3 fixes, task 1-8 was updated so the Do bullet now consumes `composeChromeLineParts` (introduced in task 1-4) and applies the two-style composition:

> `styledTop := borderStyle.Render(left) + chrome + borderStyle.Render(right)`

And the Acceptance Criteria correctly require: "Chrome content is rendered with no explicit foreground SGR — verified by the structured split test described in Do."

However, the **Context** section still carries the pragmatic-interpretation NOTE block from before the cycle 1 fix:

> *Note: the implementation in this task takes a pragmatic interpretation — tinting the whole top edge with `previewBorderColor` rather than splitting the styling boundary inside `composeChromeLine`'s output. This satisfies the user-visible acceptance criterion ("all four edges coloured") and is a single-point change if reviewers prefer the stricter form.*

This NOTE actively contradicts the post-fix Do/Acceptance, which now requires the strict two-style form. An implementer reading the Context first will believe the pragmatic single-tint form is acceptable, write the simpler `styledTop := lipgloss.NewStyle().Foreground(previewBorderColor).Render(chrome)` form from the cycle-1 Do, and then fail the chrome-no-foreground-SGR test from the post-fix Acceptance. The Context paragraph is stale and must be brought in line with the live Do/Acceptance.

**Current** (from live task description, Context section):
```
From spec § Top edge composition > Color application: the top edge is composed as two stylings concatenated — border parts (corners + padding ─ + filler ─) wrapped in lipgloss.NewStyle().Foreground(previewBorderColor).Render(…) so they pick up the design colour; chrome content rendered with no explicit foreground, inheriting terminal default. Final assembly at the View() call site, conceptually: styledBorder('╭─') + chromeContent + styledBorder(filler + '─╮').
From § Top edge composition > Color application > Implication for composeChromeLine's purity: top-edge styling — border parts coloured, chrome parts default — happens at the call site in View(). The structured split (composeChromeLineParts) exposes the boundary so the call site can apply the two-style composition without re-running cascade logic.
From § Initial sizing and preview-open ordering: viewport.SetSize(max(0, width − 2), max(0, height − 2)) is called once with initial dimensions (same max(0, …) clamp as the resize handler). View() recomputes the chrome line on every tick, so no separate pre-computation is needed at construction time. The first View() call on the freshly-constructed previewModel renders with correct dimensions — no race between preview-open and the first WindowSizeMsg.
From § Style sourcing: corner and edge characters used in the manually-composed top edge are sourced from the chosen lipgloss border value (lipgloss.RoundedBorder()) rather than hardcoded.
```

Note: the actual contradictory NOTE paragraph from cycle-1's Do section was removed from the current Do (good), but the Context above is consistent. Let me look more carefully — re-reading the live task data shows the Context section is now clean and the contradictory NOTE has been removed in cycle 1. **Withdrawing this finding** — on re-read, the live task no longer carries the pragmatic-interpretation NOTE; that text only persists in the stale `phase-1-tasks.md` mirror file. The live tick description is consistent.

**Resolution**: Withdrawn (false alarm — NOTE was already removed in cycle 1, only the stale mirror file retains it)
**Notes**: The repo's `phase-1-tasks.md` file is out of sync with the live tick database; this is a content-staleness concern but not an integrity-of-plan concern under the cycle-2 charter (the live `tick show` output is what implementers will read per the reading.md contract).

---

### 4. phase-1-tasks.md mirror file is stale relative to the live tick database

**Severity**: Minor
**Plan Reference**: `.workflows/preview-visual-distinction/planning/preview-visual-distinction/phase-1-tasks.md` (entire file)
**Category**: Task Self-Containment (incidental — not strictly an integrity criterion, but flagged for operator awareness)
**Change Type**: (no plan change — flag for orchestrator)

**Details**:
The `phase-1-tasks.md` file contains the cycle-0 task content and was not updated when cycle-1 integrity findings were applied to the tick database. Per `reading.md`, implementers locate tasks via `tick show <id>` — so the live source is authoritative and this mirror file is informational only. However, the file's existence and stale state could mislead a casual reader. No action required for plan integrity; flagged so the orchestrator can decide whether to refresh or remove the mirror at conclusion.

**Resolution**: Pending (informational — no plan content change proposed)
**Notes**: Out of scope for cycle-2 integrity remediation per the "Task scope only — check the plan as built" rule. Mentioned for completeness.

---
