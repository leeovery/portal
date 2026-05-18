TASK: Implement composeChromeLine Cascade Tiers 1-4 (preview-visual-distinction-1-4)

ACCEPTANCE CRITERIA:
- composeChromeLine is unexported package-level pure function with the spec signature.
- For every `width >= 0` returned string has `lipgloss.Width == width + 2` and no `\n`.
- For `width < 0` returns `""`.
- Tier 1/2/3/4 selection verified by table-driven tests at threshold widths and 8/7-cell name-budget boundary.
- Corner / edge characters sourced from `lipgloss.RoundedBorder()`, not hardcoded.
- Tier-4 degenerate widths 2, 3, 4 each produce a valid output.

STATUS: Complete

SPEC CONTEXT: Spec § Width cascade defines a four-tier predicate-over-output cascade. Each tier produces a candidate, measures it via `lipgloss.Width`, returns it if it fits; otherwise falls through. Tier 1 truncates window name (≥ 8 cells available); tier 2 drops `· win: {name}`; tier 3 swaps to compact keymap; tier 4 is load-bearing corners+filler.

IMPLEMENTATION:
- Status: Implemented (with structural refactor beyond minimum)
- Location:
  - `internal/tui/pagepreview.go:33` — `minWindowNameCells = 8`
  - `internal/tui/pagepreview.go:135-138` — `chromeSegmentSeparator` / `chromeKeymapPadding`
  - `internal/tui/pagepreview.go:145-147` — `tier4Row`
  - `internal/tui/pagepreview.go:169-172` — `composeChromeLine`
  - `internal/tui/pagepreview.go:185-202` — `composeChromeLineParts`
  - `internal/tui/pagepreview.go:213-247` — `selectChromeTier`
- Notes: Split into `composeChromeLine` + `composeChromeLineParts` with `selectChromeTier` + `tier4Row` underneath — used by `View()` to apply BorderForeground colour to border parts while leaving chrome content unstyled. Behaviour byte-identical to spec contract.

TESTS:
- Status: Adequate
- Coverage (`internal/tui/pagepreview_compose_chrome_test.go`):
  - All four tiers at threshold args (200, 103, 102, 95, 50, 13, 2, 1, 0) plus 8/7-cell name-budget boundary.
  - Invariants pinned: `width < 0 → ""`; `lipgloss.Width == arg+2` across 13 thresholds; no embedded newlines; corner glyphs sourced from `lipgloss.RoundedBorder()`.
  - Tier-4 degenerate widths 0, 1, 2 tested.
  - `composeChromeLineParts` concatenation property guards the new structural sibling.
- Notes: UTF-8/CJK/ZWJ coverage lives in 1-3's `truncateToCells` tests.

CODE QUALITY:
- Project conventions: Followed. No `t.Parallel()`, no `tmuxtest`.
- SOLID: `selectChromeTier` single-responsibility tier selector; `composeChromeLine`/`composeChromeLineParts` thin adapters. `tier4Row` factored to prevent drift.
- DRY: Strong. Cascade arithmetic lives in one place.
- Complexity: Low. `selectChromeTier` flat fall-through; `composeChromeLineParts` 3-branch dispatch.
- Modern idioms: Uses Go 1.21+ `max` builtin, `lipgloss.Width`, `runewidth`.
- Readability: Excellent — doc comments explain WHY.
- Security: N/A.
- Performance: Negligible.
- Issues: None blocking.

BLOCKING ISSUES: None.

NON-BLOCKING NOTES:
- [idea] `composeChromeLineParts`/`tier4Row`/`selectChromeTier` extend task 1-4's stated scope to support task 1-8's split-styling. Factoring is clean; flagging for traceability.
- [idea] `selectChromeTier`'s tier-1 filler computation could be simplified to `filler := nameBudget - lipgloss.Width(truncated)` since truncation only shrinks. Defensive but not worth changing.
- [quickfix] Doc comment on `composeChromeLine` (line 154) says "below that, returns the empty string" referring to widths `< 2`, but the function actually returns "" only for `width < 0` and a non-empty tier-4 row for width 0/1. Contract enforced and tested; only wording is ambiguous.
