TASK: lazy-resume-on-attach-3-9 (tick-8af98c) — The Canvas Fill Is One Kernel, Not Two

ACCEPTANCE CRITERIA:
- `backfillCanvasBackground` and `padLineToCanvasWidth` are called from exactly one non-test file in `internal/tui`, in one loop — the kernel; a source assertion holds it there so the second copy cannot come back.
- The picker's composed frame is byte-identical before and after, across the sessions page with and without a notice band, the projects page, a modal, the theme panel open, and colourless.
- The theme panel composite still lands after the fill and before the gutter inset, and the panel's own cells are neither backfilled nor padded by the fill.
- The gutter is unchanged: `insetCanvasCanvas` still paints canvas gutters and `insetColourless` still paints SGR-free ones, at the same geometry `gutterPadding` computes.
- Content taller than the content region is still clamped to it, and a region taller than the content is still padded with blank canvas rows — for both callers, through the one kernel.
- The resume panel and discard confirmation render byte-identically to their current output at every size their suites drive.
- Every existing canvas suite passes untouched: `TestCanvasCellBackground_EveryInGridCellIsCanvas`, `TestCanvasCellBackground_TitleAndFooterGaps`, `TestOuterFill_*`, `TestContentInset_GutterPaintedCanvas`, `TestColourless_FillEmitsNoCanvasBackground`.

STATUS: complete

SPEC CONTEXT: The lazy resume panel (spec 5.2) fills the whole pane with the active theme's canvas so nothing behind it shows through, with a card centred on it and a plain stack below the card's size; NO_COLOR (spec 5.2 carve-out) paints no canvas and renders on the terminal's native colours. The picker has painted the same owned canvas since the Modern Vivid reskin. This task collapses the two copies of the per-line fill loop (picker `fillCanvas`, pane `fillPaneCanvas`) into one kernel so a later correction to the per-cell background rule reaches both surfaces.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/canvas_fill.go:11-13 — `canvasStyle`, the one-line canvas style helper
  - internal/tui/canvas_fill.go:20-40 — `fillPaneCanvas`, the single kernel (colourless branch to `fillColourless`; otherwise backfill + pad per line, clamp at h, blank canvas rows to h)
  - internal/tui/model.go:3022-3034 — `fillCanvas` now calls the kernel, then `overlayThemePanelOnContent`, then `insetColourless` / `insetCanvasCanvas(..., canvasStyle(m.themeState.active))`
  - internal/tui/resume_pane_canvas.go:20-28 — `renderPaneScreen` still calls `fillPaneCanvas` for both the card and the stack; the old pane-only comment and the `ansi` import are gone
- Notes:
  - The transformation is byte-equivalent by reading. The removed coloured arm and the kernel differ only in `strings.Split`+range versus `strings.SplitSeq` (same line sequence) and a temporary versus a nested call; `canvasStyle(th)` returns exactly the `lipgloss.NewStyle().Background(th.Canvas.Color())` both copies built inline. The colourless arm is unchanged in effect: the kernel's colourless branch is the same `fillColourless` call the picker made directly.
  - The order fill → panel composite → gutter inset is preserved (model.go:3026-3033), and the load-bearing-order comment on `overlayThemePanelOnContent` (model.go:3036-3039) still holds.
  - Deliberate, sound divergence from the literal wording of criterion 1: `backfillCanvasBackground` is also called from internal/tui/theme_panel_render.go:115 (`themePanelPainter.paint`). That call predates this task (it is present in the parent commit), is not a fill loop (it backfills the panel's own pre-padded rows, which the outer fill must never reach under criterion 3), and calls the same shared function, so a correction to the per-cell SGR rule still reaches it. The guard names the exemption explicitly and allows it for `backfillCanvasBackground` only; `padLineToCanvasWidth` is held to the kernel file alone. Routing the panel through the kernel would contradict criterion 3, so this is not a loss.
  - `fillPaneCanvas` kept its name and signature as the task directs; its comment no longer says it is the pane's alone.

TESTS:
- Status: Adequate
- Coverage:
  - "it paints the per-cell background in one place" (internal/tui/canvas_fill_test.go:20-61): parses the package's non-test sources, fails on any call site outside the allow-list, requires exactly one call of each helper in canvas_fill.go, and fails if the scan finds no call sites at all (so it cannot pass vacuously).
  - "it composes the frame the inline fill composed" (canvas_fill_test.go:94-143): compares `m.fillCanvas(view)` against `preConsolidationFillCanvas` (canvas_fill_test.go:65-92), a frozen copy of the removed inline body that matches the parent commit's code line for line, across all six cases the criterion names: sessions with and without a band (`flashText` drives the band arbiter at notice_band.go:236), projects, the help modal, the theme panel open (with an open-state invariant check), and colourless. It uses a frozen reference implementation instead of a golden file. That is a sound substitute: it is independent code that would diverge if the kernel's composition changed.
  - "it fills both callers' shapes" (canvas_fill_test.go:175-213): the picker's content region (86x20) and a restored pane (20x8), each with a short view, a tall view (clamped), a narrow view, a mid-line SGR reset and colourless, asserting row text, exact width and height, and that every cell carries the canvas background (or none under NO_COLOR).
  - The moved `TestFillPaneCanvas` cases (canvas_fill_test.go:145-173) are verbatim from the deleted resume_pane_canvas_test.go block and still run across every built-in theme.
- Notes: The short-view, tall-view, mid-line-reset and colourless cases appear in both `TestFillPaneCanvas` and the both-callers table. The plan asked for both: the moved cases vary the theme, and the table varies the caller's dimensions. The shared assertion helpers (`paneRows`, `assertEveryCellIsCanvas`, `assertNoBackgroundPainted`) stay in resume_pane_canvas_test.go and are used from both files.

CODE QUALITY:
- Project conventions: Followed (no t.Parallel, guard built on sourceguardtest's `ParsePackageSources` / `ForEachFuncCall` / `CalleeName` / `ParsedSource.Position`, no raw hex, no process-artifact references in comments)
- SOLID principles: Good — one kernel owns the fill; `fillCanvas` owns only the picker-specific composite and gutter
- Complexity: Low
- Modern idioms: Yes (`strings.SplitSeq`, range-over-int)
- Readability: Good
- Issues: None

BLOCKING ISSUES:
- None

FINDINGS:
- None

UNSETTLED:
- "Every existing canvas suite passes untouched: `TestCanvasCellBackground_EveryInGridCellIsCanvas`, `TestCanvasCellBackground_TitleAndFooterGaps`, `TestOuterFill_*`, `TestContentInset_GutterPaintedCanvas`, `TestColourless_FillEmitsNoCanvasBackground`." — Reading settles "untouched": commit 0a95c0691 modifies none of canvas_cell_background_test.go, canvas_paint_test.go, content_inset_test.go or colourless_nocolor_test.go, and the fill output is byte-equivalent by reading. "Passes" can only be settled by running `go test ./internal/tui -run 'TestCanvasCellBackground_|TestOuterFill_|TestContentInset_GutterPaintedCanvas|TestColourless_FillEmitsNoCanvasBackground|TestFillCanvas_|TestFillPaneCanvas'`.
