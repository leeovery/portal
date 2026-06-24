TASK: spectrum-tui-design-5-5 — Honest loading-screen render (VISUAL): PORTAL caret + thick violet bar + ticking step-list (§10.3 / §10.4 / §2.6 / §10.2 / §10.5)

ACCEPTANCE CRITERIA:
- viewLoading renders centred PORTAL caret (text.primary + accent.violet caret) over a thick block bar (filled accent.violet, track bg.track) and a real ticking step-list
- Step rows tick done (glyph state.green / label text.muted-bright), active (glyph accent.cyan / label text.primary), pending (glyph text.faint / label text.dim) per task-5-4 live state; only Restoring sessions shows (N/M)
- First real paint gates on the Phase 1 task 1-7 detect-or-timeout gate — correct canvas from frame one, no flip
- It is a real list (rows), not an in-place text swap
- LoadingMinDuration (1.2s) min-display pad still applies, dual-gated with the terminal complete event
- Narrow/short terminals degrade (compact wordmark, height-driven) and never overflow; NO_COLOR renders colourless on native bg
- Warm path shows no loading screen (behaviour parity)
- VISUAL: a vhs tape drives the TUI to the loading state, writes a PNG, compared against `Loading 6 — Combined (thick bar)` for layout/structure/colour-role

STATUS: Complete

SPEC CONTEXT:
- §10.3 pins the exact composition: centred `PORTAL ▌` (wordmark text.primary + caret accent.violet) over a thick block bar (filled accent.violet, track bg.track) and a tick-list that ticks off as each step completes — a REAL list, not an in-place swap. Tick mapping: ✓ done (glyph state.green / label text.muted-bright), ◐ active (glyph accent.cyan / label text.primary), · pending (glyph text.faint / label text.dim). Bar weight is thick (decided).
- §10.4 maps 11 real steps → 5 friendly labels; only "Restoring sessions" carries an N/M counter; M=0 suppresses the counter and ticks ✓ immediately.
- §2.6 / §10.2: the first real paint gates on detect-or-timeout so the loading page paints the correct canvas from frame one — never a paint-then-flip. NO_COLOR skips detection and the canvas.
- §2.9 token roles confirm: text.detail covers "counts"; text.primary/muted-bright/dim/faint ramp matches the §10.3 label mapping exactly.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/tui/loading_view.go — the whole §10.3 render: renderLoadingScreen (143), composeLoadingBlock (182), renderLoadingWordmark/renderBlockWordmark (271/299), renderLoadingBar (344), renderTickList/renderTickRow (384/439), tickRowTokens (462), spacedCounter (489), renderErrorFooter (403, §10.5).
  - internal/tui/model.go — viewLoading (3793) reads loadingProgress.View() (or FailedView on fatal); BootstrapProgressMsg arm folds into the accumulator (2034); dual-gate in the LoadingMinElapsedMsg (2002) + BootstrapCompleteMsg (2036) arms; first-paint gate in View() (3260) holds the blank frame until modeResolved(); colourless flag threaded through.
  - internal/tui/loading_progress.go (task 5-4) — the pure accumulator viewLoading consumes.
- Notes:
  - Token mapping matches §10.3 / §2.9 exactly (done state.green/text.muted-bright; active accent.cyan/text.primary BOLD; pending text.faint/text.dim; counter text.detail; failed-row state.red ✗ for §10.5).
  - The note about task 8-5 is satisfied: loadingStyle/loadingFg delegate to header.go's headerCanvasBg/headerStyle, and the degrade reuses fullWordmark/headerCaret/headerCompactWordmark — verified against CURRENT code, no stale local copy.
  - The first-paint gate is shared (View()'s modeResolved() check), so the loading page paints the resolved/dark-fallback canvas from frame one — no flip. Correct per §2.6/§10.2.
  - Bar fraction is derived from completed-step count /11 (task 5-4), bar width derived from the rendered wordmark width (spans the full logo) and clamped to w — a real layout-correction, not a magic 30.

TESTS:
- Status: Adequate
- Coverage (internal/tui/loading_view_test.go):
  - block banner + caret bar + thick bar tokens + 5-row list (RendersBlockBannerCaretBarAndList)
  - bar spans full wordmark width (BarWidthEqualsWordmarkWidth); centred column (BlockColumnIsCentered); 2-row section gaps (SectionGapsAreTwoRows); tick rows left-aligned within the list (TickRowsLeftAlignedWithinList); caret flush across ragged rows (CaretIsFlushAcrossBannerRows)
  - per-state glyph + label tokens from live progress (TickStatesUseSpecdTokens)
  - spaced `8 / 12` counter only on active restore, text.detail, exactly once, un-spaced form not leaked (CounterSpacedOnlyOnActiveRestore); M=0 suppression (SuppressesCounterWhenM0)
  - real list not in-place swap — each label on its own single line (IsRealListNotInPlaceSwap)
  - first-paint canvas gate, blank-before-resolve then dark-canvas-after, no flip (PaintsCanvasFromFrameOneGated)
  - dual-gate both orderings (TransitionDualGated)
  - narrow degrade block→single-row→compact with no overflow + threshold boundary (DegradesNarrowWithoutOverflow); short no-overflow + list never cut (ShortNoOverflow); error-frame height never overflows across the short range (ErrorFrameNeverOverflowsHeight)
  - NO_COLOR: no canvas/hue, glyph-distinct (ColourlessNoCanvasGlyphDistinct)
  - warm path never lands on PageLoading (TestWarmPath_NoLoadingScreen)
  - §10.5 error-frame centred composition + canvas-island guard (ErrorFrameCentredComposition, CentredPaddingCarriesCanvasNoIslands)
- Notes: Tests fold real BootstrapProgressMsg sequences through the accumulator (midRestoreProgress) rather than hand-constructing view state, so they exercise the same path the live channel drives. Not over-tested — each test pins a distinct invariant. Every acceptance criterion and listed micro-test has a corresponding assertion.

VISUAL VERIFICATION:
- testdata/vhs/loading.tape drives cmd/capturetool --fixture loading-screen (in-memory tmux fakes, no real server/daemon/config), seeds the reference mid-restore state, blocks the receiver so the page parks, pins the dark canvas, and writes testdata/vhs/loading.png.
- Compared testdata/vhs/loading.png against testdata/vhs/reference/loading-mv.png (`Loading 6 — Combined (thick bar)`): MATCH for layout/structure/colour-role — PORTAL block wordmark (text.primary lavender) + flush violet caret; thick partial-filled violet bar over a dark bg.track track spanning the wordmark width; 5-row tick-list with green ✓ done rows, cyan ◐ bold "Restoring sessions  8 / 12", faint · pending rows; centred on the dark owned canvas. Behaviour parity (warm path = no loading screen) confirmed by TestWarmPath_NoLoadingScreen.
- Acceptable, expected divergence: the captured terminal frame has tighter inter-section vertical rhythm than the Paper px gaps (terminal-native spacing — documented design-vs-terminal convention), not a defect.

CODE QUALITY:
- Project conventions: Followed. Leaf token/canvas styles delegate to the single header.go source (no re-implemented carve-out); 4+-arg calls split one-per-line; doc comments carry the §-references and the load-bearing rationale (caret-flush, ragged-row pad, height-degrade order).
- SOLID principles: Good. Render layer is a pure projection over task-5-4's accumulator (clean single-responsibility split: accumulator = state, view = pixels); the mapping lives in exactly one site (tickRowTokens) so no row drifts.
- Complexity: Acceptable. composeLoadingBlock's height-degrade ladder is the densest spot but is linear, well-commented, and pinned by the short/narrow overflow tests.
- Modern idioms: Yes. Value-receiver pure funcs, lipgloss JoinVertical/Place composition, ansi.Truncate for clamp.
- Readability: Good. Intent-named helpers (singleRowWordmarkHeight, loadingSectionGap, blockBannerMaxRowWidth) keep the degrade arithmetic legible.
- Issues: One write-only field (latestProgress) — see non-blocking notes.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] internal/tui/model.go:323-326,2029 — `latestProgress BootstrapProgressMsg` is write-only dead state: it is assigned in the BootstrapProgressMsg arm but never read anywhere (the render path uses the folded `loadingProgress` accumulator). Its own doc comment ("Kept for any consumer that reads the raw last event") describes a consumer that does not exist. Remove the field and its assignment (touches struct + one arm — mechanical, no behaviour change).
- [do-now] internal/tui/loading_view.go:84-94 — the const block comment block at line 84 ("loadingBlockWordmarkWidth is the rendered width...") documents a `loadingBlockWordmarkWidth` constant that is not in this block (the block declares loadingTickGlyphSlot / loadingTickGap / loadingQuitHint); the banner-width doc actually belongs to `loadingBlockBannerWidth` at line 96. Re-point/trim the stale doc sentence so it does not describe a non-existent identifier.
