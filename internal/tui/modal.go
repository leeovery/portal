package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// modalState tracks which modal overlay is currently active.
type modalState int

const (
	modalNone        modalState = iota
	modalKillConfirm            // Kill session confirmation
	modalRename                 // Rename session with textinput
)

// modalStyle defines the bordered box style for modal overlays.
var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(1, 2)

// renderModal overlays styled modal content centered on top of the list view.
// The list view remains visible behind the modal overlay.
func renderModal(content string, listView string, width, height int) string {
	styledModal := modalStyle.Render(content)
	overlayX := max(0, (width-lipgloss.Width(styledModal))/2)
	overlayY := max(0, (height-lipgloss.Height(styledModal))/2)
	return placeOverlay(overlayX, overlayY, styledModal, listView)
}

// placeOverlay composites the foreground string on top of the background string
// at the given x, y position. Characters in the foreground replace characters
// in the background, leaving the rest of the background visible.
func placeOverlay(x, y int, fg, bg string) string {
	fgLines := strings.Split(fg, "\n")
	bgLines := strings.Split(bg, "\n")

	// Ensure background has enough lines
	for len(bgLines) < y+len(fgLines) {
		bgLines = append(bgLines, "")
	}

	for i, fgLine := range fgLines {
		bgIdx := y + i
		if bgIdx < 0 || bgIdx >= len(bgLines) {
			continue
		}

		bgLine := []rune(bgLines[bgIdx])
		fgRunes := []rune(fgLine)

		// Extend background line with spaces if needed
		for len(bgLine) < x+len(fgRunes) {
			bgLine = append(bgLine, ' ')
		}

		// Overwrite background runes with foreground runes
		copy(bgLine[x:x+len(fgRunes)], fgRunes)

		bgLines[bgIdx] = string(bgLine)
	}

	return strings.Join(bgLines, "\n")
}
