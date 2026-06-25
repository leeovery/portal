package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// destructive_confirm.go owns the SINGLE destructive-confirm panel grammar shared by
// the §8.3 kill modal and the §8.6 delete-project modal — two modals that render the
// identical "destructive confirm" element: a state.red ▲ <Title> header, a state.red +
// bold target name row, an optional set of extra body rows (the delete modal's project
// path), ONE canvas-painted blank separator, a text.detail consequence word-wrapped at
// the shared body width, and a `y <verb>   esc cancel` footer. Before this file the
// per-compartment render logic was duplicated wholesale across kill_modal.go and the
// near-verbatim delete_modal.go clone; the destructive treatment (▲ glyph, state.red
// role, text.detail consequence colour, body-width 52, footer shape) now lives here in
// exactly one place, so a future change is a single edit and the two modals can never
// silently diverge.
//
// Destructive emphasis is carried by glyph + colour + bold (§2.2/§2.5), never colour
// alone: the ▲ triangle and the title + target name render in state.red AND bold, so
// under the NO_COLOR carve-out the ▲ glyph + bold still mark the action as destructive
// on the terminal's native fg.

const (
	// destructiveTitleGlyph is the destructive ▲ triangle that opens the header —
	// state.red per §2.9 (red is destructive-only). Glyph + colour + bold (§2.2). The
	// single definition shared by the kill and delete modals.
	destructiveTitleGlyph = "▲"
	// destructiveBodyWidth is the word-wrap target for the consequence line (in cells)
	// and the panel's minimum content width, so the panel stays a consistent size
	// regardless of target-name/path length and the consequence wraps to the ~two lines
	// the §8.3/§8.6 references show. The single definition shared by both modals.
	destructiveBodyWidth = 52
	// destructiveKeyCancel / destructiveLabelCancel are the always-present cancel hint —
	// the dismiss key lives in the footer (§8.1) as `esc cancel` for every destructive
	// confirm.
	destructiveKeyCancel   = "esc"
	destructiveLabelCancel = "cancel"
)

// destructiveConfirmSpec captures the per-modal DATA a destructive-confirm panel needs;
// everything else (the ▲ glyph, the state.red/text.detail roles, the separator, the
// body width, the cancel hint) is owned by renderDestructiveConfirm. The kill modal
// supplies a nameTrailer (the `· N window(s)` count rides the name row); the delete
// modal supplies extraBodyRows (the project-path row below the name).
type destructiveConfirmSpec struct {
	// title is the header title text (rendered state.red + bold beside the ▲).
	title string
	// targetName is the destructive target (session / project name), rendered state.red
	// + bold as the body's first row.
	targetName string
	// nameTrailer, when non-empty, is appended to the name row in text.detail after a
	// two-cell canvas gap (the kill modal's `· N window(s)` count) — rendered by
	// destructiveNameRow. Empty for delete.
	nameTrailer string
	// extraBodyRows are already-styled rows inserted below the name row, before the
	// blank separator (the delete modal's project-path row). Empty for kill.
	extraBodyRows []string
	// consequence is the irreversibility / record-only warning, word-wrapped at
	// destructiveBodyWidth and rendered in text.detail.
	consequence string
	// confirmKey / confirmLabel are the footer's confirm hint (e.g. `y` / `kill`); the
	// cancel hint is always `esc cancel`.
	confirmKey   string
	confirmLabel string
}

// renderDestructiveConfirm composes the destructive-confirm panel for the given spec,
// drawing the three compartments through the shared single-tone joined panel:
//
//	header:  ▲ <Title>                 (▲ + title, state.red + bold)
//	body:    <name> [· trailer]         (name state.red+bold, trailer text.detail)
//	         [extra rows]                (e.g. the delete modal's project path)
//	         <blank>                     (the single "what" → "warning" separator)
//	         <consequence …>             (text.detail, word-wrapped at body width)
//	footer:  <key> <verb>   esc cancel   (glyphs accent.blue, labels text.detail)
//
// Vertical spacing is terminal-native FLUSH (the help/kill/delete convention): every
// compartment's content is flush to its dividers; the ONE blank row inside the body is
// the deliberate semantic separator between the target and the warning.
func renderDestructiveConfirm(spec destructiveConfirmSpec, mode theme.Mode, colourless bool) string {
	header := []string{destructiveHeaderRow(spec.title, mode, colourless)}
	body := destructiveBodyRows(spec, mode, colourless)
	footer := []string{destructiveFooterRow(spec.confirmKey, spec.confirmLabel, mode, colourless)}
	return renderJoinedPanel([][]string{header, body, footer}, theme.MV.BorderSeparator, mode, colourless)
}

// destructiveHeaderRow renders `▲ <Title>` — the ▲ glyph and the title text both in
// state.red and bold (glyph + colour + bold, §2.2). Under NO_COLOR the state.red hue
// drops (native fg) but the glyph + bold remain so the destructive signal survives.
func destructiveHeaderRow(title string, mode theme.Mode, colourless bool) string {
	style := headerStyle(theme.MV.StateRed, mode, colourless).Bold(true)
	glyph := style.Render(destructiveTitleGlyph)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	titleSeg := style.Render(title)
	return lipgloss.JoinHorizontal(lipgloss.Top, glyph, gap, titleSeg)
}

// destructiveBodyRows builds the body compartment: the target name row (with an optional
// same-line trailer), any extra body rows, ONE blank separator row, then the
// word-wrapped consequence line(s).
func destructiveBodyRows(spec destructiveConfirmSpec, mode theme.Mode, colourless bool) []string {
	rows := []string{destructiveNameRow(spec.targetName, spec.nameTrailer, mode, colourless)}
	rows = append(rows, spec.extraBodyRows...)
	// The single blank row separating the "what" (target) from the "warning"
	// (consequence) — the body's only blank, canvas-painted so it carries no
	// terminal-bg island.
	rows = append(rows, headerCanvasBg(mode, colourless).Render(""))
	rows = append(rows, destructiveConsequenceRows(spec.consequence, mode, colourless)...)
	return rows
}

// destructiveNameRow renders the target name in state.red + bold (the destructive target
// emphasis). When trailer is non-empty it appends `  <trailer>` in text.detail on the
// same line (the kill modal's `· N window(s)` count); an empty trailer renders the name
// alone (the delete modal's name row).
func destructiveNameRow(name, trailer string, mode theme.Mode, colourless bool) string {
	nameSeg := headerStyle(theme.MV.StateRed, mode, colourless).Bold(true).Render(name)
	if trailer == "" {
		return nameSeg
	}
	gap := headerCanvasBg(mode, colourless).Render("  ")
	trailerSeg := headerStyle(theme.MV.TextDetail, mode, colourless).Render(trailer)
	return lipgloss.JoinHorizontal(lipgloss.Top, nameSeg, gap, trailerSeg)
}

// destructiveConsequenceRows word-wraps the consequence sentence to destructiveBodyWidth
// and renders each wrapped line in text.detail — so the panel grows to the body width
// and the consequence reads across the wrapped lines of the §8.3/§8.6 reference. The
// single word-wrap loop shared by both modals.
func destructiveConsequenceRows(text string, mode theme.Mode, colourless bool) []string {
	wrapped := ansi.Wordwrap(text, destructiveBodyWidth, "")
	style := headerStyle(theme.MV.TextDetail, mode, colourless)
	lines := strings.Split(wrapped, "\n")
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, style.Render(line))
	}
	return rows
}

// destructiveFooterRow renders `<key> <verb>   esc cancel` — the key glyphs in
// accent.blue, the labels in text.detail, via the shared renderConfirmCancelFooter so
// the confirm/cancel footer shape lives in one place. The cancel hint is always
// `esc cancel`.
func destructiveFooterRow(confirmKey, confirmLabel string, mode theme.Mode, colourless bool) string {
	return renderConfirmCancelFooter(confirmKey, confirmLabel, destructiveKeyCancel, destructiveLabelCancel, mode, colourless)
}
