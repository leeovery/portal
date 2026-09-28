TASK: A Card When It Fits, a Plain Stack When It Does Not, on a Full-Pane Canvas (lazy-resume-on-attach-3-2)

ACCEPTANCE CRITERIA:
- A pane exactly the card's width and height renders the card; a pane one column narrower, and a pane one row shorter, each render the plain stack instead — the threshold is read off the built card, so a card width change moves it with no edit here.
- The stack builder is called with the pane's width, and is not called at all when the card fits.
- Every line of the result is exactly `w` display cells wide and there are exactly `h` lines, for every size in the table including `1x1`.
- Under a non-colourless render every cell carries the theme's `canvas` background — no line contains a background reset that is not immediately re-set.
- Under `colourless` the `ansi.Strip`ped output is byte-identical to the non-colourless render's stripped output, and the colourless output carries no SGR background parameter at all.
- A non-positive width or height renders at the fallback dimensions rather than panicking or returning an empty string.
- Content taller or wider than the pane is clamped rather than overflowing, and the clamp keeps the stack's first and last rows: twenty rows in five -> first, the three after it, last; six in three -> first, the one after it, last; two-row pane -> first and last; one-row pane -> first alone.

STATUS: issues_found

SPEC CONTEXT: Spec 5.2 — the panel fills the pane in the active theme with a compact card centred on it; below the size the card needs it degrades to a plain stack of title, command and key hints on the painted canvas, down to the smallest pane a restore can produce, because a waiting pane swallows every other key and one that drew nothing would read as a dead keyboard. NO_COLOR paints no canvas and runs no detection; coverage comes from the alternate screen (5.1), not the fill. 5.4 — the discard confirmation degrades with the pane exactly as the waiting panel does.

IMPLEMENTATION:
- Status: Implemented (with a sound later consolidation — see Notes)
- Location:
  - internal/tui/resume_pane_canvas.go:12-15 — `paneScreenParts{card func() string; stack func(width int) []string}`
  - internal/tui/resume_pane_canvas.go:20-28 — `renderPaneScreen`: `dimsOrFallback`, builds the card, measures it with `lipgloss.Width`/`lipgloss.Height`, centres it via `placeModalOnClearedCanvas` or joins the clamped stack, both wrapped in `fillPaneCanvas`
  - internal/tui/resume_pane_canvas.go:33-46 — `clampStackToPane`: per-row `clampToWidth`, then keeps rows `[0, h-1)` plus the last row; `h == 1` keeps the first alone
  - internal/tui/canvas_fill.go:20-40 — `fillPaneCanvas`: clamp to h rows, backfill via `backfillCanvasBackground`/`canvasBgParams`, pad via `padLineToCanvasWidth`, blank canvas rows below; colourless routes to `fillColourless` (internal/tui/model.go:3101)
  - internal/tui/model.go:2977-2986 — `dimsOrFallback`, extracted from `termDims` so the pane and the picker share one fallback rule
- Notes:
  - `fillPaneCanvas` was authored in resume_pane_canvas.go and later moved to canvas_fill.go by task 3-9, which also made the picker's `fillCanvas` (internal/tui/model.go:3026) call it before its theme-panel composite and gutter inset. That reverses the plan's "new free function rather than a change to the picker's own fill" in form only: the pane still fills edge to edge with no inset and no composite, and the picker now shares one kernel instead of a second copy of the backfill-and-pad loop. The change is sound, and the moved `TestFillPaneCanvas` suite still covers it (internal/tui/canvas_fill_test.go:145).
  - Every acceptance criterion holds as written: the threshold is measured off the card just built, the stack builder is called only on the fallback arm and at the resolved pane width, the stack is clamped in both dimensions before the fill, and the card arm cannot overflow because it is taken only when the measured card fits.
  - Both production consumers (internal/tui/resume_panel.go:32-38, internal/tui/resume_discard_confirm.go:17-23) go through the ladder, and each hands over single-line rows, so the clamp's row count matches the rendered line count.

TESTS:
- Status: Adequate
- Coverage:
  - Ladder: fits / one column short / one row short / threshold read off the built card / stack width recorded and the stack builder not called when the card fits / card called once / centring on a larger pane (internal/tui/resume_pane_canvas_test.go:131-200).
  - Fallback table 0x0, -1x10, 10x-1, which checks both the resolved dimensions and which arm renders (internal/tui/resume_pane_canvas_test.go:202-224).
  - Every-cell canvas paint across every built-in theme over 1x1, one-short in each dimension, exactly-fits and larger (internal/tui/resume_pane_canvas_test.go:245-257). `paneRows` is fatal on the line count and checks every line's width.
  - NO_COLOR: the stripped painted render equals the colourless render byte for byte, and the colourless render carries no escape at all (internal/tui/resume_pane_canvas_test.go:259-278). This would fail if the ladder stopped routing the flag to the colourless fill.
  - Clamp: first-and-last table (twenty in five, six in three, six in two, six in one, plus four in three and two in one), a wide-row width clamp, and truncate-not-reflow (internal/tui/resume_pane_canvas_test.go:280-325).
  - The real-screen smallest-pane edge case (three rows with a command that wraps past one row, two rows, 1x1 renders the title's first cell) is asserted in internal/tui/resume_panel_test.go:251-266.
- Notes: The fake parts builders separate the ladder from either screen's content, as the task asked. "it clamps content taller than the pane" (line 284) only asserts dimensions, but the first-and-last table right below it asserts content, so together they cover the criterion. There are no redundant tests.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, "it ..." subtest names, colours through theme tokens only, no raw hex)
- SOLID principles: Good — the ladder decides between the two forms and fills; screens own their parts; the fill kernel is shared rather than duplicated
- Complexity: Low
- Modern idioms: Yes (`strings.SplitSeq`, `for i := range n`, full-slice expression to avoid aliasing on append)
- Readability: Good
- Issues: One comment states a consequence the code falsifies (see FINDINGS).

BLOCKING ISSUES:
- None

FINDINGS:
- [in-scope] [contained] internal/tui/resume_pane_canvas.go:32 — The comment says "Overflowing instead would scroll the transcript underneath." The only site that writes this render (cmd/state_resume_draw.go:60-62) writes `hydrateAltScreenEnter` (`\x1b[?1049h`) and a cursor-home before it. So an overflowing render scrolls the pane's alternate screen, whose scrolled-off rows tmux discards, and the replayed transcript in the primary buffer is untouched. Spec 5.1/5.2 rests on exactly that isolation ("Coverage does not depend on the fill"). The real cost of overflow is that the render's own first rows (the title the clamp works to keep) scroll off the top of the pane. Replace the sentence with: "Overflowing instead would scroll the screen's own first rows off the top of the pane." — FAILS: the comment misstates why the clamp exists. It credits the clamp with protecting the transcript, which the alternate screen already does, and hides the real reason: overflow would push the title off the pane.

UNSETTLED:
- None
