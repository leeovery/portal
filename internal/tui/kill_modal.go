package tui

import (
	"fmt"

	"github.com/leeovery/portal/internal/tui/theme"
)

// The §8.3 kill-confirm modal. A reskin (not a rewrite): the confirm/cancel LOGIC
// is unchanged (handled in updateKillConfirmModal); this file owns only the kill
// modal's DATA. The destructive-confirm panel grammar (the state.red ▲ <Title>
// header, the state.red+bold target name row, the canvas blank separator, the
// text.detail consequence word-wrapped at body-width 52, and the y <verb> · esc
// cancel footer) lives once in destructive_confirm.go; this file supplies only the
// kill title / consequence / window-count / footer verb and calls the shared renderer.

const (
	// killTitle is the header title text (state.red), the §8.3 `Kill session?`.
	killTitle = "Kill session?"
	// killConsequence is the §8.3 consequence line — the irreversibility warning,
	// rendered in text.detail and word-wrapped within the panel body width.
	killConsequence = "Ends the tmux session and all its panes. Can't be undone."

	// Footer copy. The y/esc key glyphs render in accent.blue, the kill/cancel labels
	// in text.detail (§8.3).
	killKeyConfirm   = "y"
	killLabelConfirm = "kill"
	killKeyCancel    = "esc"
	killLabelCancel  = "cancel"
)

// renderKillModalContent composes the §8.3 kill-confirm modal body for the given
// session name + window count by supplying the kill DATA to the shared
// destructive-confirm renderer. The window count rides the name row via nameTrailer
// (`<name>  · N window(s)`); kill has no extra body rows.
//
//	header:  ▲ Kill session?            (▲ + title, state.red + bold)
//	body:    <name>  · N window(s)      (name state.red+bold, count text.detail)
//	         <blank>                     (the single "what" → "warning" separator)
//	         Ends the tmux session …     (consequence, text.detail, word-wrapped)
//	footer:  y kill   esc cancel         (glyphs accent.blue, labels text.detail)
func renderKillModalContent(name string, windows int, mode theme.Mode, colourless bool) string {
	spec := destructiveConfirmSpec{
		title:        killTitle,
		targetName:   name,
		nameTrailer:  killWindowCount(windows),
		consequence:  killConsequence,
		confirmKey:   killKeyConfirm,
		confirmLabel: killLabelConfirm,
	}
	return renderDestructiveConfirm(spec, mode, colourless)
}

// killWindowCount returns the `· N window(s)` count fragment with correct
// pluralisation (singular only for exactly 1; 0 and N>1 plural).
func killWindowCount(windows int) string {
	unit := "windows"
	if windows == 1 {
		unit = "window"
	}
	return fmt.Sprintf("· %d %s", windows, unit)
}

// killModalFooterRow renders `y kill   esc cancel` — the y/esc key glyphs in
// accent.blue, the kill/cancel labels in text.detail (§8.3). Routes through the shared
// renderConfirmCancelFooter so the confirm/cancel shape lives in one place.
func killModalFooterRow(mode theme.Mode, colourless bool) string {
	return renderConfirmCancelFooter(killKeyConfirm, killLabelConfirm, killKeyCancel, killLabelCancel, mode, colourless)
}

// killModalKeyHint renders one `<key> <label>` footer group via the shared
// renderKeyHint helper (key glyph accent.blue, single canvas spacer, label text.detail).
func killModalKeyHint(key, label string, mode theme.Mode, colourless bool) string {
	return renderKeyHint(key, label, theme.MV.AccentBlue, mode, colourless)
}
