package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// modalState tracks which modal overlay is currently active.
type modalState int

const (
	modalNone          modalState = iota
	modalKillConfirm              // Kill session confirmation
	modalRename                   // Rename session with textinput
	modalDeleteProject            // Delete project confirmation
	modalEditProject              // Edit project with name and alias editing
)

// modalStyle defines the bordered box style for modal overlays.
var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	Padding(1, 2)

// renderModal overlays styled modal content centered on top of the list view.
// The list view remains visible behind the modal overlay. Uses ANSI-aware width
// measurement so that escape sequences in the background do not shift the overlay.
func renderModal(content string, listView string, width, height int) string {
	styledModal := modalStyle.Render(content)
	overlayX := max(0, (width-lipgloss.Width(styledModal))/2)
	overlayY := max(0, (height-lipgloss.Height(styledModal))/2)

	fgLines := strings.Split(styledModal, "\n")
	bgLines := strings.Split(listView, "\n")

	// Ensure background has enough lines
	for len(bgLines) < overlayY+len(fgLines) {
		bgLines = append(bgLines, "")
	}

	for i, fgLine := range fgLines {
		bgIdx := overlayY + i
		if bgIdx < 0 || bgIdx >= len(bgLines) {
			continue
		}

		bgLine := bgLines[bgIdx]
		fgWidth := ansi.StringWidth(fgLine)
		bgWidth := ansi.StringWidth(bgLine)

		// Pad background with spaces if it's too narrow
		if bgWidth < overlayX+fgWidth {
			bgLine += strings.Repeat(" ", overlayX+fgWidth-bgWidth)
		}

		// Composite: left bg + foreground + right bg (all ANSI-aware)
		left := ansi.Truncate(bgLine, overlayX, "")
		right := ansi.TruncateLeft(bgLine, overlayX+fgWidth, "")
		bgLines[bgIdx] = left + fgLine + right
	}

	return strings.Join(bgLines, "\n")
}
