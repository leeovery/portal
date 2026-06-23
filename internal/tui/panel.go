package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
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
//   - pads/insets every row to a uniform innerWidth (contentWidth + 2·helpRowInset)
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
// colourless-aware via the shared helpFrameStyle / helpInsetRow primitives.
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
	innerWidth := contentWidth + 2*helpRowInset // the span of every frame edge.

	// top + bottom borders + one divider per gap between compartments + every row.
	rows := make([]string, 0, totalRows+len(compartments)+1)
	rows = append(rows, helpFrameTop(innerWidth, borderToken, mode, colourless))
	for i, comp := range compartments {
		if i > 0 {
			rows = append(rows, helpFrameDivider(innerWidth, borderToken, mode, colourless))
		}
		for _, r := range comp {
			row := helpInsetRow(r, contentWidth, mode, colourless)
			rows = append(rows, helpFrameContentLine(row, borderToken, mode, colourless))
		}
	}
	rows = append(rows, helpFrameBottom(innerWidth, borderToken, mode, colourless))
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}
