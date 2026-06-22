package tui

import (
	"strings"

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
// The body's input is a SEPARATE nested element: a thin rounded box whose outline is
// accent.violet (the §13.1 focused colour — this single input is always focused) over
// a TRANSPARENT interior (§8.1, no fill). The value renders in text.primary with a
// violet block cursor; the violet outline + cursor are the focus signal, distinct from
// the panel's border.separator frame. (No fill: a flush fill can't coexist with a thin
// rounded outline in a terminal — see renameModalInputBoxRows.)

const (
	// renameTitle is the header title text (text.primary), the §8.4 `Rename session`.
	renameTitle = "Rename session"
	// renameFieldLabel is the §8.4/§13.1 field label for the focused input —
	// accent.violet (the focused-field label colour).
	renameFieldLabel = "NEW NAME"
	// renameWasPrefix opens the `was: <old name>` context line (text.detail).
	renameWasPrefix = "was: "

	// renameInputInnerWidth is the input box's inner content width (in cells) — the
	// lipgloss box Width (the span inside the violet side borders, including the box's
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
	// renameFooterGap is the gap between the two footer key/label groups (matches the
	// reference's `⏎ rename   esc cancel` spacing).
	renameFooterGap = "   "
)

// renderRenameModalContent composes the §8.4 rename-session modal for the given
// input + old name. Three compartments drawn by the shared joined panel:
//
//	header:  Rename session              (text.primary)
//	body:    NEW NAME                     (accent.violet field label)
//	         ╭──────────────────────╮     (violet input-box outline)
//	         │ <value>▌             │     (value text.primary, violet block cursor)
//	         ╰──────────────────────╯
//	         was: <old name>             (text.detail, truncated to fit)
//	footer:  ⏎ rename   esc cancel        (glyphs accent.blue, labels text.detail)
//
// Vertical spacing is terminal-native FLUSH (the help/kill modal convention): every
// body row is flush to its dividers; the input box's three rows are its own outline,
// not blank padding. The input is styled here (value text.primary, violet block
// cursor) — its input SEMANTICS are untouched.
func renderRenameModalContent(input textinput.Model, oldName string, mode theme.Mode, colourless bool) string {
	header := []string{renameModalHeaderRow(mode, colourless)}
	body := renameModalBodyRows(input, oldName, mode, colourless)
	footer := []string{renameModalFooterRow(mode, colourless)}
	return renderJoinedPanel([][]string{header, body, footer}, mode, colourless)
}

// renameModalHeaderRow renders `Rename session` in text.primary (the
// non-destructive modal-title colour — §8.1).
func renameModalHeaderRow(mode theme.Mode, colourless bool) string {
	return headerStyle(theme.MV.TextPrimary, mode, colourless).Render(renameTitle)
}

// renameModalBodyRows builds the body compartment rows: the NEW NAME label, the
// three-row violet input box (top edge / value+cursor / bottom edge), then the
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

// renameModalInputBoxRows renders the border-defined input box: a thin ROUNDED
// outline in accent.violet over a TRANSPARENT interior — no fill (§8.1). The value
// renders in text.primary with a violet block cursor; the violet outline + cursor are
// the focus signal (§13.1 — a single-input modal's input is always focused, so the
// outline is always accent.violet; an unfocused field would be border.separator grey).
//
// No fill, by design: a flush fill cannot coexist with a thin rounded outline in a
// terminal — a border glyph owns a full cell with one background, so any fill either
// leaves a half-cell gap inside the line or bleeds half a cell past it (the rounded
// corner has the same problem). Focus is therefore carried by the outline COLOUR, not
// a fill — consistent with the edit modal's fields (the Paper mock's recessed input
// fill was dropped for this reason; see the spec §13.1 amendment + the dropped
// bg.selection-dimmed token).
//
// Each rendered line is one body row for renderJoinedPanel (which expects single-line
// rows). Under NO_COLOR the hues drop to the native fg and the box structure survives.
func renameModalInputBoxRows(input textinput.Model, mode theme.Mode, colourless bool) []string {
	value := renameInputView(input, mode, colourless)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(renameInputInnerWidth)
	if !colourless {
		style = style.BorderForeground(theme.MV.AccentViolet.ColorFor(mode))
	}
	return strings.Split(style.Render(value), "\n")
}

// renameInputView styles the textinput to the MV palette (value text.primary, violet
// block cursor, NO fill) and returns its rendered View. The input's SEMANTICS are
// untouched — only its Styles + Prompt change. The inline prompt is cleared so the
// value renders alone inside the box (the §8.4 `NEW NAME` label carries the field
// name, so a textinput prompt would double up). Cursor blink is disabled so the
// captured frame is deterministic (the cursor is always the solid violet block, never
// a blinked-off gap). Under the NO_COLOR carve-out every hue drops: the value renders
// on the native fg and the cursor falls back to a bare reverse block.
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
	styles.Cursor.Color = theme.MV.AccentViolet.ColorFor(mode)
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
	nameBudget := renameInputInnerWidth - lipgloss.Width(renameWasPrefix)
	if nameBudget < 1 {
		nameBudget = 1
	}
	name := ansi.Truncate(oldName, nameBudget, "…")
	return headerStyle(theme.MV.TextDetail, mode, colourless).Render(renameWasPrefix + name)
}

// renameModalFooterRow renders `⏎ rename   esc cancel` — the ⏎/esc key glyphs in
// accent.blue, the rename/cancel labels in text.detail (§8.4). The dismiss key
// lives in the footer (§8.1) as `esc cancel`. The ⏎ glyph matches the help modal +
// Projects footer.
func renameModalFooterRow(mode theme.Mode, colourless bool) string {
	confirm := renameModalKeyHint(renameKeyConfirm, renameLabelConfirm, mode, colourless)
	gap := headerCanvasBg(mode, colourless).Render(renameFooterGap)
	cancel := renameModalKeyHint(renameKeyCancel, renameLabelCancel, mode, colourless)
	return lipgloss.JoinHorizontal(lipgloss.Top, confirm, gap, cancel)
}

// renameModalKeyHint renders one `<key> <label>` footer group: the key glyph in
// accent.blue, a single canvas spacer, then the label in text.detail.
func renameModalKeyHint(key, label string, mode theme.Mode, colourless bool) string {
	keySeg := headerStyle(theme.MV.AccentBlue, mode, colourless).Render(key)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	labelSeg := headerStyle(theme.MV.TextDetail, mode, colourless).Render(label)
	return lipgloss.JoinHorizontal(lipgloss.Top, keySeg, gap, labelSeg)
}
