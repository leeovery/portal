package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §8.4 rename-session modal. A reskin (not a rewrite): the rename flow LOGIC
// is unchanged (handled in updateRenameModal / renameAndRefresh); this file owns
// only the MV rendering. It composes through the SAME shared single-tone joined
// panel the help + kill modals use (renderJoinedPanel) — three compartments
// (header / body / footer) separated by two joined ├───┤ dividers, all in
// border.separator.
//
// The body's input is a SEPARATE nested element drawn by the shared renderInputBox
// helper (§13.1) in its ALWAYS-EDITING variant: a thin rounded box whose outline is
// accent.orange over a TRANSPARENT interior (§8.1, no fill). The rename input is
// always focused AND always editing — there is no navigate state — so it carries the
// orange editing treatment (border + live block cursor) and the header shows the
// `◉ EDIT MODE` badge. The value renders in text.primary with an orange block cursor;
// the orange outline + cursor + badge are the editing signal, distinct from the
// panel's border.separator frame. (No fill: a flush fill can't coexist with a thin
// rounded outline in a terminal — see renderInputBox.)

const (
	// renameTitle is the header title text (text.primary), the §8.4 `Rename session`.
	renameTitle = "Rename session"
	// renameFieldLabel is the §8.4/§13.1 field label for the focused input —
	// accent.violet (the focused-field label colour).
	renameFieldLabel = "NEW NAME"
	// renameWasPrefix opens the `was: <old name>` context line (text.detail).
	renameWasPrefix = "was: "

	// renameInputInnerWidth is the input box's inner content width (in cells) — the
	// lipgloss box Width (the span inside the orange side borders, including the box's
	// 1-cell horizontal padding each side). It also anchors the panel's body width so
	// the panel stays a consistent size regardless of value/old-name length, and so an
	// over-long `was:` line truncates to fit rather than stretching the panel. Sized to
	// comfortably hold a `{project}-{nanoid}` name with room to grow.
	renameInputInnerWidth = 44

	// Footer copy + the per-group gap. The ⏎/esc key glyphs render in accent.blue,
	// the rename/cancel labels in text.detail (§8.4). The ⏎ glyph matches the help
	// modal + Projects footer (NOT the legacy ↵).
	renameKeyConfirm   = "⏎"
	renameLabelConfirm = "rename"
	renameKeyCancel    = "esc"
	renameLabelCancel  = "cancel"
)

// renderRenameModalContent composes the §8.4 rename-session modal for the given
// input + old name. Three compartments drawn by the shared joined panel:
//
//	header:  Rename session              ◉ EDIT MODE   (title text.primary, badge accent.orange right-aligned)
//	body:    NEW NAME                     (accent.violet field label)
//	         ╭──────────────────────╮     (orange input-box outline)
//	         │ <value>▌             │     (value text.primary, orange block cursor)
//	         ╰──────────────────────╯
//	         was: <old name>             (text.detail, truncated to fit)
//	footer:  ⏎ rename   esc cancel        (glyphs accent.blue, labels text.detail)
//
// Vertical spacing is terminal-native FLUSH (the help/kill modal convention): every
// body row is flush to its dividers; the input box's three rows are its own outline,
// not blank padding. The input is styled here (value text.primary, orange block
// cursor) — its input SEMANTICS are untouched.
func renderRenameModalContent(input textinput.Model, oldName string, mode theme.Mode, colourless bool) string {
	header := []string{renameModalHeaderRow(mode, colourless)}
	body := renameModalBodyRows(input, oldName, mode, colourless)
	footer := []string{renameModalFooterRow(mode, colourless)}
	return renderJoinedPanel([][]string{header, body, footer}, theme.MV.BorderSeparator, mode, colourless)
}

// renameModalHeaderRow renders `Rename session` (text.primary, the non-destructive
// modal-title colour — §8.1) left-aligned, with the always-on `◉ EDIT MODE` badge
// (accent.orange) right-aligned in the far corner — the rename input is always
// editing, so the badge is always shown (via the shared renderHeaderWithBadge,
// matching the edit modal's right-align technique). The header is pinned to the
// panel content width so the badge sits in the panel's right corner.
func renameModalHeaderRow(mode theme.Mode, colourless bool) string {
	title := headerStyle(theme.MV.TextPrimary, mode, colourless).Bold(true).Render(renameTitle)
	return renderHeaderWithBadge(title, renamePanelContentWidth(), true, mode, colourless)
}

// renamePanelContentWidth returns the rename modal's panel content width — the span
// the header row is padded to so the `◉ EDIT MODE` badge right-aligns to the panel's
// content edge. The input box (renameInputInnerWidth + its two side borders) is the
// widest body element and anchors the panel width (the header title + badge and the
// footer are both narrower), so the header pinned to this width matches the box edge.
func renamePanelContentWidth() int {
	return renameInputInnerWidth + 2
}

// renameModalBodyRows builds the body compartment rows: the NEW NAME label, the
// three-row orange input box (top edge / value+cursor / bottom edge), then the
// truncated `was: <old name>` context line.
func renameModalBodyRows(input textinput.Model, oldName string, mode theme.Mode, colourless bool) []string {
	rows := []string{renameModalLabelRow(mode, colourless)}
	rows = append(rows, renameModalInputBoxRows(input, mode, colourless)...)
	rows = append(rows, renameModalWasRow(oldName, mode, colourless))
	return rows
}

// renameModalLabelRow renders the `NEW NAME` field label in accent.violet (the
// §13.1 focused-field label colour — the input is the live editing element).
func renameModalLabelRow(mode theme.Mode, colourless bool) string {
	return headerStyle(theme.MV.AccentViolet, mode, colourless).Render(renameFieldLabel)
}

// renameModalInputBoxRows renders the border-defined input box through the SHARED
// renderInputBox helper in its always-EDITING variant: a thin ROUNDED outline in
// accent.orange over a TRANSPARENT interior — no fill (§8.1). The value renders in
// text.primary with an orange block cursor; the orange outline + cursor are the
// editing signal (§13.1 — the rename input is always editing, so the outline is
// always accent.orange and the header carries the `◉ EDIT MODE` badge). The
// textinput's own View (with its live cursor) is the content; the box border is the
// shared helper's, not a bespoke one.
//
// Each rendered line is one body row for renderJoinedPanel (which expects single-line
// rows). Under NO_COLOR the hues drop to the native fg and the box structure survives.
func renameModalInputBoxRows(input textinput.Model, mode theme.Mode, colourless bool) []string {
	value := renameInputView(input, mode, colourless)
	return renderInputBox(value, inputBoxEditing, true, renameInputInnerWidth, mode, colourless)
}

// renameInputView styles the textinput to the MV palette (value text.primary, orange
// block cursor, NO fill) and returns its rendered View. The input's SEMANTICS are
// untouched — only its Styles + Prompt change. The inline prompt is cleared so the
// value renders alone inside the box (the §8.4 `NEW NAME` label carries the field
// name, so a textinput prompt would double up). Cursor blink is disabled so the
// captured frame is deterministic (the cursor is always the solid orange block, never
// a blinked-off gap). The cursor is accent.orange to match the box's editing state
// (the §13.1 editing colour). Under the NO_COLOR carve-out every hue drops: the value
// renders on the native fg and the cursor falls back to a bare reverse block.
func renameInputView(input textinput.Model, mode theme.Mode, colourless bool) string {
	input.Prompt = ""
	styles := input.Styles()
	if colourless {
		styles.Focused.Text = lipgloss.NewStyle()
		styles.Cursor.Color = lipgloss.NoColor{}
		styles.Cursor.Blink = false
		input.SetStyles(styles)
		return input.View()
	}
	styles.Focused.Text = lipgloss.NewStyle().Foreground(theme.MV.TextPrimary.ColorFor(mode))
	styles.Cursor.Color = theme.MV.AccentOrange.ColorFor(mode)
	styles.Cursor.Blink = false
	input.SetStyles(styles)
	return input.View()
}

// renameModalWasRow renders the `was: <old name>` context line in text.detail. The
// old name is truncated with an ellipsis to the box's inner width so an over-long
// name never overflows the panel (the §8.4 edge case).
func renameModalWasRow(oldName string, mode theme.Mode, colourless bool) string {
	// The prefix is fixed-width; the name truncates within the remaining budget so the
	// whole line fits the box's inner width.
	nameBudget := max(renameInputInnerWidth-lipgloss.Width(renameWasPrefix), 1)
	name := ansi.Truncate(oldName, nameBudget, "…")
	return headerStyle(theme.MV.TextDetail, mode, colourless).Render(renameWasPrefix + name)
}

// renameModalFooterRow renders `⏎ rename   esc cancel` — the ⏎/esc key glyphs in
// accent.blue, the rename/cancel labels in text.detail (§8.4). The dismiss key
// lives in the footer (§8.1) as `esc cancel`. The ⏎ glyph matches the help modal +
// Projects footer.
func renameModalFooterRow(mode theme.Mode, colourless bool) string {
	return renderConfirmCancelFooter(renameKeyConfirm, renameLabelConfirm, renameKeyCancel, renameLabelCancel, mode, colourless)
}
