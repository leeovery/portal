TASK: spectrum-tui-design-2-2 — Header block: PORTAL wordmark + violet caret + right-aligned `session manager` subtitle + full-width 2px separator rule, with narrow degrade, one-row-per-delegate pagination invariant preserved, owned-canvas render with no edge bleed.

ACCEPTANCE CRITERIA (from tick-21cdbd):
- Header renders PORTAL (uppercase letter-spaced heavy text.primary) + immediately-right block caret (accent.violet) + right-aligned `session manager` subtitle (text.detail) over a full-width 2px border.separator rule, all via tokens (no literal hex).
- Header height subtracted from the list height budget at every size-apply call site; one-row-per-delegate invariant holds (no overflow; grouped page count too).
- Narrow degrade: below the minimum width the subtitle drops then the wordmark collapses to compact; never overflows (2.7 progressive per-dimension).
- Header paints on the owned canvas with no edge bleed (leaf .Background(canvas)); the Phase 1 outer fill is unmodified.
- VISUAL VERIFICATION (mandatory): vhs tape drives the TUI to Sessions flat and writes a PNG; header region matches Sessions — Modern Vivid v2 / (Light) for layout/structure/colour-role.
- Behaviour parity: only chrome changes; the one behavioural delta is the recomputed list height keeping pagination exact.

STATUS: Complete

SPEC CONTEXT:
- §3.1 (header): wordmark PORTAL uppercase letter-spaced (~0.26em) heavy text.primary (decorative — exempt from text-contrast ratio); caret solid block `▌` accent.violet immediately right; subtitle right-aligned `session manager` text.detail; full-width 2px border.separator rule under the header; narrow degrade collapses wordmark to compact + drops subtitle (per §2.7).
- §2.7 (narrow degrade): progressive, per-dimension — width-driven steps apply in order (drop right header hint → wordmark→compact → truncate names); never overflows; one-row-per-delegate invariant always holds.
- §3.5 (pagination invariant): bubbles/list height-driven paginator; any rows the header consumes must be subtracted from the list height budget.
- §3.6 (no full-screen frame): owned canvas is a flat fill, not a frame; structure carried by the two horizontal rules + per-element treatments.
- §3 note: measurements are Paper-frame reference values; "exact cell mapping is finalised at implementation (terminal cells, not web px)" — so "2px" maps to a terminal-cell-appropriate heavy rule.

IMPLEMENTATION:
- Status: Implemented (clean, well-factored).
- Location:
  - internal/tui/header.go — full header block:
    - renderHeaderBlock (header.go:177) → JoinVertical(band, rule, blank); 3 rows.
    - headerBand (header.go:191) — wordmark (text.primary, Bold) + canvas gap + caret (accent.violet) JoinHorizontal'd, right-aligned subtitle (text.detail) with a canvas-painted flex spacer; clamps to w, never overflows.
    - headerSeparatorRule (header.go:127) — full-width `▁` lower-block rule in border.separator.
    - headerWordmarkFor / headerShowsSubtitle (header.go:109,118) — §2.7 progressive degrade selectors (compact wordmark below headerWordmarkMinWidth=13; drop subtitle below headerSubtitleMinWidth=30).
    - headerStyle / headerCanvasBg (header.go:87,99) — leaf .Background(canvas) paint; bare style under NO_COLOR (§2.5 carve-out).
    - headerWidthOrFallback (header.go:75) — single width source; zero/unset → 80 fallback.
    - padRightWithStyle / headerPadRight (header.go:225,238) — shared right-pad geometry (DRY: also reused by the §11 notice band per noticeBandPadRight).
  - internal/tui/model.go:
    - renderHeader (model.go:4148) + headerHeight (model.go:4227) — single render/measure entry points, BOTH resolved against m.contentWidth() and m.canvasMode/m.colourless, so budget and render agree exactly.
    - applySessionListSize (model.go:1299, reserved at 1312) folds m.headerHeight(width) into the list budget alongside footer + notice band.
    - applyProjectListSize (model.go:1360, reserved at 1361) also reserves m.headerHeight — header reused on Projects page.
    - viewSessionList composes header FIRST: JoinVertical(Left, header, [slot,] listView, footer) (model.go:4104/4106).
- Notes:
  - No literal hex anywhere in header.go (grep clean) — every colour flows through theme.MV tokens (TextPrimary, AccentViolet, TextDetail, BorderSeparator, Canvas). All five tokens exist in theme.go.
  - "2px rule": spec §3.1 says "2px" but §3 explicitly defers exact cell mapping to implementation ("terminal cells, not web px"). Implemented as a single-row heavy lower-block rule (`▁`), documented in-source (header.go:37-43, 122-126) and pinned by TestHeaderBlock_SeparatorRule (height==1). Matches the Paper reference (one thin full-width line) — correct interpretation, NOT a drift.
  - Budget/render width-agreement is the load-bearing invariant and it holds: both sides use m.contentWidth() (the inset content region), so the gutter is accounted for once and consistently.

TESTS:
- Status: Adequate (thorough, not over-tested).
- Coverage (internal/tui/header_test.go — 13 tests):
  - RendersWordmarkCaretSubtitleRule (dark+light) — glyph presence + per-token foreground SGR for all four roles; every line exactly w wide.
  - VerticalRhythm — pins the 3-row band/rule(flush)/blank structure; blank row has no visible glyph.
  - BlankRowsPaintCanvas — blank rows carry canvas bg coloured, bare under NO_COLOR.
  - SeparatorRule — full-width single-row heavy rule.
  - NarrowDegradeProgressive — full → drop subtitle (wordmark stays) → compact wordmark, step order enforced.
  - NeverOverflowsAtMinWidth — multiple narrow widths incl. minTerminalWidth, never exceeds w.
  - PaintsOnCanvasNoEdgeBleed (dark+light) — canvas bg sequence present.
  - ColourlessDropsHueAndCanvas — §2.5 carve-out: structure intact, no canvas bg, no foreground hue.
  - ZeroWidthFallsBackTo80 — fallback composes.
  - HeaderHeight_EqualsThreeRows (coloured+colourless) — the budget contract value (3).
  - ComposesHeaderFirst — header wordmark precedes the list title in viewSessionList.
  - HeaderHeight_SubtractedFromListBudget — composed view <= termH, filled frame == termH.
  - HeaderHeight_CountedAtEverySizeApplySite — construction seed → resize → rebuild path stays within termH.
- Notes:
  - The colour-role assertion via tokenFgSeq (the `38;2;r;g;b` core substring) is a smart, robust way to assert role-token usage even when fg+bg+bold merge into one SGR — directly proves "via tokens, no literal hex" at render time.
  - The micro-acceptance "leaves list navigation/selection/filtering behaviour unchanged" has no dedicated assertion in header_test.go. It is covered indirectly: the change is purely additive chrome (no nav/selection/filter code touched) and the budget tests confirm pagination stays exact. Behaviour parity rests on the broader GREEN suite rather than a header-scoped nav test. Acceptable for a chrome-only task; noted below.
  - Grouped page-count re-verification (acceptance bullet) is exercised by the broader grouped-view tests + the shared applySessionListSize chokepoint (grouped modes route through the same budget), not by a header-specific grouped overflow test. Adequate via the shared chokepoint.

CODE QUALITY:
- Project conventions: Followed. Token-layer indirection (no hex at call site), leaf .Background(canvas) mirrors SessionDelegate.tokenStyle, NO_COLOR carve-out handled consistently, single width source. No t.Parallel (matches repo rule).
- SOLID principles: Good. SRP per function (band / rule / blank / wordmark-select / subtitle-gate / pad each isolated); width/mode/colourless threaded as params (no hidden state).
- DRY: Good. padRightWithStyle is a genuine shared-geometry extraction reused by header + notice band (the Phase 10-1 refactor); no copy-paste of pad logic.
- Complexity: Low. headerBand has the only branching (subtitle fits / drop / clamp), each branch clear and commented.
- Modern idioms: Yes. Idiomatic lipgloss JoinVertical/JoinHorizontal composition; strings.Repeat for fills.
- Readability: Excellent. Extensive rationale comments (the deliberate flush-rule asymmetry, the trailing-blank-lives-here-not-as-section-margin reasoning, the threshold arithmetic) — self-documenting and explains WHY, not just what.
- Issues: None blocking.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] internal/tui/header_test.go — Add a header-scoped behaviour-parity assertion (e.g. a table that drives a few nav/filter keypresses through a model with the header present and asserts selection index / filter state match the pre-header expectation), so the "list nav/selection/filter unchanged" micro-acceptance has a header-local guard rather than relying solely on the broader suite. Decide whether this is worth the duplication given the change is provably additive.
