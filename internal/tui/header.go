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
	// A box-drawing horizontal renders the §3.1 "2px" rule as a single thin
	// full-width line matching the Paper frame weight (terminal 2px ≈ a heavy/thick
	// horizontal rule — the frame shows one thin full-width line, not a 2-row band).
	headerRuleGlyph = "─"
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

// renderHeaderBlock renders the §3.1 header block for the given terminal width
// and resolved canvas mode (and NO_COLOR carve-out). It joins the wordmark+caret
// band — with the right-aligned subtitle filling the band to width via a
// canvas-painted spacer — over the full-width separator rule. The block never
// overflows: a width below the per-dimension thresholds drops the subtitle then
// collapses the wordmark to its compact form (§2.7).
func renderHeaderBlock(width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	band := headerBand(w, mode, colourless)
	rule := headerSeparatorRule(w, mode, colourless)
	return lipgloss.JoinVertical(lipgloss.Left, band, rule)
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

// headerPadRight pads seg (whose rendered width is segWidth) with canvas-painted
// spaces out to exactly w cells, so the band carries the canvas on every cell
// without an edge-bleed island. A segment already at/over w is returned unchanged
// (the band is clamped to w by construction at the call sites).
func headerPadRight(seg string, segWidth, w int, mode theme.Mode, colourless bool) string {
	if segWidth >= w {
		return seg
	}
	pad := headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", w-segWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, seg, pad)
}
