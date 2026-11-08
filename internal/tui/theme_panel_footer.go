package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

// Fixed rather than derived from the widest glyph handed in, so the key column
// never moves with what the keymap holds.
const themePanelFooterKeyColumnWidth = 3

// Vertical because a horizontal keymap cannot fit the panel's column budget.
// Over-wide rows are returned unpadded, never truncated.
func renderThemePanelFooter(entries []keymapEntry, width int, th theme.Theme, colourless bool) string {
	return lipgloss.JoinVertical(lipgloss.Left, themePanelFooterRows(entries, width, th, colourless)...)
}

// Measured off the rendered block so the reserved rows are the rows that
// render; the block never wraps, so no width or theme is needed.
func themePanelFooterHeight(entries []keymapEntry) int {
	return lipgloss.Height(renderThemePanelFooter(entries, 0, theme.Theme{}, true))
}

// Non-core entries ride the descriptor for dispatch parity only.
func themePanelFooterRows(entries []keymapEntry, width int, th theme.Theme, colourless bool) []string {
	rows := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.Core {
			continue
		}
		rows = append(rows, themePanelFooterRow(e, width, th, colourless))
	}
	return rows
}

func themePanelFooterRow(e keymapEntry, width int, th theme.Theme, colourless bool) string {
	row := keyColumnRow(
		helpKeyGlyph(e), e.Action,
		headerStyle(th.AccentKey, th, colourless),
		headerStyle(th.TextMuted, th, colourless),
		themePanelFooterKeyColumnWidth, footerKeyLabelGap, th, colourless,
	)
	return headerPadRight(row, lipgloss.Width(row), width, th, colourless)
}
