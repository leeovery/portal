TASK: spectrum-tui-design-1-6 — Owned mode-matched canvas paint: leaf `.Background(canvas)` + outer full-terminal fill as the last layer

ACCEPTANCE CRITERIA:
- (VISUAL) vhs capture of the Sessions foundation screen shows every cell painted the canvas colour (#0b0c14 dark / #e1e2e7 light) — no edge bleed at line ends, no unpainted mid-screen rows — matching `Sessions — Modern Vivid v2` (dark) / `Sessions — Modern Vivid (Light)` (light), flat full-terminal fill with NO frame (§3.6).
- The outer fill is a single wrap point in model.go View(), applied as the last layer over the assembled per-page view; the leaf styles carry `.Background(canvas)`.
- The one-row-per-delegate pagination invariant is preserved: the fill is outside the list's height budget; a dynamic vertical change re-pads to termH without changing the list's row count or overflowing the viewport.
- Zero-size (termWidth/termHeight == 0) falls back to safe defaults so the fill never blanks the screen.
- Behaviour parity vs the pre-reskin Sessions implementation: navigation, selection, filtering, and key handling identical underneath the new fill.

STATUS: Complete

SPEC CONTEXT: §1 (Canvas ownership) mandates Portal paints its own mode-matched canvas on every cell in two layers — (1) leaf `.Background(canvas)` on every text/accent run, and (2) an outer full-terminal fill (Width=termW, Height=termH, Background=canvas) wrapping the already-composed view as the LAST layer, so no edge bleeds and empty mid-screen rows are painted. The fill must NOT participate in the list's height budget (§3.5 / §4.1 one-row-per-delegate pagination invariant — the original cursor-invisible / missing-title / left-shift overflow bug class, §5.1). §3.6: flat full-terminal fill, NOT a frame. §15.1: foundation frames are the dark/light Sessions MV references. §2.5 NO_COLOR carve-out suppresses the canvas entirely. Task explicitly permits a temporary/injected mode source ahead of detection task 1-7.

IMPLEMENTATION:
- Status: Implemented (exceeds the minimal task scope — see Notes)
- Location:
  - Single outer wrap point: `internal/tui/model.go:3265` — `View()` → `tea.NewView(m.fillCanvas(m.viewString()))`, the last layer over the per-page dispatch in `viewString()` (model.go:3772). Confirmed there is no per-page fill anywhere else.
  - Outer fill: `fillCanvas` (model.go:3418), with `insetCanvasCanvas` / `fillColourless` / `insetColourless` placers and `gutterPadding` (model.go:3458–3556).
  - Mid-line backfill: `backfillCanvasBackground` + `rewriteSGRWithCanvasBg` / `sgrBackgroundActive` / `canvasBgParams` / `padLineToCanvasWidth` (model.go:3558–3768).
  - Injected mode seam: `WithCanvasMode` / `m.canvasMode` (default theme.Dark), swapped in by 1-7 without touching the wrap (canvas_paint_test.go:30–53 pins this).
  - Leaf `.Background(canvas)`: `SessionDelegate`/`ProjectDelegate` via `tokenStyle`/`canvasBg` (session_item.go:194–278), footer/header/pagination help styles (`applyCanvasMode` model.go:1123, `canvasHelpStyles`/`canvasPaginationDots` model.go:879–922), title bar (model.go:1175).
  - Zero-size fallback: `termDims` (model.go:3329) → 80×24 via `fallbackTermWidth`/`fallbackTermHeight`, matching viewLoading.
  - List budget folds the inset + header + footer + notice band: `applySessionListSize` (model.go:1299) → `applyListSize` (model.go:1278).
- Notes: The implementation goes materially beyond the literal task. Two extensions landed here (or were absorbed into this surface) that the task text does not require: (a) a global content-gutter inset (Hinset=2 / Vinset=1) folded into the width AND height budgets, and (b) a per-line `backfillCanvasBackground` SGR rewrite that paints every interior cell with an explicit canvas SGR (the mosh/Blink mid-line-bleed fix, since OSC 11 alone leaves gaps on terminals that ignore it). Both are correct, well-documented, and folded into the budget so the pagination invariant is not perturbed — they do not constitute drift from the acceptance criteria, but they are extra surface area carried under this task's banner. The single-wrap-point and budget invariants both hold with the extensions present.

TESTS:
- Status: Adequate (strong, mapped 1:1 to the task's test list, plus the inset/cell-bg extensions)
- Coverage:
  - canvas_paint_test.go covers all seven task tests: paints canvas on every cell + dark/light full-bleed (TestOuterFill_PaintsEveryCellTheCanvas), outside the list height budget (TestOuterFill_OutsideListHeightBudget asserts PerPage unchanged), re-pads to termH on a flash band (TestOuterFill_RePadsToTermHOnVerticalChange), pagination invariant at a short terminal (TestOuterFill_PaginationInvariantPreserved), zero-size 80×24 fallback (TestOuterFill_ZeroSizeFallback), and the injected-mode seam (TestWithCanvasMode / TestCanvasMode_DefaultsToDark).
  - canvas_cell_background_test.go is a terminal-independent cell-by-cell parser asserting EVERY in-grid cell carries an explicit background (canvas or a content tint) — the precise mosh/Blink regression gate, with a targeted title-row/footer-gap spot-check.
  - content_inset_test.go covers the inset budget folding, frame-dims-unchanged, gutter-painted-canvas, NO_COLOR native-bg gutter, grouped + flat pagination invariant, tiny-terminal clamp boundary, zero-size, and nav parity (TestContentInset_NavigationUnchanged).
  - Behaviour-parity (task test 7) is covered by TestContentInset_NavigationUnchanged plus TestColourless_NavigationParity / TestColourless_FilterParity (colourless_nocolor_test.go). The structural argument is sound: `fillCanvas` wraps a pure `viewString()` and `Update` is untouched, so nav/selection/filter are provably cosmetic-or-nil.
  - Visual gate: foundation captures committed (sessions-flat.png, sessions-flat-light.png, sessions-flat-nocolor.png) beside the references (reference/sessions-modern-vivid-v2.png, reference/sessions-modern-vivid-light.png). The dark capture I inspected matches the reference for the canvas paint — full-bleed flat fill, no UI frame, inky #0b0c14 painted edge-to-edge, violet selection bar/tint, green attached marker, full chrome.
- Notes: No under- or over-testing observed. The cell-by-cell parser duplicates production's SGR-classification logic (applySGR mirrors sgrBackgroundActive) — intentional and load-bearing as an independent oracle, not redundant.

CODE QUALITY:
- Project conventions: Followed. All canvas colour sourcing flows through `theme.MV.Canvas.ColorFor(mode)` — no raw hex at any call site (consistent with the §2.9 closed-vocabulary rule established in 1-3). Small interfaces / injected seam (`WithCanvasMode`) match the codebase DI pattern. No `t.Parallel()` in the new tests (correct).
- SOLID principles: Good. Single wrap point, single content-region derivation (`insetRegion` shared by budget + placement so they cannot drift), shared `gutterPadding` so coloured and colourless frames are byte-identical in layout. `padLineToCanvasWidth` / `backfillCanvasBackground` are focused single-purpose helpers.
- Complexity: Acceptable. `backfillCanvasBackground` + its SGR helpers are intrinsically intricate (an ANSI state machine), but the complexity is well-contained, the parser is reused per-frame (documented perf note), and each helper is small with a clear contract.
- Modern idioms: Yes. Idiomatic Go, `strings.Builder` with `Grow`, reused `ansi.Parser`.
- Readability: Good — exceptionally thorough doc comments anchored to spec sections; intent is clear throughout.
- Issues: None blocking. One deliberate documented separation: `padLineToCanvasWidth` does NOT truncate a content line wider than contentW (returns it untruncated), which would break the "every line == termW" invariant IF a line over-ran. This is correct here — over-long names truncate via §2.7 (a separate task) before reaching the fill, and the foundation tests confirm no over-run occurs — so it is a documented degrade boundary, not a latent bug in this task's scope.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/model.go:3761 (padLineToCanvasWidth) — decide whether the over-width branch (gap<=0 returns the line untruncated) should defensively clamp/truncate to contentW so the termW invariant is structurally guaranteed rather than relying on upstream §2.7 name truncation. Currently safe and documented; this is a "should we harden the boundary" design call, not a fix, and belongs to the §2.7 truncation task.
