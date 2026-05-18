---
status: complete
created: 2026-05-18
cycle: 2
phase: Traceability Review
topic: Preview Visual Distinction
---

# Review Tracking: Preview Visual Distinction - Traceability

## Findings

No findings. The plan is a faithful, complete translation of the specification.

## Coverage Summary

### Direction 1: Specification → Plan

Every spec section maps to plan coverage:

- **Overview / Goal / Non-goals** → Phase-level scope confined to `internal/tui/pagepreview.go`; acceptance criterion explicitly states `pageSessions` `View()` unchanged.
- **Visual treatment > Frame structure** → Task 1-8 composes manual top edge + lipgloss-rendered left/right/bottom.
- **Border style (RoundedBorder)** → Tasks 1-4 and 1-8 source corners from `lipgloss.RoundedBorder()` (no hardcoding).
- **Border colour (AdaptiveColor)** → Task 1-1 declares `previewBorderColor = lipgloss.AdaptiveColor{Light: "#3B5577", Dark: "#7B95BD"}`.
- **Colour robustness (NO_COLOR / 8-16-colour / truecolor)** → Spec explicitly says "No explicit Portal handling is required"; correctly absent from plan.
- **Chrome line content > Segments (Window/Pane/win:/keymap)** → Task 1-4 composes all four segments with `·` separators.
- **Keymap glyphs (⇥ ⏎ ⎋)** → Task 1-1 pins both `verboseKeymap` and `compactKeymap` to spec-exact bytes.
- **Verbose / Compact / Constants** → Task 1-1.
- **Font fallback** → Acceptable-degradation note; correctly no task needed.
- **Width cascade (tiers 1-4 + load-bearing tier 4)** → Task 1-4 implements all four tiers with threshold-boundary tests at 8/7-cell minimum and degenerate widths 2/3/4.
- **Display-cell-aware truncation** → Task 1-3 covers ASCII / CJK / emoji ZWJ / combining marks.
- **Top edge composition > Column layout** → Task 1-4 pseudocode mirrors spec columns.
- **Top edge composition > Color application** → Task 1-8 implements the spec's "two stylings concatenated" composition via `composeChromeLineParts` (cycle-1 finding 1 fix preserved).
- **SGR reset injection** → Task 1-6 covers all six spec edge cases.
- **Resize behaviour** → Task 1-7 wires `tea.WindowSizeMsg` to `viewport.SetSize(max(0, W−2), max(0, H−2))` with clamp test.
- **Initial sizing and preview-open ordering** → Task 1-8 initialises viewport in `NewPreviewModel`; first-frame correctness test asserted.
- **Scroll redraw** → Spec says no special handling; correctly absent from plan.
- **Integration with page state machine** → Spec says no interaction with bootstrap; phase acceptance asserts `pageSessions` `View()` is unchanged.
- **Vertical degradation** → Spec says "render anyway"; correctly absent from plan.
- **Code shape changes** → All file-scope-summary entries mapped: chromeLine() deletion (1-7), previewChromeHeight rename (1-2), NewPreviewModel (1-8), keymap constants (1-1), SGR injector (1-6), `previewBorderColor` declaration (1-1), Style sourcing (1-1/1-4/1-8). The `model.go:1421` no-change is captured in the phase acceptance.
- **Tests > Surface 1-5 + Chrome-row invariant** → Surface 1 (composeChromeLine) → 1-4; Surface 2 (truncation primitive) → 1-3; Surface 3 (SGR injection) → 1-6; Surface 4 (resize) → 1-7; Surface 5 (E2E cascade) → 1-9; Chrome-row invariant → 1-5.

### Direction 2: Plan → Specification

Every task traces cleanly back to spec sections (Spec Reference fields on each task identify the source sections; spot-checked content matches):

- 1-1 (keymap constants + previewBorderColor) → § Keymap glyphs, § Border colour, § Style sourcing.
- 1-2 (previewFrameOverhead rename) → § Code shape changes > Rename previewChromeHeight.
- 1-3 (truncateToCells) → § Display-cell-aware truncation, § Width cascade > Tier 1.
- 1-4 (composeChromeLine + composeChromeLineParts) → § Width cascade, § Chrome line content, § Top edge composition, § Top edge composition > Color application > Implication for composeChromeLine's purity.
- 1-5 (chrome-row invariant test) → § Chrome-row invariant for resize math, § Tests > Chrome-row invariant test.
- 1-6 (injectSGRResets) → § SGR reset injection.
- 1-7 (tea.WindowSizeMsg + delete chromeLine) → § Resize behaviour, § Code shape changes > Replace chromeLine().
- 1-8 (View() + NewPreviewModel viewport init) → § Frame structure, § Border style, § Border colour, § Top edge composition, § Top edge composition > Color application, § Resize behaviour, § Initial sizing and preview-open ordering, § SGR reset injection.
- 1-9 (E2E cascade-tier test) → § Tests > Surface 5, § Test conventions.

No hallucinated content found. No invented requirements, edge cases, or acceptance criteria.

### Cycle 1 findings status

Both cycle-1 traceability findings (chrome-styling deviation at the call site and the missing parts-returning primitive at task 1-4) are resolved:

- Task 1-8 now uses `composeChromeLineParts` to apply the spec's "two stylings concatenated" composition (border parts coloured, chrome content default foreground).
- Task 1-4 now defines both `composeChromeLine` and `composeChromeLineParts` sharing a single private tier-selection helper.

### Documented ambiguity resolutions

Two narrow spec ambiguities are noted in task Context blocks with explicit resolutions; both resolutions are spec-consistent and do not introduce new behavior beyond the spec's column-layout contract:

1. Task 1-3 — ZWJ-sequence truncation behavior (spec leaves unpinned; task treats ZWJ runes as ordinary runewidth-measured units, which matches `go-runewidth` semantics).
2. Task 1-4 — "whitespace padding" between counters and keymap (task uses a single space, with the right-side `─` filler producing the visual right-alignment described in spec § Top edge composition > Column layout).

Both resolutions are within the spec's degree of freedom and are caught by the end-to-end cascade test (1-9) if drift occurs.
