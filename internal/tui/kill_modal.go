package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §8.3 kill-confirm modal. A reskin (not a rewrite): the confirm/cancel LOGIC
// is unchanged (handled in updateKillConfirmModal); this file owns only the MV
// rendering. It composes through the SAME shared single-tone joined panel the help
// modal uses (renderJoinedPanel) — three compartments (header / body / footer)
// separated by two joined ├───┤ dividers, all in border.separator.
//
// Destructive emphasis is carried by glyph + colour + bold (§2.2/§2.5), never
// colour alone: the ▲ triangle and the title + name render in state.red AND bold,
// so under the NO_COLOR carve-out the ▲ glyph + bold still mark the action as
// destructive on the terminal's native fg.

const (
	// killTitleGlyph is the destructive ▲ triangle that opens the header — state.red
	// per §2.9 (red is destructive-only). Glyph + colour + bold (§2.2).
	killTitleGlyph = "▲"
	// killTitle is the header title text (state.red), the §8.3 `Kill session?`.
	killTitle = "Kill session?"
	// killConsequence is the §8.3 consequence line — the irreversibility warning,
	// rendered in text.detail and word-wrapped within the panel body width.
	killConsequence = "Ends the tmux session and all its panes. Can't be undone."
	// killBodyWidth is the word-wrap target for the consequence line (in cells). It
	// also anchors the panel's minimum content width so the panel stays a consistent
	// size regardless of session-name length, and so the consequence wraps to the
	// ~two lines the §8.3 reference shows rather than stretching the panel.
	killBodyWidth = 52

	// Footer copy + the per-group gap. The y/esc key glyphs render in accent.blue,
	// the kill/cancel labels in text.detail (§8.3).
	killKeyConfirm   = "y"
	killLabelConfirm = "kill"
	killKeyCancel    = "esc"
	killLabelCancel  = "cancel"
)

// renderKillModalContent composes the §8.3 kill-confirm modal body for the given
// session name + window count. Three compartments drawn by the shared joined panel:
//
//	header:  ▲ Kill session?            (▲ + title, state.red + bold)
//	body:    <name>  · N window(s)      (name state.red+bold, count text.detail)
//	         <blank>                     (the single "what" → "warning" separator)
//	         Ends the tmux session …     (consequence, text.detail, word-wrapped)
//	footer:  y kill   esc cancel         (glyphs accent.blue, labels text.detail)
//
// Vertical spacing is terminal-native FLUSH (§ the help modal's convention): every
// compartment's content is flush to its dividers; the ONE blank row inside the body
// is a deliberate semantic separator (the only blank). The window count pluralises
// (`1 window` / `N windows`, `0 windows` defensively).
func renderKillModalContent(name string, windows int, mode theme.Mode, colourless bool) string {
	header := []string{killModalHeaderRow(mode, colourless)}
	body := killModalBodyRows(name, windows, mode, colourless)
	footer := []string{killModalFooterRow(mode, colourless)}
	return renderJoinedPanel([][]string{header, body, footer}, theme.MV.BorderSeparator, mode, colourless)
}

// killModalHeaderRow renders `▲ Kill session?` — the ▲ glyph and the title text
// both in state.red and bold (glyph + colour + bold, §2.2). Under NO_COLOR the
// state.red hue drops (native fg) but the glyph + bold remain so the destructive
// signal survives.
func killModalHeaderRow(mode theme.Mode, colourless bool) string {
	style := headerStyle(theme.MV.StateRed, mode, colourless).Bold(true)
	glyph := style.Render(killTitleGlyph)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	title := style.Render(killTitle)
	return lipgloss.JoinHorizontal(lipgloss.Top, glyph, gap, title)
}

// killModalBodyRows builds the body compartment rows: the name·count line, ONE
// blank separator row, then the word-wrapped consequence line(s).
func killModalBodyRows(name string, windows int, mode theme.Mode, colourless bool) []string {
	rows := []string{killModalNameRow(name, windows, mode, colourless)}
	// The single blank row separating the "what" (name + count) from the "warning"
	// (consequence) — the body's only blank, canvas-painted so it carries no
	// terminal-bg island.
	rows = append(rows, headerCanvasBg(mode, colourless).Render(""))
	rows = append(rows, killModalConsequenceRows(mode, colourless)...)
	return rows
}

// killModalNameRow renders `<name>  · N window(s)`: the session name in state.red
// + bold, then the bullet + pluralised window count in text.detail, on one line.
func killModalNameRow(name string, windows int, mode theme.Mode, colourless bool) string {
	nameSeg := headerStyle(theme.MV.StateRed, mode, colourless).Bold(true).Render(name)
	gap := headerCanvasBg(mode, colourless).Render("  ")
	count := headerStyle(theme.MV.TextDetail, mode, colourless).Render(killWindowCount(windows))
	return lipgloss.JoinHorizontal(lipgloss.Top, nameSeg, gap, count)
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

// killModalConsequenceRows word-wraps the consequence sentence to killBodyWidth and
// renders each wrapped line in text.detail — so the panel grows to killBodyWidth
// and the consequence reads across the ~two lines of the §8.3 reference.
func killModalConsequenceRows(mode theme.Mode, colourless bool) []string {
	wrapped := ansi.Wordwrap(killConsequence, killBodyWidth, "")
	style := headerStyle(theme.MV.TextDetail, mode, colourless)
	lines := strings.Split(wrapped, "\n")
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, style.Render(line))
	}
	return rows
}

// killModalFooterRow renders `y kill   esc cancel` — the y/esc key glyphs in
// accent.blue, the kill/cancel labels in text.detail (§8.3). The dismiss key lives
// in the footer (§8.1) as `esc cancel`. Routes through the shared
// renderConfirmCancelFooter so the confirm/cancel shape lives in one place.
func killModalFooterRow(mode theme.Mode, colourless bool) string {
	return renderConfirmCancelFooter(killKeyConfirm, killLabelConfirm, killKeyCancel, killLabelCancel, mode, colourless)
}

// killModalKeyHint renders one `<key> <label>` footer group via the shared
// renderKeyHint helper (key glyph accent.blue, single canvas spacer, label text.detail).
func killModalKeyHint(key, label string, mode theme.Mode, colourless bool) string {
	return renderKeyHint(key, label, theme.MV.AccentBlue, mode, colourless)
}
