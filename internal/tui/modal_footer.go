package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

type footerHintGroup struct {
	key   string
	label string
}

// An empty key takes the label-only path — the form the edit footer's
// consequence note collapses onto.
func renderKeyHint(key, label string, keyTok theme.Token, th theme.Theme, colourless bool) string {
	labelSeg := headerStyle(th.TextMuted, th, colourless).Render(label)
	if key == "" {
		return labelSeg
	}
	keySeg := headerStyle(keyTok, th, colourless).Render(key)
	gap := headerCanvasBg(th, colourless).Render(" ")
	return lipgloss.JoinHorizontal(lipgloss.Top, keySeg, gap, labelSeg)
}

func renderBlueKeyHint(key, label string, th theme.Theme, colourless bool) string {
	return renderKeyHint(key, label, th.AccentKey, th, colourless)
}

// A glyph at or past columnWidth takes no pad segment: a canvas style renders the
// empty string as a styled empty run, so padding unconditionally would leave a
// stray escape pair on every full-width row.
func keyColumnRow(glyph, label string, keyStyle, labelStyle lipgloss.Style, columnWidth int, gap string, th theme.Theme, colourless bool) string {
	key := keyStyle.Render(glyph)
	keyWidth := lipgloss.Width(key)
	pad := ""
	if keyWidth < columnWidth {
		pad = headerCanvasBg(th, colourless).Render(spaces(columnWidth - keyWidth))
	}
	gapSeg := headerCanvasBg(th, colourless).Render(gap)
	return lipgloss.JoinHorizontal(lipgloss.Top, key, pad, gapSeg, labelStyle.Render(label))
}

func renderConfirmCancelFooter(confirmKey, confirmLabel, cancelKey, cancelLabel string, th theme.Theme, colourless bool) string {
	return renderModalFooter([]footerHintGroup{{confirmKey, confirmLabel}, {cancelKey, cancelLabel}}, th, colourless)
}

// renderModalFooter lays a modal's key hints out on one row, modalFooterGap
// apart.
func renderModalFooter(groups []footerHintGroup, th theme.Theme, colourless bool) string {
	parts := make([]string, 0, 2*len(groups))
	for i, g := range groups {
		if i > 0 {
			parts = append(parts, headerCanvasBg(th, colourless).Render(modalFooterGap))
		}
		parts = append(parts, renderKeyHint(g.key, g.label, th.AccentKey, th, colourless))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

const modalFooterGap = "   "
