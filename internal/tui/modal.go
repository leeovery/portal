package tui

import (
	"charm.land/lipgloss/v2"
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

// renderModalOnClearedCanvas is the §8.1/13.5 shared blank-screen modal layer:
// when a modal is open the page behind it is CLEARED to the owned canvas (§1) and
// the centred border-defined panel sits on that flat fill — replacing the former
// composite-over-the-live-list mechanic (now removed). It returns ONLY the centred
// panel placed into the inset content region (width × height); the outer fillCanvas
// wrap in View() then paints the owned mode-matched canvas into every surrounding
// cell, so the cleared backdrop is the SAME Phase 1 fill the rest of Portal uses
// (and is suppressed to the terminal native bg under NO_COLOR by the SAME
// fillCanvas → fillColourless path — no second carve-out here).
//
// §14.6 DECISION — ADAPT (not a modal-system rework). The former splice mechanic
// composited the panel OVER the rendered list view, so it structurally could not
// clear-to-canvas: the list rows were its background. Rather than rework an overlay
// engine, the page composers (viewSessionList / viewProjectList) call this instead
// of building the full page when a modal is active — they hand only the
// content-region dims, this centres the panel on a blank region, and the Phase 1
// fillCanvas outer wrap supplies the cleared canvas.
//
// The panel's own styling/copy (the ⚠ header, sectioned layout, red name, footer)
// is a LATER task (3-5); this layer keeps the existing modalStyle box and only
// changes the BACKDROP + CENTRING. lipgloss.Place reuses the existing centre maths
// sized to the content region (the 80×24 fallback is applied by the caller via
// termDims so the region is never zero-sized).
func renderModalOnClearedCanvas(content string, width, height int) string {
	panel := modalStyle.Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}
