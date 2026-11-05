package tui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

const (
	deleteTitle       = "Delete project?"
	deleteConsequence = "Removes this project from Portal (name, aliases, tags). Your sessions and files are untouched."

	deleteKeyConfirm   = "y"
	deleteLabelConfirm = "delete"
)

func renderDeleteModalContent(name, path string, th theme.Theme, colourless bool) string {
	return renderDestructiveConfirm(deleteModalSpec(name, path, th, colourless), th, colourless)
}

func deleteModalSpec(name, path string, th theme.Theme, colourless bool) destructiveConfirmSpec {
	return destructiveConfirmSpec{
		title:         deleteTitle,
		targetName:    name,
		extraBodyRows: []string{deleteModalPathRow(path, th, colourless)},
		consequence:   deleteConsequence,
		confirmKey:    deleteKeyConfirm,
		confirmLabel:  deleteLabelConfirm,
	}
}

func deleteModalPathRow(path string, th theme.Theme, colourless bool) string {
	visible := ansi.Truncate(path, destructiveBodyWidth, "…")
	return headerStyle(th.TextMuted, th, colourless).Render(visible)
}
