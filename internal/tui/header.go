package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §3.1 shared header block: the PORTAL wordmark + violet block caret on the
// left, the right-aligned "session manager" subtitle on the same band, over a
// full-width separator rule. It is the first visible chrome of every owned-canvas
// surface (Sessions first; later surfaces compose the same block).
//
// Every cell the header emits carries the owned canvas background (leaf
// .Background(canvas), §1) so there is no terminal-bg island behind the band or
// in the right-aligned spacer gap. The outer fill in View() (model.fillCanvas)
// still owns the line-end pad and the empty rows below — this only paints the
// header's own cells. Under the NO_COLOR carve-out (§2.5) the header drops every
// hue and the canvas background, rendering on the terminal's native fg/bg with
// the structure (wordmark / caret / subtitle / rule) intact.

const (
	// fullWordmark is the letter-spaced PORTAL wordmark (≈0.26em letter-spacing
	// approximated as one cell between each glyph, §3.1). It is the default,
	// full-width form.
	fullWordmark = "P O R T A L"
	// headerCompactWordmark is the narrow-degrade step-2 collapse: the same letters
	// without the per-glyph spacing, so the header still reads "PORTAL" at a narrow
	// width without overflowing.
	headerCompactWordmark = "PORTAL"
	// headerCaret is the one retained retro flourish — a solid block caret in
	// accent.violet, immediately right of the wordmark (§3.1).
	headerCaret = "▌"
	// headerSubtitle is the right-aligned subtitle in text.detail (§3.1).
	headerSubtitle = "session manager"
	// headerRuleGlyph is the full-width separator rule glyph (border.separator).
	// The lower-block sits at the BOTTOM edge of its cell (not the vertical middle
	// like the box-drawing `─`), so the rule does not "float" with whitespace above
	// AND below it — the line lands low in its row, balancing the space above and
	// below the PORTAL wordmark. Unlike the underscore, the lower-block draws as one
	// continuous solid bar across the full width (no inter-cell dashing).
	headerRuleGlyph = "▁"
)

// headerFallbackWidth is the zero/unset-width fallback (matching fillCanvas /
// viewLoading) so the header still composes before the first WindowSizeMsg.
const headerFallbackWidth = 80

// minTerminalWidth is the minimum supported terminal width (§2.7). At or below
// the per-dimension thresholds below the header degrades rather than overflows;
// this is the floor the narrow-degrade tests exercise.
const minTerminalWidth = 40

// Narrow-degrade thresholds (§2.7, progressive per-dimension). Pinned here as the
// implementation detail the spec defers. Measured against the full band layout:
//
//   - headerSubtitleMinWidth: the band is `<wordmark>+caret  <subtitle>` with at
//     least a two-cell gap. Full wordmark+caret is 13 cells (11 letter-spaced + a
//     space + the 1-cell caret), the subtitle is 15 cells; 13 + 2 + 15 = 30. Below
//     30 the right subtitle would crowd the wordmark, so step 1 drops it.
//   - headerWordmarkMinWidth: with the subtitle already gone, the full
//     letter-spaced wordmark+caret needs 13 cells. Below that the wordmark collapses
//     to the compact 6-cell "PORTAL" (+caret = 7) form (step 2) so it never
//     overflows even very narrow terminals.
const (
	headerSubtitleMinWidth = 30
	headerWordmarkMinWidth = 13
)

// headerWidthOrFallback resolves a raw terminal width to the width the header is
// laid out against, applying the zero/unset 80-cell fallback. It is the single
// width source so the budget computation (applySessionListSize) and the render
// (viewSessionList) agree exactly on the header's height.
func headerWidthOrFallback(width int) int {
	if width <= 0 {
		return headerFallbackWidth
	}
	return width
}

// headerStyle returns a leaf style carrying the role token's mode-resolved
// FOREGROUND over the owned canvas Background for the mode — the header's leaf
// paint (mirroring SessionDelegate.tokenStyle). Under the NO_COLOR carve-out it
// returns a bare style (no hue, no canvas) so the run renders on the terminal's
// native fg/bg.
func headerStyle(fg theme.Token, mode theme.Mode, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().
		Foreground(fg.ColorFor(mode)).
		Background(theme.MV.Canvas.ColorFor(mode))
}

// headerCanvasBg returns the structural-spacer style: Background(canvas) for the
// mode, or a bare style under the NO_COLOR carve-out (so the right-aligned
// spacer gap is canvas-painted, not a terminal-bg island).
func headerCanvasBg(mode theme.Mode, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(mode))
}

// headerWordmarkFor selects the wordmark form for the given laid-out width: the
// compact form below the wordmark threshold (§2.7 step 2), the full letter-spaced
// form otherwise.
func headerWordmarkFor(width int) string {
	if width < headerWordmarkMinWidth {
		return headerCompactWordmark
	}
	return fullWordmark
}

// headerShowsSubtitle reports whether the right-aligned subtitle renders at the
// given laid-out width (§2.7 step 1 drops it below the subtitle threshold).
func headerShowsSubtitle(width int) bool {
	return width >= headerSubtitleMinWidth
}

// headerSeparatorRule renders the full-width separator rule beneath the header
// band (§3.1). It is one row of the heavy box-drawing horizontal in
// border.separator (terminal 2px ≈ a heavy/thick horizontal rule, matching the
// Paper frame weight). Under the NO_COLOR carve-out the rule keeps its glyphs but
// drops the colour and the canvas, rendering on the terminal's native fg/bg.
func headerSeparatorRule(width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	rule := strings.Repeat(headerRuleGlyph, w)
	return headerStyle(theme.MV.BorderSeparator, mode, colourless).Render(rule)
}

// blankCanvasRow renders ONE full-width canvas-painted blank row: w spaces styled
// with headerCanvasBg for the mode (Background(canvas) coloured, a bare style —
// native bg — under the NO_COLOR carve-out). It is the structural vertical-spacer
// row the header block stacks between its chrome rows so the blank-row gaps carry
// the owned canvas with no terminal-bg island. Mirrors headerSeparatorRule's
// structure, swapping the heavy rule glyph for a space and the rule colour for the
// bare canvas-background style.
func blankCanvasRow(w int, mode theme.Mode, colourless bool) string {
	return headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", w))
}

// renderHeaderBlock renders the §3.1 header block for the given terminal width and
// resolved canvas mode (and NO_COLOR carve-out). Three rows, top to bottom:
//
//	band, rule, blank
//
// — the wordmark+caret band (right-aligned subtitle filled to width via a
// canvas-painted spacer), then the full-width separator rule FLUSH beneath it (the
// rule is the wordmark's underline — one header unit, no gap between them), then
// ONE blank row (the rule → "Sessions" section-header gap). All three rows carry
// the owned canvas so there is no terminal-bg island between the chrome rows.
//
// Spacing rationale (the deliberate asymmetry, learned the hard way): a blank row
// BETWEEN two glyph rows is a glyph-to-glyph gap — the blank row PLUS the empty
// half-cell under the wordmark PLUS the empty half-cell above the thin rule glyph —
// so it renders ~50% TALLER than the Vinset=1 canvas gutter ABOVE the band (an
// edge-to-glyph gap of one clean row). A blank row below the wordmark therefore
// reads visibly bigger than the gutter above it; counting rows does NOT balance
// them. So the wordmark→rule gap is FLUSH (rule as underline) — the closest the
// whole-row terminal can get to "equal space above and below the wordmark" without
// the blank-row inflation. The breathing room is the single blank below the rule.
//
// The trailing blank (the rule → "Sessions" gap) lives HERE, in the block, rather
// than as a TOP margin on the section header, for two reasons: (1) the single
// headerHeight measurement (lipgloss.Height of this block) is what
// applySessionListSize reserves from the list budget, so folding the gap in here
// makes it auto-budgeted with no separate constant; (2) applySectionHeader swaps
// the section header in by line-0 string surgery that ASSUMES the title is on line
// 0 — a top margin on the section header would shift the title off line 0 and break
// that surgery. (The section header's BOTTOM gap is the TitleBar PaddingBottom in
// applyCanvasMode, which the list measures and reserves separately.)
//
// The block never overflows: a width below the per-dimension thresholds drops the
// subtitle then collapses the wordmark to its compact form (§2.7).
func renderHeaderBlock(width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	band := headerBand(w, mode, colourless)
	rule := headerSeparatorRule(w, mode, colourless)
	blank := blankCanvasRow(w, mode, colourless)
	return lipgloss.JoinVertical(lipgloss.Left, band, rule, blank)
}

// headerBand renders the single header band: PORTAL wordmark + violet caret on
// the left, the right-aligned subtitle on the same band, the gap between them
// filled with canvas-painted spaces so the whole band is canvas (no edge bleed in
// the gap). The band is always exactly w cells wide; if the left segment alone
// already meets or exceeds w (a very narrow terminal) the right subtitle and the
// spacer collapse so the band is clamped to w and never overflows.
func headerBand(w int, mode theme.Mode, colourless bool) string {
	wordmark := headerStyle(theme.MV.TextPrimary, mode, colourless).Bold(true).
		Render(headerWordmarkFor(w))
	caret := headerStyle(theme.MV.AccentViolet, mode, colourless).Render(headerCaret)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	left := lipgloss.JoinHorizontal(lipgloss.Top, wordmark, gap, caret)

	leftWidth := lipgloss.Width(left)

	// No subtitle (narrow degrade step 1) OR the left segment already fills the
	// band: pad the left segment with canvas spaces out to w and return.
	if !headerShowsSubtitle(w) || leftWidth >= w {
		return headerPadRight(left, leftWidth, w, mode, colourless)
	}

	subtitle := headerStyle(theme.MV.TextDetail, mode, colourless).Render(headerSubtitle)
	subWidth := lipgloss.Width(subtitle)

	// If the subtitle no longer fits beside the left segment (leaving room for at
	// least one spacer cell), drop it rather than overflow.
	if leftWidth+1+subWidth > w {
		return headerPadRight(left, leftWidth, w, mode, colourless)
	}

	spacerWidth := w - leftWidth - subWidth
	spacer := headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", spacerWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, subtitle)
}

// padRightWithStyle is the shared right-pad geometry: it returns seg (whose
// rendered width is segWidth) unchanged when it already fills/overflows w, else it
// joins seg with a fill-styled pad of exactly w-segWidth spaces so every cell out
// to w carries the supplied fill (no terminal-bg island). headerPadRight and
// noticeBandPadRight are thin wrappers that bind their respective fill style.
func padRightWithStyle(seg string, segWidth, w int, fill lipgloss.Style) string {
	if segWidth >= w {
		return seg
	}
	pad := fill.Render(strings.Repeat(" ", w-segWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, seg, pad)
}

// headerPadRight pads seg (whose rendered width is segWidth) with canvas-painted
// spaces out to exactly w cells, so the band carries the canvas on every cell
// without an edge-bleed island. A segment already at/over w is returned unchanged
// (the band is clamped to w by construction at the call sites). It binds the canvas
// fill and delegates the pad geometry to padRightWithStyle.
func headerPadRight(seg string, segWidth, w int, mode theme.Mode, colourless bool) string {
	return padRightWithStyle(seg, segWidth, w, headerCanvasBg(mode, colourless))
}
