package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

type modalState int

const (
	modalNone modalState = iota
	modalKillConfirm
	modalRename
	modalDeleteProject
	modalEditProject
	modalHelp
)

// Centring only — the outer fillCanvas wrap in View() paints the backdrop.
func placeModalOnClearedCanvas(panel string, width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

func renderHelpModalOnClearedCanvas(entries []keymapEntry, legend []helpLegendEntry, width, height int, th theme.Theme, colourless bool) string {
	panel := renderHelpModalContent(entries, th, colourless, legend)
	return placeModalOnClearedCanvas(panel, width, height)
}

func renderKillModalOnClearedCanvas(name string, windows int, width, height int, th theme.Theme, colourless bool) string {
	panel := renderKillModalContent(name, windows, th, colourless)
	return placeModalOnClearedCanvas(panel, width, height)
}

func renderDeleteModalOnClearedCanvas(name, path string, width, height int, th theme.Theme, colourless bool) string {
	panel := renderDeleteModalContent(name, path, th, colourless)
	return placeModalOnClearedCanvas(panel, width, height)
}

func renderRenameModalOnClearedCanvas(input textinput.Model, oldName string, width, height int, th theme.Theme, colourless bool) string {
	panel := renderRenameModalContent(input, oldName, th, colourless)
	return placeModalOnClearedCanvas(panel, width, height)
}

func renderEditModalOnClearedCanvas(m Model, width, height int, th theme.Theme, colourless bool) string {
	panel := m.renderEditProjectContent()
	return placeModalOnClearedCanvas(panel, width, height)
}
