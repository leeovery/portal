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

// modalRoom is the most rows a modal takes in a content region height rows
// tall: it keeps clear of the region's first and last rows, where the page's
// header and footer sit, so however short the terminal it reaches neither.
func modalRoom(height int) int {
	return height - 2
}

// placeModal centres the layout's panel fitted to the room a modal has, its
// scrolling rows offset rows down.
func placeModal(layout panelLayout, offset, width, height int, th theme.Theme, colourless bool) string {
	panel := renderFittedPanel(layout, modalRoom(height), offset, th.Border, th, colourless)
	return placeModalOnClearedCanvas(panel, width, height)
}

// The title names the scroll keys only where the body scrolls in the room.
func renderHelpModalOnClearedCanvas(entries []keymapEntry, legend []helpLegendEntry, offset, width, height int, th theme.Theme, colourless bool) string {
	layout := helpModalLayout(entries, legend, false, th, colourless)
	if layout.rows() > modalRoom(height) {
		layout = helpModalLayout(entries, legend, true, th, colourless)
	}
	return placeModal(layout, offset, width, height, th, colourless)
}

func renderKillModalOnClearedCanvas(name string, offersAttach bool, width, height int, th theme.Theme, colourless bool) string {
	return placeModal(destructiveConfirmLayout(killModalSpec(name, offersAttach), th, colourless), 0, width, height, th, colourless)
}

func renderDeleteModalOnClearedCanvas(name, path string, width, height int, th theme.Theme, colourless bool) string {
	return placeModal(destructiveConfirmLayout(deleteModalSpec(name, path, th, colourless), th, colourless), 0, width, height, th, colourless)
}

func renderRenameModalOnClearedCanvas(input textinput.Model, oldName string, width, height int, th theme.Theme, colourless bool) string {
	return placeModal(renameModalLayout(input, oldName, th, colourless), 0, width, height, th, colourless)
}

// The body scrolls to the focused field, so Tab carries the view with it.
func renderEditModalOnClearedCanvas(m Model, width, height int, th theme.Theme, colourless bool) string {
	layout := m.editModalLayout()
	return placeModal(layout, m.editModalOffset(layout.room(modalRoom(height))), width, height, th, colourless)
}
