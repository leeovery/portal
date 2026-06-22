package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// modalState tracks which modal overlay is currently active.
type modalState int

const (
	modalNone          modalState = iota
	modalKillConfirm              // Kill session confirmation
	modalRename                   // Rename session with textinput
	modalDeleteProject            // Delete project confirmation
	modalEditProject              // Edit project with name and alias editing
	modalHelp                     // §8.5 per-page ? help (descriptor-driven keymap reference)
)

// modalBorderStyle is the shared border-defined panel style for the NON-help
// modals (kill / rename / delete / edit): a rounded border whose colour is
// border.separator per §8.1 (the OUTER panel frame is the 2-tone border's
// separator leg), mode-aware, with the existing Padding(1,2) preserved. Under the
// NO_COLOR carve-out (colourless) the rounded glyphs stay but NO border foreground
// is set, so the frame renders on the terminal's native fg (matching every other
// colourless surface). It replaces the former static modalStyle, whose missing
// BorderForeground fell back to terminal-default white in BOTH modes (the FIX 3
// bug). The help modal does NOT use this — it HAND-DRAWS its own frame in
// renderHelpModalContent so its header divider joins the side borders via real
// `├`/`┤` junctions.
func modalBorderStyle(mode theme.Mode, colourless bool) lipgloss.Style {
	s := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)
	if !colourless {
		s = s.BorderForeground(theme.MV.BorderSeparator.ColorFor(mode))
	}
	return s
}

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
// FIX 3 threads the resolved mode + colourless flag through so the panel frame is
// drawn in border.separator (§8.1) instead of the former terminal-default white.
// The panel's own styling/copy (the ⚠ header, sectioned layout, red name, footer)
// is a LATER task (3-5); this layer keeps the existing Padding(1,2) box and only
// fixes the BORDER COLOUR (and the earlier BACKDROP + CENTRING). lipgloss.Place
// reuses the existing centre maths sized to the content region (the 80×24 fallback
// is applied by the caller via termDims so the region is never zero-sized).
func renderModalOnClearedCanvas(content string, width, height int, mode theme.Mode, colourless bool) string {
	panel := modalBorderStyle(mode, colourless).Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

// renderHelpModalOnClearedCanvas composes the §8.5 help modal panel and centres it
// on the cleared owned canvas, exactly like renderModalOnClearedCanvas but with the
// help modal's OWN fully HAND-DRAWN panel: renderHelpModalContent assembles the
// whole bordered frame itself (top/bottom borders, side `│`, the joined `├`/`┤`
// divider, and the flush content rows) — there is NO lipgloss auto-border wrap, so
// the divider can carry real junctions into the side frame and the vertical spacing
// is flush (zero blank rows). The frame is SINGLE-TONE border.separator, mode- and
// colourless-aware. This leaves the shared modalBorderStyle's Padding(1,2) intact
// for the OTHER modals. The same lipgloss.Place centres it on the inset region.
func renderHelpModalOnClearedCanvas(entries []keymapEntry, width, height int, mode theme.Mode, colourless bool) string {
	panel := renderHelpModalContent(entries, mode, colourless)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

// renderKillModalOnClearedCanvas composes the §8.3 kill-confirm modal panel and
// centres it on the cleared owned canvas, exactly like renderHelpModalOnClearedCanvas
// but with the kill modal's own hand-drawn single-tone joined panel
// (renderKillModalContent — the SAME frame the help modal uses, three compartments
// instead of two). The shared modalBorderStyle's Padding(1,2) is left intact for the
// OTHER (not-yet-reskinned) modals; this path bypasses it.
func renderKillModalOnClearedCanvas(name string, windows int, width, height int, mode theme.Mode, colourless bool) string {
	panel := renderKillModalContent(name, windows, mode, colourless)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

// renderDeleteModalOnClearedCanvas composes the §8.6 delete-project confirm modal
// panel and centres it on the cleared owned canvas, exactly like
// renderKillModalOnClearedCanvas but with the delete modal's own hand-drawn
// single-tone joined panel (renderDeleteModalContent — the SAME frame the kill modal
// uses, three compartments). The confirm/cancel LOGIC is unchanged
// (updateDeleteProjectModal); only the rendering is reskinned. The shared
// modalBorderStyle's Padding(1,2) is left intact for the OTHER modals; this path
// bypasses it.
func renderDeleteModalOnClearedCanvas(name, path string, width, height int, mode theme.Mode, colourless bool) string {
	panel := renderDeleteModalContent(name, path, mode, colourless)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

// renderRenameModalOnClearedCanvas composes the §8.4 rename-session modal panel and
// centres it on the cleared owned canvas, exactly like renderKillModalOnClearedCanvas
// but with the rename modal's own hand-drawn single-tone joined panel
// (renderRenameModalContent — the SAME frame the help/kill modals use, three
// compartments with the violet-outlined input box nested in the body). The shared
// modalBorderStyle's Padding(1,2) is left intact for the OTHER (not-yet-reskinned)
// modals; this path bypasses it. The rename flow LOGIC (updateRenameModal /
// renameAndRefresh) is unchanged — only the rendering is reskinned.
func renderRenameModalOnClearedCanvas(input textinput.Model, oldName string, width, height int, mode theme.Mode, colourless bool) string {
	panel := renderRenameModalContent(input, oldName, mode, colourless)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

// renderEditModalOnClearedCanvas composes the §8.2/§13.1 two-mode edit-project modal
// panel and centres it on the cleared owned canvas, exactly like
// renderRenameModalOnClearedCanvas but with the edit modal's own hand-drawn
// single-tone joined panel (renderEditProjectContent — the SAME frame the
// help/kill/rename modals use, three compartments: header / body / footer). It
// BYPASSES the shared modalBorderStyle's Padding(1,2) box (which would wrap the
// already-framed joined panel in a redundant second border); the not-yet-reskinned
// modals keep that box. The edit-modal LOGIC (updateEditProjectModal) is unchanged —
// only the rendering is reskinned.
func renderEditModalOnClearedCanvas(m Model, width, height int, mode theme.Mode, colourless bool) string {
	panel := m.renderEditProjectContent()
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}
