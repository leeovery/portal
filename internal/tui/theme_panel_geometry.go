package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

const (
	// Fixed rather than content-driven, so the panel does not resize as the
	// theme library changes.
	themePanelPreferredWidth = 30
	themePanelMinWidth       = 24

	// Twice the preferred width, so at least half the previewed page stays
	// visible; two stages only, so a resize does not re-lay-out per column.
	themePanelPreferredAffordance = 2 * themePanelPreferredWidth

	themePanelBorderWidth = 1

	// Charged once, in themePanelBlock.
	themePanelGutterWidth = 1

	themePanelMinBodyRows = 1

	// Counted even though the slot is unreserved when empty: the floor would be
	// one row short the moment a message appears.
	themePanelFloorMessageRows = 1
)

type themePanelHeaderShape struct {
	ruleRow  int
	labelRow int
	rows     int
}

// Below the rule in either shape — the rule runs through the border's column.
func (s themePanelHeaderShape) borderFrom() int {
	return s.ruleRow + 1
}

const themePanelCompactHeaderRows = 2

func themePanelCompactHeaderShape() themePanelHeaderShape {
	return themePanelHeaderShape{ruleRow: 0, labelRow: 1, rows: themePanelCompactHeaderRows}
}

// Measured off the page's renderers, never literals, so the panel tracks page
// chrome changes. Zero-width colourless measurement is safe: row counts are
// functions of content only. bandRows is the page's notice-band slot, which
// composes between the header block and the list: it pushes the section-header
// row — and every list row under it — down by its own height, and the label and
// body must travel with them or the two columns run different rhythms. The rule
// is unaffected: the band sits below it.
func themePanelPageAlignedHeaderShape(bandRows int) themePanelHeaderShape {
	label := lipgloss.Height(renderHeaderBlock(0, theme.Theme{}, true)) + bandRows
	return themePanelHeaderShape{
		ruleRow:  lipgloss.Height(headerBand(0, theme.Theme{}, true)),
		labelRow: label,
		rows:     label + sectionHeaderBlockRows(),
	}
}

// Decides the header shape; renderers must not re-decide it. Charging the blank
// alignment rows to the floor would refuse terminals that can render the panel.
// A band's rows are charged the same way, so a banded page short enough to fail
// the page-aligned floor drops to the compact shape rather than overflowing.
func themePanelHeaderShapeFor(height, bandRows int, dirUnusable bool) themePanelHeaderShape {
	pageAligned := themePanelPageAlignedHeaderShape(bandRows)
	if height >= themePanelFloorFor(pageAligned.rows, dirUnusable) {
		return pageAligned
	}
	return themePanelCompactHeaderShape()
}

func themePanelHeaderRows(height, bandRows int, dirUnusable bool) int {
	return themePanelHeaderShapeFor(height, bandRows, dirUnusable).rows
}

// ThemePanelMinWidthTerminal reports the widest terminal whose content region
// steps the panel down to its minimum width — one column below the preferred
// affordance, plus the gutter the content region is inset by on both sides.
func ThemePanelMinWidthTerminal() int {
	return themePanelPreferredAffordance - 1 + 2*Hinset
}

// ThemePanelFloorTerminalHeight reports the shortest terminal the panel opens
// in: the height floor over a usable directory, plus the gutter on both sides.
// One row less refuses.
func ThemePanelFloorTerminalHeight() int {
	return themePanelMinHeight(false) + 2*Vinset
}

type themePanelDim int

const (
	dimNone themePanelDim = iota
	dimWidth
	dimHeight
)

// w is clamped on the refusing path too: the width callers in theme_panel.go
// ignore ok (the floor already refused), and an unclamped w would render a
// sub-minimum panel.
func themePanelWidthFor(contentW int) (w int, ok bool) {
	if contentW >= themePanelPreferredAffordance {
		return themePanelPreferredWidth, true
	}
	return themePanelMinWidth, contentW >= themePanelMinWidth
}

// Compact header cost, never page-aligned — its blank padding rows would refuse a
// renderable panel.
func themePanelMinHeight(dirUnusable bool) int {
	return themePanelFloorFor(themePanelCompactHeaderRows, dirUnusable)
}

func themePanelFloorFor(headerRows int, dirUnusable bool) int {
	return themePanelChromeRows(headerRows, dirUnusable, themePanelFloorMessageRows) + themePanelMinBodyRows
}

// The floor and the body budget share this chrome set, so a new component cannot
// reach one arithmetic and miss the other.
func themePanelChromeRows(headerRows int, dirUnusable bool, messageRows int) int {
	return headerRows +
		themePanelDirRowHeight(dirUnusable) +
		messageRows +
		themePanelFooterHeight(themePanelKeymap())
}

// Entry and resize must consume this answer, not derive their own arithmetic —
// passing one and failing the other opens a broken frame or refuses a fitting
// panel. Width first, so failing both reports narrow.
func themePanelFloor(contentW, contentH int, dirUnusable bool) (dim themePanelDim, ok bool) {
	if _, wide := themePanelWidthFor(contentW); !wide {
		return dimWidth, false
	}
	if contentH < themePanelMinHeight(dirUnusable) {
		return dimHeight, false
	}
	return dimNone, true
}

// Border and gutter are charged exactly once, here.
func themePanelInnerWidth(width int) int {
	return max(width-themePanelBorderWidth-themePanelGutterWidth, 0)
}

// The remainder is not trusted: `bubbles/list` renders a hard minimum of three
// rows however few it is given, so renderThemePanel clamps the body.
func themePanelListSize(p themePanel, height int) (width, rows int) {
	inner := themePanelInnerWidth(p.width)
	reserved := themePanelChromeRows(
		themePanelHeaderRows(height, p.bandRows, p.union.DirUnusable),
		p.union.DirUnusable,
		themePanelMessageHeight(p.message),
	)
	return inner, max(height-reserved, themePanelMinBodyRows)
}
