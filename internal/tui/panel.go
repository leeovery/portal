package tui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

const (
	panelRuleGlyph = "─"
	// The per-row L/R inset for content rows. The divider does not take it — it
	// runs the full inner width so its `├`/`┤` junctions meet both side borders.
	panelRowInset = 2

	panelFrameTopLeft     = "╭"
	panelFrameTopRight    = "╮"
	panelFrameBottomLeft  = "╰"
	panelFrameBottomRight = "╯"
	panelFrameSide        = "│"
	panelFrameTeeLeft     = "├"
	panelFrameTeeRight    = "┤"
)

// Each compartment is a slice of already-styled rows at their natural width;
// dividers are drawn between adjacent compartments. Spacing is flush — a caller
// wanting a blank row passes an empty content row, which is inset and bordered.
func renderJoinedPanel(compartments [][]string, borderToken theme.Token, th theme.Theme, colourless bool) string {
	rows, _ := framedPanelRows(compartments, borderToken, th, colourless)
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// framedPanelRows are the joined panel's rows, its borders and dividers among
// them, and the width inside its side borders.
func framedPanelRows(compartments [][]string, borderToken theme.Token, th theme.Theme, colourless bool) ([]string, int) {
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
	innerWidth := contentWidth + 2*panelRowInset

	rows := make([]string, 0, totalRows+len(compartments)+1)
	rows = append(rows, panelFrameTop(innerWidth, borderToken, th, colourless))
	for i, comp := range compartments {
		if i > 0 {
			rows = append(rows, panelFrameDivider(innerWidth, borderToken, th, colourless))
		}
		for _, r := range comp {
			row := panelInsetRow(r, contentWidth, th, colourless)
			rows = append(rows, panelFrameContentLine(row, borderToken, th, colourless))
		}
	}
	rows = append(rows, panelFrameBottom(innerWidth, borderToken, th, colourless))
	return rows, innerWidth
}

// panelLayout is a joined panel's compartments, and how many at its head and
// foot hold still when it's fitted to fewer rows than it has: the rows of the
// compartments between them, and the dividers among those, scroll.
type panelLayout struct {
	compartments [][]string
	head, foot   int
}

// rows is how many rows the framed panel has.
func (l panelLayout) rows() int {
	n := 2 + len(l.compartments) - 1
	for _, comp := range l.compartments {
		n += len(comp)
	}
	return n
}

// scrolling is the framed row the scrolling rows start at, and how many
// there are.
func (l panelLayout) scrolling() (from, n int) {
	from = 1
	for _, comp := range l.compartments[:l.head] {
		from += len(comp) + 1
	}
	middle := l.compartments[l.head : len(l.compartments)-l.foot]
	n = max(len(middle)-1, 0)
	for _, comp := range middle {
		n += len(comp)
	}
	return from, n
}

// room is how many of the scrolling rows show when the panel is fitted to
// height rows: all of them where it fits, else what the rows that hold still
// leave, one at the least.
func (l panelLayout) room(height int) int {
	_, n := l.scrolling()
	if l.rows() <= height {
		return n
	}
	return min(n, max(height-(l.rows()-n), 1))
}

// maxOffset is the furthest the scrolling rows scroll fitted to height rows.
func (l panelLayout) maxOffset(height int) int {
	_, n := l.scrolling()
	return n - l.room(height)
}

// renderFittedPanel is the layout's panel in no more than height rows, its
// borders always drawn: whole where it fits, else with its scrolling rows
// offset rows down, clamped to their range, and its bottom border signing
// which way rows are hidden. Only where the rows that hold still outgrow
// height does it draw more.
func renderFittedPanel(l panelLayout, height, offset int, borderToken theme.Token, th theme.Theme, colourless bool) string {
	rows, innerWidth := framedPanelRows(l.compartments, borderToken, th, colourless)
	if len(rows) <= height {
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	}
	from, n := l.scrolling()
	room := l.room(height)
	offset = min(max(offset, 0), n-room)
	fitted := slices.Concat(rows[:from], rows[from+offset:from+offset+room], rows[from+n:len(rows)-1])
	fitted = append(fitted, panelFrameBottomSigned(innerWidth, moreSign(offset > 0, offset+room < n), borderToken, th, colourless))
	return lipgloss.JoinVertical(lipgloss.Left, fitted...)
}

const (
	panelMoreAbove = "↑ more"
	panelMoreBelow = "↓ more"
	panelMoreBoth  = "↑↓ more"
)

// moreSign is the sign of which way a fitted panel hides rows, "" where it
// hides none.
func moreSign(above, below bool) string {
	switch {
	case above && below:
		return panelMoreBoth
	case above:
		return panelMoreAbove
	case below:
		return panelMoreBelow
	}
	return ""
}

// panelFrameBottomSigned is the bottom border with the sign set in it, in the
// hint token, ending where the content rows end. A border too narrow to hold
// it, or no sign, draws the plain border.
func panelFrameBottomSigned(w int, sign string, borderToken theme.Token, th theme.Theme, colourless bool) string {
	lead := w - lipgloss.Width(sign) - 1 - panelRowInset
	if sign == "" || lead < 1 {
		return panelFrameBottom(w, borderToken, th, colourless)
	}
	frame := panelFrameStyle(borderToken, th, colourless)
	return lipgloss.JoinHorizontal(lipgloss.Top,
		frame.Render(panelFrameBottomLeft+strings.Repeat(panelRuleGlyph, lead)+" "),
		headerStyle(th.TextMuted, th, colourless).Render(sign),
		frame.Render(" "+strings.Repeat(panelRuleGlyph, panelRowInset-1)+panelFrameBottomRight),
	)
}

// No background — the frame glyphs sit on whatever the placed canvas supplies.
func panelFrameStyle(borderToken theme.Token, th theme.Theme, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(borderToken.Color())
}

func panelFrameTop(w int, borderToken theme.Token, th theme.Theme, colourless bool) string {
	line := panelFrameTopLeft + strings.Repeat(panelRuleGlyph, w) + panelFrameTopRight
	return panelFrameStyle(borderToken, th, colourless).Render(line)
}

func panelFrameBottom(w int, borderToken theme.Token, th theme.Theme, colourless bool) string {
	line := panelFrameBottomLeft + strings.Repeat(panelRuleGlyph, w) + panelFrameBottomRight
	return panelFrameStyle(borderToken, th, colourless).Render(line)
}

func panelFrameDivider(w int, borderToken theme.Token, th theme.Theme, colourless bool) string {
	line := panelFrameTeeLeft + strings.Repeat(panelRuleGlyph, w) + panelFrameTeeRight
	return panelFrameStyle(borderToken, th, colourless).Render(line)
}

func panelFrameContentLine(row string, borderToken theme.Token, th theme.Theme, colourless bool) string {
	side := panelFrameStyle(borderToken, th, colourless).Render(panelFrameSide)
	return lipgloss.JoinHorizontal(lipgloss.Top, side, row, side)
}

// Inset and pad are canvas-painted so the row leaves no terminal-bg island.
func panelInsetRow(row string, contentWidth int, th theme.Theme, colourless bool) string {
	inset := headerCanvasBg(th, colourless).Render(strings.Repeat(" ", panelRowInset))
	padded := headerPadRight(row, lipgloss.Width(row), contentWidth, th, colourless)
	return lipgloss.JoinHorizontal(lipgloss.Top, inset, padded, inset)
}
