package tui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §8.6 delete-project confirm modal. A reskin (not a rewrite): the
// confirm/cancel LOGIC is unchanged (handled in updateDeleteProjectModal); this file
// owns only the delete modal's DATA. It shares the §8.3 kill modal's destructive
// treatment through the common destructive_confirm.go renderer — this file supplies
// only the delete title / consequence / footer verb plus the project-path extra body
// row (expressed as data, not a forked render path).
//
// The body's consequence line is DISTINCT from kill's: deleting a project removes
// only the PORTAL RECORD (name, aliases, tags); the sessions and files are
// untouched. This disambiguates a record delete from a session kill (§8.6).

const (
	// deleteTitle is the header title text (state.red), the §8.6 `Delete project?`.
	deleteTitle = "Delete project?"
	// deleteConsequence is the §8.6 RECORD-ONLY consequence line — distinct from
	// kill's session-ending warning. Rendered in text.detail, word-wrapped within the
	// panel body width.
	deleteConsequence = "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched."

	// Footer copy. The y/esc key glyphs render in accent.blue, the delete/cancel labels
	// in text.detail (§8.6).
	deleteKeyConfirm   = "y"
	deleteLabelConfirm = "delete"
	deleteKeyCancel    = "esc"
	deleteLabelCancel  = "cancel"
)

// renderDeleteModalContent composes the §8.6 delete-project confirm modal body for
// the given project name + path by supplying the delete DATA to the shared
// destructive-confirm renderer. The project path is passed as an extra body row
// (below the name), not a forked render path.
//
//	header:  ▲ Delete project?          (▲ + title, state.red + bold)
//	body:    <name>                      (project name, state.red + bold)
//	         <path>                       (project path, text.detail)
//	         <blank>                      (the single "what" → "warning" separator)
//	         Removes this project …       (record-only consequence, text.detail, wrapped)
//	footer:  y delete   esc cancel        (glyphs accent.blue, labels text.detail)
func renderDeleteModalContent(name, path string, mode theme.Mode, colourless bool) string {
	spec := destructiveConfirmSpec{
		title:         deleteTitle,
		targetName:    name,
		extraBodyRows: []string{deleteModalPathRow(path, mode, colourless)},
		consequence:   deleteConsequence,
		confirmKey:    deleteKeyConfirm,
		confirmLabel:  deleteLabelConfirm,
	}
	return renderDestructiveConfirm(spec, mode, colourless)
}

// deleteModalPathRow renders the project path in text.detail, truncated with an
// ellipsis to destructiveBodyWidth so an over-long path never overflows the panel (the
// §8.6 edge case — mirrors the rename modal's `was:` truncation). This is the delete
// modal's distinct extra body row.
func deleteModalPathRow(path string, mode theme.Mode, colourless bool) string {
	visible := ansi.Truncate(path, destructiveBodyWidth, "…")
	return headerStyle(theme.MV.TextDetail, mode, colourless).Render(visible)
}

// deleteModalFooterRow renders `y delete   esc cancel` — the y/esc key glyphs in
// accent.blue, the delete/cancel labels in text.detail (§8.6). Routes through the
// shared renderConfirmCancelFooter (mirrors killModalFooterRow).
func deleteModalFooterRow(mode theme.Mode, colourless bool) string {
	return renderConfirmCancelFooter(deleteKeyConfirm, deleteLabelConfirm, deleteKeyCancel, deleteLabelCancel, mode, colourless)
}

// deleteModalKeyHint renders one `<key> <label>` footer group via the shared
// renderKeyHint helper (key glyph accent.blue, single canvas spacer, label text.detail).
func deleteModalKeyHint(key, label string, mode theme.Mode, colourless bool) string {
	return renderKeyHint(key, label, theme.MV.AccentBlue, mode, colourless)
}
