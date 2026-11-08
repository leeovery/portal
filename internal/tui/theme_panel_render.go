package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// th never reaches the list's rows — they are drawn by the delegate the model
// assigned; the caller keeps the two in step. p is a value, so the sizing
// lands on this frame's copy and never mutates the model.
func renderThemePanel(p themePanel, height int, th theme.Theme, colourless bool) string {
	inner, bodyRows := themePanelListSize(p, height)
	sizeList(&p.list, inner, bodyRows)

	header := themePanelHeaderShapeFor(height, p.bandRows, p.union.DirUnusable)
	rows := themePanelHeaderBlock(header, p.width, th, colourless)
	rows = appendBlock(rows, themePanelDirRow(p.union.DirUnusable, th, colourless))
	rows = appendBlock(rows, clampBlockHeight(p.list.View(), bodyRows))
	rows = appendBlock(rows, renderThemePanelMessage(p.message, inner, themePanelMessageWraps(p, height), th, colourless))
	rows = appendBlock(rows, renderThemePanelFooter(themePanelKeymap(), inner, th, colourless))

	return themePanelBlock(rows, header.borderFrom(), height, p.width, th, colourless)
}

// The block fills the shape it is handed: page-aligned, the rule and label land
// in the page's rule lane and section-header row; compact, they close up at rows
// 0 and 1.
//
// The rule spans the border column and sits above the label so it shares the
// page's rule lane — do not reorder or notch it: a full-height border makes the
// panel read as a second column rather than a layer. Rule-then-label is
// deliberate and must not be flipped to label-then-rule: with the rule in the
// page's lane, `Themes` lands on the `Sessions` section-header row, which
// is the alignment the panel is meant to keep. The width is the panel's own —
// it never takes the page's fallback.
func themePanelHeaderBlock(shape themePanelHeaderShape, width int, th theme.Theme, colourless bool) []string {
	rows := make([]string, shape.rows)
	rows[shape.ruleRow] = ruleOfWidth(width, th, colourless)
	rows[shape.labelRow] = headerStyle(th.AccentMode, th, colourless).Bold(true).
		Render(themePanelHeaderLabel)
	return rows
}

// Chrome, not a list row — a list row would vanish on page-down. Rows still
// render beneath it, or an unreadable-directory user loses the `●` entirely.
func themePanelDirRow(unusable bool, th theme.Theme, colourless bool) string {
	if !unusable {
		return ""
	}
	return headerStyle(th.AccentAttention, th, colourless).Render(themePanelDirUnreadable)
}

func themePanelDirRowHeight(unusable bool) int {
	return blockHeight(themePanelDirRow(unusable, theme.Theme{}, true))
}

// Left border only — an edged frame would read as an inset dialog, not a
// slide-over. Rows are padded, never truncated: every composed row is authored
// to fit the minimum inner width, below which the panel refuses to open.
func themePanelBlock(rows []string, borderFrom, height, width int, th theme.Theme, colourless bool) string {
	inner := themePanelInnerWidth(width)
	prefix := headerStyle(th.Border, th, colourless).Render(panelFrameSide) +
		headerCanvasBg(th, colourless).Render(spaces(themePanelGutterWidth))
	blank := blankCanvasRow(max(inner, 0), th, colourless)
	painter := newThemePanelPainter(th, colourless)

	out := make([]string, 0, max(height, 0))
	for _, row := range rows {
		if len(out) == height {
			break
		}
		if len(out) < borderFrom {
			out = append(out, painter.paint(themePanelPadRow(row, width, th, colourless)))
			continue
		}
		out = append(out, prefix+painter.paint(themePanelPadRow(row, inner, th, colourless)))
	}
	for len(out) < height {
		out = append(out, prefix+blank)
	}
	return strings.Join(out, "\n")
}

// An empty row becomes a whole canvas blank: lipgloss's horizontal join has no
// defined geometry for a zero-width segment.
func themePanelPadRow(row string, w int, th theme.Theme, colourless bool) string {
	if row == "" {
		return blankCanvasRow(max(w, 0), th, colourless)
	}
	return headerPadRight(row, lipgloss.Width(row), w, th, colourless)
}

// The outer full-terminal fill runs before the panel is composited, so it never
// reaches a panel cell; `bubbles/list` pads with unstyled spaces that would be
// terminal-bg islands. Under NO_COLOR the zero value backfills nothing.
type themePanelPainter struct {
	canvasBg string
	parser   *ansi.Parser
}

func newThemePanelPainter(th theme.Theme, colourless bool) themePanelPainter {
	if colourless {
		return themePanelPainter{}
	}
	return themePanelPainter{
		canvasBg: canvasBgParams(th.Canvas.Color()),
		parser:   ansi.NewParser(),
	}
}

func (p themePanelPainter) paint(row string) string {
	return backfillCanvasBackground(row, p.canvasBg, p.parser)
}

// Composite, never re-lay-out: the base stays at the unreduced width so the
// previewed surface does not reflow. A label cut mid-word is accepted.
func overlayThemePanel(base, panel string, contentW int) string {
	background := lipgloss.NewLayer(base).X(0).Y(0).Z(0)
	foreground := lipgloss.NewLayer(panel).X(max(contentW-lipgloss.Width(panel), 0)).Y(0).Z(1)
	return lipgloss.NewCompositor(background, foreground).Render()
}

// lipgloss.Height("") is 1, so a naive split would give an empty block an
// unbudgeted row.
func appendBlock(rows []string, block string) []string {
	if block == "" {
		return rows
	}
	return append(rows, strings.Split(block, "\n")...)
}

// `bubbles/list` renders a hard minimum of three rows however few it is given;
// uncut, the overshoot comes off the footer. Do not raise themePanelMinBodyRows
// to the list's minimum: that silently redefines the render floor.
func clampBlockHeight(block string, rows int) string {
	if blockHeight(block) <= rows {
		return block
	}
	return strings.Join(strings.Split(block, "\n")[:max(rows, 0)], "\n")
}

// Empty counts zero, not lipgloss.Height's 1, matching appendBlock.
func blockHeight(block string) int {
	if block == "" {
		return 0
	}
	return lipgloss.Height(block)
}
