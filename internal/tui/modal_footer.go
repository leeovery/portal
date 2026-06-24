package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// modal_footer.go owns the SINGLE canonical implementation of the footer key-hint
// shape and the confirm/cancel footer row — the §3.4 / §8.x footer contract. Before
// this file the `<key/glyph> <label>` primitive (key glyph in accent.blue, a one-cell
// canvas-painted gap, label in text.detail, joined horizontally) was independently
// re-authored across killModalKeyHint, deleteModalKeyHint, renameModalKeyHint,
// previewFooterHint, editFooterGroup and renderFooterEntry; the three modal footer
// rows each hand-assembled confirm-hint + fixed gap + cancel-hint. They now all route
// through renderKeyHint / renderConfirmCancelFooter so the convention (key-glyph colour
// role, gap width) lives in exactly one place and the modals can never silently drift
// from the footer.

// footerHintGroup is one `<key/glyph> <label>` footer-hint pair — the single shape
// modelling the {Key/Glyph, Label} concept across the contextual edit footer and the
// Preview nav footer. (It replaces the former parallel footerGroup / previewFooterGroup
// value types.) An empty key renders the label alone via renderKeyHint.
type footerHintGroup struct {
	key   string
	label string
}

// renderKeyHint renders one `<key> <label>` footer hint: the key glyph in keyTok over
// the owned canvas, a single canvas-painted gap, then the label in text.detail — joined
// horizontally. It is the ONE place the §3.4 / §8.x footer key-hint shape is authored;
// every modal/footer hint routes through it (callers default keyTok to accent.blue).
//
// An empty key takes the label-only fast path (no glyph, no gap) — the form the edit
// footer's `empty on save = delete` consequence note collapses onto.
func renderKeyHint(key, label string, keyTok theme.Token, mode theme.Mode, colourless bool) string {
	labelSeg := headerStyle(theme.MV.TextDetail, mode, colourless).Render(label)
	if key == "" {
		return labelSeg
	}
	keySeg := headerStyle(keyTok, mode, colourless).Render(key)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	return lipgloss.JoinHorizontal(lipgloss.Top, keySeg, gap, labelSeg)
}

// renderConfirmCancelFooter renders the two-hint modal footer row: the confirm hint,
// the fixed canvas-painted gap (modalFooterGap, "   "), then the cancel hint — both
// hints via renderKeyHint with accent.blue key glyphs. The three modal footer rows
// (kill y/kill·esc/cancel, delete y/delete·esc/cancel, rename ⏎/rename·esc/cancel)
// route through here, passing their per-modal key/label constants as arguments.
func renderConfirmCancelFooter(confirmKey, confirmLabel, cancelKey, cancelLabel string, mode theme.Mode, colourless bool) string {
	confirm := renderKeyHint(confirmKey, confirmLabel, theme.MV.AccentBlue, mode, colourless)
	gap := headerCanvasBg(mode, colourless).Render(modalFooterGap)
	cancel := renderKeyHint(cancelKey, cancelLabel, theme.MV.AccentBlue, mode, colourless)
	return lipgloss.JoinHorizontal(lipgloss.Top, confirm, gap, cancel)
}

// modalFooterGap is the fixed gap between the two key/label groups in a confirm/cancel
// modal footer row (the reference's `y kill   esc cancel` spacing). It replaces the
// per-modal killFooterGap / deleteFooterGap / renameFooterGap constants, which were all
// "   " (three canvas spaces).
const modalFooterGap = "   "
