package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §8.6 delete-project confirm modal. A reskin (not a rewrite): the
// confirm/cancel LOGIC is unchanged (handled in updateDeleteProjectModal); this
// file owns only the MV rendering. It MIRRORS the §8.3 kill modal's destructive
// treatment and composes through the SAME shared single-tone joined panel
// (renderJoinedPanel) — three compartments (header / body / footer) separated by
// two joined ├───┤ dividers, all in border.separator. No fill (§8.1).
//
// Destructive emphasis is carried by glyph + colour + bold (§2.2/§2.5), never
// colour alone: the ▲ triangle and the title + project name render in state.red AND
// bold, so under the NO_COLOR carve-out the ▲ glyph + bold still mark the action as
// destructive on the terminal's native fg — exactly like the kill modal.
//
// The body's consequence line is DISTINCT from kill's: deleting a project removes
// only the PORTAL RECORD (name, aliases, tags); the sessions and files are
// untouched. This disambiguates a record delete from a session kill (§8.6).

const (
	// deleteTitleGlyph is the destructive ▲ triangle that opens the header — state.red
	// per §2.9 (red is destructive-only). Glyph + colour + bold (§2.2). Same as kill.
	deleteTitleGlyph = "▲"
	// deleteTitle is the header title text (state.red), the §8.6 `Delete project?`.
	deleteTitle = "Delete project?"
	// deleteConsequence is the §8.6 RECORD-ONLY consequence line — distinct from
	// kill's session-ending warning. Rendered in text.detail, word-wrapped within the
	// panel body width.
	deleteConsequence = "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched."
	// deleteBodyWidth is the word-wrap target for the consequence line (in cells). It
	// also anchors the panel's minimum content width so the panel stays a consistent
	// size regardless of project-name/path length, and bounds where the path
	// truncates so an over-long path never stretches the panel.
	deleteBodyWidth = 52

	// Footer copy + the per-group gap. The y/esc key glyphs render in accent.blue,
	// the delete/cancel labels in text.detail (§8.6).
	deleteKeyConfirm   = "y"
	deleteLabelConfirm = "delete"
	deleteKeyCancel    = "esc"
	deleteLabelCancel  = "cancel"
	// deleteFooterGap is the gap between the two footer key/label groups (matches the
	// reference's `y delete   esc cancel` spacing — mirrors kill).
	deleteFooterGap = "   "
)

// renderDeleteModalContent composes the §8.6 delete-project confirm modal body for
// the given project name + path. Three compartments drawn by the shared joined
// panel, mirroring the kill modal:
//
//	header:  ▲ Delete project?          (▲ + title, state.red + bold)
//	body:    <name>                      (project name, state.red + bold)
//	         <path>                       (project path, text.detail)
//	         <blank>                      (the single "what" → "warning" separator)
//	         Removes this project …       (record-only consequence, text.detail, wrapped)
//	footer:  y delete   esc cancel        (glyphs accent.blue, labels text.detail)
//
// Vertical spacing is terminal-native FLUSH (the help/kill modal convention): every
// compartment's content is flush to its dividers; the ONE blank row inside the body
// is the deliberate semantic separator between the target (name + path) and the
// warning (consequence).
func renderDeleteModalContent(name, path string, mode theme.Mode, colourless bool) string {
	header := []string{deleteModalHeaderRow(mode, colourless)}
	body := deleteModalBodyRows(name, path, mode, colourless)
	footer := []string{deleteModalFooterRow(mode, colourless)}
	return renderJoinedPanel([][]string{header, body, footer}, mode, colourless)
}

// deleteModalHeaderRow renders `▲ Delete project?` — the ▲ glyph and the title text
// both in state.red and bold (glyph + colour + bold, §2.2). Under NO_COLOR the
// state.red hue drops (native fg) but the glyph + bold remain so the destructive
// signal survives. Mirrors killModalHeaderRow.
func deleteModalHeaderRow(mode theme.Mode, colourless bool) string {
	style := headerStyle(theme.MV.StateRed, mode, colourless).Bold(true)
	glyph := style.Render(deleteTitleGlyph)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	title := style.Render(deleteTitle)
	return lipgloss.JoinHorizontal(lipgloss.Top, glyph, gap, title)
}

// deleteModalBodyRows builds the body compartment rows: the project name (state.red
// + bold), the path (text.detail, truncated to fit), ONE blank separator row, then
// the word-wrapped record-only consequence line(s).
func deleteModalBodyRows(name, path string, mode theme.Mode, colourless bool) []string {
	rows := []string{deleteModalNameRow(name, mode, colourless)}
	rows = append(rows, deleteModalPathRow(path, mode, colourless))
	// The single blank row separating the "what" (name + path) from the "warning"
	// (consequence) — the body's only blank, canvas-painted so it carries no
	// terminal-bg island. Mirrors the kill modal's body separator.
	rows = append(rows, headerCanvasBg(mode, colourless).Render(""))
	rows = append(rows, deleteModalConsequenceRows(mode, colourless)...)
	return rows
}

// deleteModalNameRow renders the project name in state.red + bold (the destructive
// target emphasis — mirrors the kill modal's session-name row).
func deleteModalNameRow(name string, mode theme.Mode, colourless bool) string {
	return headerStyle(theme.MV.StateRed, mode, colourless).Bold(true).Render(name)
}

// deleteModalPathRow renders the project path in text.detail, truncated with an
// ellipsis to deleteBodyWidth so an over-long path never overflows the panel (the
// §8.6 edge case — mirrors the rename modal's `was:` truncation).
func deleteModalPathRow(path string, mode theme.Mode, colourless bool) string {
	visible := ansi.Truncate(path, deleteBodyWidth, "…")
	return headerStyle(theme.MV.TextDetail, mode, colourless).Render(visible)
}

// deleteModalConsequenceRows word-wraps the record-only consequence sentence to
// deleteBodyWidth and renders each wrapped line in text.detail — so the panel grows
// to deleteBodyWidth and the consequence reads across the wrapped lines. Mirrors
// killModalConsequenceRows but with the distinct record-only copy.
func deleteModalConsequenceRows(mode theme.Mode, colourless bool) []string {
	wrapped := ansi.Wordwrap(deleteConsequence, deleteBodyWidth, "")
	style := headerStyle(theme.MV.TextDetail, mode, colourless)
	lines := strings.Split(wrapped, "\n")
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, style.Render(line))
	}
	return rows
}

// deleteModalFooterRow renders `y delete   esc cancel` — the y/esc key glyphs in
// accent.blue, the delete/cancel labels in text.detail (§8.6). The dismiss key
// lives in the footer (§8.1) as `esc cancel`. Mirrors killModalFooterRow.
func deleteModalFooterRow(mode theme.Mode, colourless bool) string {
	confirm := deleteModalKeyHint(deleteKeyConfirm, deleteLabelConfirm, mode, colourless)
	gap := headerCanvasBg(mode, colourless).Render(deleteFooterGap)
	cancel := deleteModalKeyHint(deleteKeyCancel, deleteLabelCancel, mode, colourless)
	return lipgloss.JoinHorizontal(lipgloss.Top, confirm, gap, cancel)
}

// deleteModalKeyHint renders one `<key> <label>` footer group: the key glyph in
// accent.blue, a single canvas spacer, then the label in text.detail. Mirrors
// killModalKeyHint.
func deleteModalKeyHint(key, label string, mode theme.Mode, colourless bool) string {
	keySeg := headerStyle(theme.MV.AccentBlue, mode, colourless).Render(key)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	labelSeg := headerStyle(theme.MV.TextDetail, mode, colourless).Render(label)
	return lipgloss.JoinHorizontal(lipgloss.Top, keySeg, gap, labelSeg)
}
