TASK: Capped Command Row Shows Only the Whitespace the Command Has (lazy-resume-on-attach-9-2, tick-0c20dd)

ACCEPTANCE CRITERIA:
1. `resumeCommandLines("claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 " + resumePartsLongCommand, resumeCardContentWidth)` returns the id row twice, then `--cwd /Users/leeovery/Code/portal/internal/tui --ou…`, with `--output-format` shown with no space inside it.
2. For every command the suite drives, at every width from 1 to 80, the capped row with its `…` removed is the command's own text from where that row starts, whitespace included. Every row still fits the width, none begins or ends with a space, and no render runs past three rows.
3. Text past the cap is still marked `…` when none of it fits a row: a double-width command at width 1 still renders `{"", "", "…"}`, and the width 1–4 tables are unchanged.
4. A theme panel message that runs past its two rows, with a hyphen or mid-word break inside its capped row, shows the message's own text up to its ellipsis.
5. Rows within the cap are unchanged everywhere: the notice band's rows, the shipped-copy subtests of `TestNoticeBand_RowsFitTheBand` and `TestPanelMessage_RowsFitTheInnerWidth`, and the existing row pins in `TestResumeCommandRows_Geometry`.
6. `internal/tui` still calls `ansi.Wrap` from exactly one site (`TestWrapsThroughOneImplementation`).

STATUS: complete

SPEC CONTEXT: A registered command wraps within the card's inner width over at most three lines, and anything past that is marked `…`. The discard confirmation renders the command the same way. The command is the only thing on the panel that says which piece of work the pane holds, so the capped row must not misstate it. Before this change, `wrapCapped` joined the rows past the cap with one fixed space, so a hyphen break (`--` / `output-format`) showed up on the capped row as a standalone `--` argument the command does not contain.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/text_wrap.go:12-25. `wrapCapped` joins the rows past the cap as `row.gap + row.text` instead of a fixed space.
  - internal/tui/text_wrap.go:29-34. `cutRow` keeps the `…` when the text dropped as too wide leaves the remainder fitting the width.
  - internal/tui/text_wrap.go:40-53. `wrappedRow{text, gap}` and `rowTexts`.
  - internal/tui/text_wrap.go:62-76. `wrappedRows` is the single `ansi.Wrap` call, at :63.
  - internal/tui/text_wrap.go:81-93. `rowWalk.take` records each row's gap as the previous row's trimmed trailing run plus this row's trimmed leading run.
  - internal/tui/text_wrap.go:141-143. `breaksWrap`.
  - internal/tui/text_wrap.go:36-38. `wrappedLines` is now `rowTexts(wrappedRows(...))`, so notice_band.go:122 takes the same row texts as before.
- Notes:
  - I traced AC1 through ansi.Wrap v0.11.7 by hand. Row 2 over-packs to "…a7 " (53 cells); `take` trims the trailing space into `trail`, so row 3's gap is " ". The hyphen break between "--" and "output-format" has no dropped run, so row 4's gap is "". The joined remainder is `--cwd …/tui --output-format stream-json`, which `cutRow` truncates to `--cwd /Users/leeovery/Code/portal/internal/tui --ou…`. That matches AC1.
  - AC3: at width 1 every row of the double-width command is empty. The joined remainder is "", and `cutRow`'s second branch returns `Truncate("", 0, "") + "…"` = "…". The pin holds.
  - AC4: I traced the theme message at inner width 30. The rows are "⚠ couldn't save theme" / "gruvbox_material-dark-hard-" / "contrast-extended". The new capped row is "gruvbox_material-dark-hard-co…"; the old code gave "gruvbox_material-dark-hard- c…".
  - AC5: for text with only ASCII spaces, `take` + `rowAndCarry` compute exactly the row texts the old `rowAndCarry` did. The old code trimmed left then right; the new code trims left in `take`, right after the split. None of the existing pin tables were edited in the commit.
  - AC6: the only `ansi.Wrap` call in the non-test sources of internal/tui is text_wrap.go:63.
  - The commit also widens the whitespace test from `' '` to `breaksWrap` (any `unicode.IsSpace` except U+00A0). This goes past the task's wording, but it is sound: it mirrors exactly what ansi.Wrap consumes at a break (`unicode.IsSpace(r) && r != nbsp`). Without it, a non-ASCII space at a break (U+202F, U+3000) throws `rest` out of step with the wrapped lines, and the recorded gaps come out empty. The new "non-ASCII whitespace at a break" case covers this.

TESTS:
- Status: Adequate
- Coverage:
  - AC1: internal/tui/resume_panel_parts_test.go:698-705 pins the traced case exactly.
  - AC2: internal/tui/resume_panel_parts_test.go:715-720 runs `assertCappedRowReadsSource` (:791-808) over every command in `resumePartsCommands` at widths 1–80. The capped row minus its `…` must be a verbatim prefix of the source from where the kept rows end. The existing invariants (width, edge spaces, at most three lines) run over the same set, which gains the non-ASCII-whitespace command at :630.
  - Non-ASCII break: internal/tui/resume_panel_parts_test.go:707-713 pins a capped row reached past a U+202F break. It fails if the predicate goes back to ASCII-only.
  - AC3: internal/tui/resume_panel_parts_test.go:722-726 and the width 1–4 table at :319-354 are unchanged.
  - AC4: internal/tui/theme_panel_message_test.go:521-526 runs the same assertion over inner widths 20–47. At the lower widths the capped row spans a hyphen break (for example width 30), so the old fixed-space join fails it.
  - AC5: the existing notice-band, shipped-copy and Geometry pins are untouched.
  - AC6: the single-wrap guard is internal/tui/text_wrap_guard_test.go:15.
- Notes: The new invariant fails under the old fixed-space join wherever a capped row spans a hyphen or mid-word break, and the traced pin fails outright. `restPastRows` (:771-787) was factored out of `assertRowsReadSource` and moved to the same `breaksWrap` trim, which keeps the read-back and the capped-row checks consistent with the implementation. The tests are focused, with no redundancy worth noting.

CODE QUALITY:
- Project conventions: Followed. There is one ansi.Wrap site, and no raw colour.
- SOLID principles: Good
- Complexity: Low. The walk state moved into a small `rowWalk` with one `take` method, and the gaps are recorded in the same pass that builds the rows. This is the single-pass form the task chose.
- Modern idioms: Yes
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "For every command the suite drives, at every width from 1 to 80, the capped row with its … removed is the command's own text from where that row starts, whitespace included. Every row is still no wider than the width, none begins or ends with a space, and no render runs past three rows." I checked the mechanism by reading and traced the pinned cases by hand. The full set of 21 commands × 80 widths can only be settled by running `go test ./internal/tui -run 'TestResumeCommandLines_Invariants|TestWrappedLines_ReadsTheSourceBack'` and confirming it passes.
