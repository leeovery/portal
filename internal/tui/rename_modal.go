package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

const (
	renameTitle      = "Rename session"
	renameFieldLabel = "NEW NAME"
	renameWasPrefix  = "was: "

	// Anchors the panel's body width so the panel size is stable regardless of
	// value/old-name length.
	renameInputInnerWidth = 44

	renameKeyConfirm   = "⏎"
	renameLabelConfirm = "rename"
	renameKeyCancel    = "esc"
	renameLabelCancel  = "cancel"
)

func renderRenameModalContent(input textinput.Model, oldName string, th theme.Theme, colourless bool) string {
	header := []string{renameModalHeaderRow(th, colourless)}
	body := renameModalBodyRows(input, oldName, th, colourless)
	footer := []string{renameModalFooterRow(th, colourless)}
	return renderJoinedPanel([][]string{header, body, footer}, th.Border, th, colourless)
}

// Always editing — there is no navigate state — so the badge always shows.
func renameModalHeaderRow(th theme.Theme, colourless bool) string {
	title := headerStyle(th.TextPrimary, th, colourless).Bold(true).Render(renameTitle)
	return renderHeaderWithBadge(title, renamePanelContentWidth(), true, editModeIndicator, th, colourless)
}

// The input box is the widest body element and anchors the panel width, so the
// header pinned to this width matches the box edge.
func renamePanelContentWidth() int {
	return renameInputInnerWidth + 2
}

func renameModalBodyRows(input textinput.Model, oldName string, th theme.Theme, colourless bool) []string {
	rows := []string{renameModalLabelRow(th, colourless)}
	rows = append(rows, renameModalInputBoxRows(input, th, colourless)...)
	rows = append(rows, renameModalWasRow(oldName, th, colourless))
	return rows
}

func renameModalLabelRow(th theme.Theme, colourless bool) string {
	return headerStyle(th.AccentPrimary, th, colourless).Render(renameFieldLabel)
}

func renameModalInputBoxRows(input textinput.Model, th theme.Theme, colourless bool) []string {
	value := renameInputView(input, th, colourless)
	return renderInputBox(value, inputBoxEditing, true, renameInputInnerWidth, th, colourless)
}

// Mutates only Prompt and Styles on the value copy. Blink is off so a captured
// frame is deterministic.
func renameInputView(input textinput.Model, th theme.Theme, colourless bool) string {
	input.Prompt = ""
	styles := input.Styles()
	if colourless {
		styles.Focused.Text = lipgloss.NewStyle()
		styles.Cursor.Color = lipgloss.NoColor{}
		styles.Cursor.Blink = false
		input.SetStyles(styles)
		return input.View()
	}
	styles.Focused.Text = lipgloss.NewStyle().Foreground(th.TextPrimary.Color())
	styles.Cursor.Color = th.AccentAttention.Color()
	styles.Cursor.Blink = false
	input.SetStyles(styles)
	return input.View()
}

// Truncated within the budget left by the prefix so a long name never overflows.
func renameModalWasRow(oldName string, th theme.Theme, colourless bool) string {
	nameBudget := max(renameInputInnerWidth-lipgloss.Width(renameWasPrefix), 1)
	name := ansi.Truncate(oldName, nameBudget, "…")
	return headerStyle(th.TextMuted, th, colourless).Render(renameWasPrefix + name)
}

func renameModalFooterRow(th theme.Theme, colourless bool) string {
	return renderConfirmCancelFooter(renameKeyConfirm, renameLabelConfirm, renameKeyCancel, renameLabelCancel, th, colourless)
}
