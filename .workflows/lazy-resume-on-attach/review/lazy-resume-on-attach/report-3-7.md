TASK: The Wrap Re-Flows Its Overshoot Instead of Dropping It (lazy-resume-on-attach-3-7)

ACCEPTANCE CRITERIA:
- `resumeCommandLines("a-b-c-d-e-f -- --port=3000 --resume", 12)` renders the standalone `--` argument on the following row instead of dropping it, and the same command at width 6 keeps it too.
- Every line is still no wider than the width asked for, at every width from 1 to 80 across the suite's commands.
- No line `resumeCommandLines` returns begins or ends with a space, at any width from 1 to 80.
- The only text a render loses is text past `resumeCommandMaxLines`, and that row carries `resumeEllipsis`: a command that fits three re-flowed rows carries every character it was given, in order.
- A grapheme wider than the width terminates rather than looping: a double-width command renders `{"", "", "…"}` at width 1, and the widths 1-4 tables pass at their re-measured values.
- The degraded stack is untouched: `clampStackToPane` still truncates its styled rows and still renders no more rows than the pane's height.
- `RenderResumePanel` and `RenderResumeDiscardConfirm` still return exactly Width × Height cells at every size their suites drive, card and plain stack alike.

STATUS: issues_found

SPEC CONTEXT: The waiting panel and the discard confirmation both render the registered command, which is the only thing on either screen saying which piece of work the pane holds. The spec requires a command longer than the card to wrap within the card's inner width over at most three lines, with anything beyond marked `…`, and the card's width unchanged; the discard confirmation renders it the same way. The degraded (plain-stack) form below the card's size must keep the key hints visible and never overflow the pane.

IMPLEMENTATION:
- Status: Implemented
- Location: internal/tui/text_wrap.go:30-47 (`wrappedLines` — the re-flow walk), :52-62 (`droppedGap` — restores the space the wrap consumed at a break so a carry is not glued to the next line), :68-75 (`rowAndCarry` — trim, split at width, carry the overshoot), :80-87 (`splitAtWidth` — the termination bound for a grapheme wider than the width), :89-94 (`clampToWidth`, unchanged); internal/tui/resume_panel_parts.go:30-32 (`resumeCommandLines` reaches it through `wrapCapped`); internal/tui/resume_pane_canvas.go:33-46 (`clampStackToPane`, untouched, still truncating through `clampToWidth`).
- Notes: The code has moved on from the plan's wording, and the move is sound. The re-flow lives in `text_wrap.go` rather than `resume_panel_parts.go` because task 3-8 later moved it there, as the plan intended. The implementation also goes beyond the "Do" list by carrying the whitespace the wrap dropped at a break (`droppedGap`) onto the carry. Without that, the worked command's `--` would be glued onto `--port`. With it, the rows read the source back in order. I traced the pinned `ansi.Wrap` v0.11.7 by hand on the worked command. At width 12 it emits `"a-b-c-d-e-f -- " / "--port=3000 " / "--resume"`, and the re-flow gives `{"a-b-c-d-e-f", "-- --port=30", "00 --resume"}`. At width 6 it gives `{"a-b-c-", "d-e-f", "-- --…"}`. In both cases the standalone `--` stays on screen. Every row is bounded by `ansi.Truncate` at width and trimmed at both edges. The capped row is truncated with `…` and trimmed. So the width and edge-space invariants hold by construction. Every carry iteration shrinks the carry by at least one grapheme, because `TruncateLeft` returns a strictly narrower suffix or the fallback strips the first cluster, so the loop terminates. The `…`-marked capped row inherits one defect from the cap's join; it is recorded under FINDINGS.

TESTS:
- Status: Adequate
- Coverage: Every test the plan names exists.
  - The re-flow table at widths 12 and 6: internal/tui/resume_panel_parts_test.go:148-163.
  - The third-row `…`: :165-173.
  - Width 1-80 invariants: no line wider than the width at :646-654, no edge space at :656-664.
  - Lossless reconstruction of commands that fit three rows: :674-695.
  - The empty row for a grapheme wider than the width: :697-701.
  - The degraded-stack truncation: internal/tui/resume_pane_canvas_test.go:298-302.

  Beyond the named tests, `TestWrappedLines_ReadsTheSourceBack` (:727-756) checks at the kernel level, across every command and width 1-80, that the rows read the source back in order with nothing glued together. That is the test that would fail if the carry dropped, duplicated or glued text. I re-derived the widths 1-4 tables (:319-354) from the pinned library by hand for both commands, and all eight rows match. So does the 24-column "carries the command's text" pin (:305-309). A restored clamp would fail both the re-flow pin and `assertLinesReconstruct`.
- Notes: No test covers the defect under FINDINGS. `assertRowsCarryCommand` (:69-80) compares space-stripped text, and the edge-space invariant checks only row edges, so a space inserted inside the capped row passes both.

CODE QUALITY:
- Project conventions: Followed (no `t.Parallel`, in-package unit tests, a single `ansi.Wrap` site enforced by internal/tui/text_wrap_guard_test.go)
- SOLID principles: Good — each helper does one job: walk, gap, split, terminate
- Complexity: Acceptable
- Modern idioms: Yes
- Readability: Good; the comments state the reasons and match the code. No comment still claims the clamp cuts the overshoot away.
- Issues: None beyond FINDINGS

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [spreading] internal/tui/text_wrap.go:17 — `wrapCapped` re-joins the rows past the cap with a single space whether or not the source has one there. Not all of `wrappedLines`' row boundaries are spaces: `ansi.Wrap` also breaks straight after a hyphen. So when the last kept row ends on a hyphen break shorter than the width, the capped row shows a space the command does not contain. Traced against the pinned library: `resumeCommandLines("claude --resume 4f2c9a1e-7b33-4d01-9f6a-2c8e510db4a7 " + resumePartsLongCommand, resumeCardContentWidth)` wraps to the rows `claude --resume <uuid>` ×2, `--cwd /Users/leeovery/Code/portal/internal/tui --` (49 cells, a hyphen break) and `output-format stream-json`. Its third row therefore reads `--cwd /Users/leeovery/Code/portal/internal/tui -- o…` where the command has `--output-format`. The rule predates this task (the cap's join, which task 3-8 extracted), so this is not a re-flow regression. It does contradict this task's stated outcome: the command on screen is the command that will run, up to the `…`. Fix: build the capped row from the source remainder after the last kept row, or have `wrappedLines` report per-boundary gaps so the join restores only the whitespace the source had. Pin the case above as a row. The fix has more than one defensible form. It also moves `themePanelMessageText` (internal/tui/theme_panel_message.go:131), the other `wrapCapped` caller. — FAILS: on the card and the discard confirmation, a user sees a standalone `--` separator argument that the registered command does not contain, and approves or discards on that misreading. No test observes it: `assertRowsCarryCommand` strips spaces, and the edge-space invariant checks only row edges.

UNSETTLED:
- None
