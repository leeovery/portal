package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

const (
	// panelRuleGlyph is the horizontal box-drawing glyph for the divider AND every
	// frame edge run (top / bottom border + the divider). Single-tone: it renders in
	// border.separator everywhere (the 2-tone footer leg was dropped).
	panelRuleGlyph = "─"
	// panelRowInset is the per-row L/R inset (in cells) the header text and body rows
	// carry inside the hand-drawn frame. It matches the reference's ~22px
	// paddingInline. The divider does NOT carry this inset — it runs the full inner
	// width (W) so its `├`/`┤` junctions meet both side borders.
	panelRowInset = 2

	// The hand-drawn frame glyphs (a rounded box with real header-divider junctions).
	// EVERY one renders in border.separator (single-tone). The top/bottom corners and
	// the divider tees join the side `│` runs into one continuous frame.
	panelFrameTopLeft     = "╭"
	panelFrameTopRight    = "╮"
	panelFrameBottomLeft  = "╰"
	panelFrameBottomRight = "╯"
	panelFrameSide        = "│"
	panelFrameTeeLeft     = "├"
	panelFrameTeeRight    = "┤"
)

// renderJoinedPanel is the shared single-tone, hand-drawn, border-joined panel —
// the chrome the help modal (§8.5), the kill modal (§8.3), AND the §9.1 full-screen
// preview overlay all compose through. It draws a rounded box (╭─╮ / │…│ / ╰─╯)
// whose EVERY glyph — corners, sides, and the compartment dividers — renders in the
// caller-supplied borderToken (single-tone: the 2-tone footer leg of §8.1 was
// dropped in task 3-4), with the dividers joined to the side borders via real ├/┤
// junctions. The modals pass theme.MV.BorderSeparator (grey); the preview passes
// theme.MV.AccentCyan (the "peek mode" hue, §9.1).
//
// Input is a list of compartments, each a slice of already-styled content rows (at
// their natural width). The helper:
//
//   - measures the widest row across ALL compartments → contentWidth,
//   - pads/insets every row to a uniform innerWidth (contentWidth + 2·panelRowInset)
//     and wraps it in the │ side borders,
//   - interleaves a joined ├───┤ divider between EACH adjacent pair of compartments
//     (so N compartments → N-1 dividers), the divider spanning the full innerWidth
//     so its tees meet both sides,
//   - brackets the whole stack with the ╭─╮ top and ╰─╯ bottom borders.
//
// The vertical spacing is FLUSH (terminal-native, diverging from the Paper px
// padding): no blank frame rows are added — any blank rows a caller wants inside a
// compartment are passed as empty content rows, so they are inset-and-bordered
// like every other row (not bare). Every assembled line is exactly innerWidth+2
// cells, so the frame columns align top to bottom. The frame is mode- and
// colourless-aware via the shared panelFrameStyle / panelInsetRow primitives.
func renderJoinedPanel(compartments [][]string, borderToken theme.Token, mode theme.Mode, colourless bool) string {
	contentWidth := 0
	totalRows := 0
	for _, comp := range compartments {
		totalRows += len(comp)
		for _, r := range comp {
			if w := lipgloss.Width(r); w > contentWidth {
				contentWidth = w
			}
		}
	}
	innerWidth := contentWidth + 2*panelRowInset // the span of every frame edge.

	// top + bottom borders + one divider per gap between compartments + every row.
	rows := make([]string, 0, totalRows+len(compartments)+1)
	rows = append(rows, panelFrameTop(innerWidth, borderToken, mode, colourless))
	for i, comp := range compartments {
		if i > 0 {
			rows = append(rows, panelFrameDivider(innerWidth, borderToken, mode, colourless))
		}
		for _, r := range comp {
			row := panelInsetRow(r, contentWidth, mode, colourless)
			rows = append(rows, panelFrameContentLine(row, borderToken, mode, colourless))
		}
	}
	rows = append(rows, panelFrameBottom(innerWidth, borderToken, mode, colourless))
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// panelFrameStyle returns the single-tone frame paint: the borderToken foreground
// for the mode, or a bare style (native fg) under the NO_COLOR carve-out — so the
// frame glyphs survive colourless but carry no hue. NO background is set (the frame
// glyphs sit on whatever the placed canvas supplies). The modals pass
// theme.MV.BorderSeparator; the §9.1 preview passes theme.MV.AccentCyan.
func panelFrameStyle(borderToken theme.Token, mode theme.Mode, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(borderToken.ColorFor(mode))
}

// panelFrameTop renders the top border line: `╭` + `─`×w + `╮`, all in borderToken.
func panelFrameTop(w int, borderToken theme.Token, mode theme.Mode, colourless bool) string {
	line := panelFrameTopLeft + strings.Repeat(panelRuleGlyph, w) + panelFrameTopRight
	return panelFrameStyle(borderToken, mode, colourless).Render(line)
}

// panelFrameBottom renders the bottom border line: `╰` + `─`×w + `╯`.
func panelFrameBottom(w int, borderToken theme.Token, mode theme.Mode, colourless bool) string {
	line := panelFrameBottomLeft + strings.Repeat(panelRuleGlyph, w) + panelFrameBottomRight
	return panelFrameStyle(borderToken, mode, colourless).Render(line)
}

// panelFrameDivider renders the joined compartment divider: `├` + `─`×w + `┤`, all
// in borderToken (single-tone). The `├`/`┤` tees visibly join the side borders.
func panelFrameDivider(w int, borderToken theme.Token, mode theme.Mode, colourless bool) string {
	line := panelFrameTeeLeft + strings.Repeat(panelRuleGlyph, w) + panelFrameTeeRight
	return panelFrameStyle(borderToken, mode, colourless).Render(line)
}

// panelFrameContentLine wraps a content row (already exactly w cells wide) with the
// left/right `│` side borders (in borderToken), yielding a w+2 cell frame line.
func panelFrameContentLine(row string, borderToken theme.Token, mode theme.Mode, colourless bool) string {
	side := panelFrameStyle(borderToken, mode, colourless).Render(panelFrameSide)
	return lipgloss.JoinHorizontal(lipgloss.Top, side, row, side)
}

// panelInsetRow wraps a content row (whose natural width is at most contentWidth)
// with the per-row L/R canvas inset (panelRowInset cells each side) and pads the
// content out to contentWidth, so every header/body row is exactly innerWidth cells
// (contentWidth + 2·panelRowInset) — the divider's width. The inset and pad are canvas-
// painted so the row carries the owned canvas with no terminal-bg island.
func panelInsetRow(row string, contentWidth int, mode theme.Mode, colourless bool) string {
	inset := headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", panelRowInset))
	padded := headerPadRight(row, lipgloss.Width(row), contentWidth, mode, colourless)
	return lipgloss.JoinHorizontal(lipgloss.Top, inset, padded, inset)
}
