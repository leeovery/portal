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

// placeModalOnClearedCanvas is the SINGLE home of the §8.1/§13.5 cleared-canvas
// modal centring: it dead-centres an already-built panel string in the inset
// content region (width × height) via lipgloss.Place, both axes Center. Every
// render*ModalOnClearedCanvas wrapper supplies only its own panel (its content
// builder differs; the placement maths does not) and routes the final placement
// through here, so the centring expression — and any future change to it, e.g. a
// non-centred placement — lives in exactly one place rather than the five verbatim
// copies that had accreted across the 3-4…3-9 modal reskin tasks. The outer
// fillCanvas wrap in View() then paints the owned mode-matched canvas into every
// surrounding cell (NO_COLOR suppression + the 80×24 fallback inherited from that
// Phase 1 path), so this layer only centres — it never paints the backdrop.
func placeModalOnClearedCanvas(panel string, width, height int) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

// renderHelpModalOnClearedCanvas composes the §8.5 help modal panel and centres it
// on the cleared owned canvas via placeModalOnClearedCanvas. Like every modal it
// supplies its OWN fully HAND-DRAWN panel: renderHelpModalContent assembles the
// whole bordered frame itself (top/bottom borders, side `│`, the joined `├`/`┤`
// divider, and the flush content rows) — there is NO lipgloss auto-border wrap, so
// the divider can carry real junctions into the side frame and the vertical spacing
// is flush (zero blank rows). The frame is SINGLE-TONE border.separator, mode- and
// colourless-aware. The same lipgloss.Place centres it on the inset region.
func renderHelpModalOnClearedCanvas(entries []keymapEntry, width, height int, mode theme.Mode, colourless bool) string {
	panel := renderHelpModalContent(entries, mode, colourless)
	return placeModalOnClearedCanvas(panel, width, height)
}

// renderKillModalOnClearedCanvas composes the §8.3 kill-confirm modal panel and
// centres it on the cleared owned canvas via placeModalOnClearedCanvas, like every
// modal wrapper but with the kill modal's own hand-drawn single-tone joined panel
// (renderKillModalContent — the SAME frame the help modal uses, three compartments
// instead of two).
func renderKillModalOnClearedCanvas(name string, windows int, width, height int, mode theme.Mode, colourless bool) string {
	panel := renderKillModalContent(name, windows, mode, colourless)
	return placeModalOnClearedCanvas(panel, width, height)
}

// renderDeleteModalOnClearedCanvas composes the §8.6 delete-project confirm modal
// panel and centres it on the cleared owned canvas via placeModalOnClearedCanvas,
// like every modal wrapper but with the delete modal's own hand-drawn single-tone
// joined panel (renderDeleteModalContent — the SAME frame the kill modal uses, three
// compartments). The confirm/cancel LOGIC is unchanged (updateDeleteProjectModal);
// only the rendering is reskinned.
func renderDeleteModalOnClearedCanvas(name, path string, width, height int, mode theme.Mode, colourless bool) string {
	panel := renderDeleteModalContent(name, path, mode, colourless)
	return placeModalOnClearedCanvas(panel, width, height)
}

// renderRenameModalOnClearedCanvas composes the §8.4 rename-session modal panel and
// centres it on the cleared owned canvas via placeModalOnClearedCanvas, like every
// modal wrapper but with the rename modal's own hand-drawn single-tone joined panel
// (renderRenameModalContent — the SAME frame the help/kill modals use, three
// compartments with the violet-outlined input box nested in the body). The rename
// flow LOGIC (updateRenameModal / renameAndRefresh) is unchanged — only the
// rendering is reskinned.
func renderRenameModalOnClearedCanvas(input textinput.Model, oldName string, width, height int, mode theme.Mode, colourless bool) string {
	panel := renderRenameModalContent(input, oldName, mode, colourless)
	return placeModalOnClearedCanvas(panel, width, height)
}

// renderEditModalOnClearedCanvas composes the §8.2/§13.1 two-mode edit-project modal
// panel and centres it on the cleared owned canvas via placeModalOnClearedCanvas,
// like every modal wrapper but with the edit modal's own hand-drawn single-tone
// joined panel (renderEditProjectContent — the SAME frame the help/kill/rename modals
// use, three compartments: header / body / footer). Because the panel is already
// fully framed it is placed directly — there is NO lipgloss auto-border wrap that
// would add a redundant second border. The edit-modal LOGIC (updateEditProjectModal)
// is unchanged — only the rendering is reskinned.
func renderEditModalOnClearedCanvas(m Model, width, height int, mode theme.Mode, colourless bool) string {
	panel := m.renderEditProjectContent()
	return placeModalOnClearedCanvas(panel, width, height)
}
