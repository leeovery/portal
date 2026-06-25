package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §8.5 per-page `?` help modal — a NEW modal type (§14.4): a generic
// two-column renderer over the per-page keymap descriptor (the single source of
// truth for the footer + help DISPLAY, §12.1), NOT hand-authored content per
// page. It lists the page's COMPLETE keymap (every descriptor entry, footer-core
// AND help-only — the full reference, not just the footer's overflow), so a
// binding change updates the footer and the help DISPLAY together. (The
// descriptor does NOT govern key dispatch — that is the live per-page Update
// switch, kept in sync via keymap_dispatch_guard_test.go; see keymap.go.)
//
// It is the documented §8.1 exception to the contextual-footer rule: the dismiss
// hint lives in the HEADER right-corner (`esc close`), and the body IS the keymap
// — there is no contextual footer. The panel reuses the shared cleared-canvas
// placement (renderHelpModalOnClearedCanvas → lipgloss.Place on the cleared owned
// canvas, §13.5) so the Sessions/Projects help inherits the 3-1 cleared-canvas
// shell, but HAND-DRAWS its OWN bordered panel (no lipgloss auto-border) so the
// header divider uses real `├`/`┤` junctions into the side frame and the vertical
// spacing is FLUSH (zero blank rows). The whole frame — corners, sides, divider,
// and every `─` run — is SINGLE-TONE border.separator (the 2-tone footer leg was
// dropped). The header text + body rows carry their own per-row inset
// (panelRowInset).
//
// NOTE: Phase 4 task 4-7 wired the Preview `?` help — it OVERLAYS the preview
// without blanking it (§8.5/§9.3) and routes the Preview keymap descriptor
// through these SAME renderers (renderHelpModalContent; see overlayHelpOnPreview
// in pagepreview.go), so the three help modals stay descriptor-driven and never
// drift.

const (
	// helpTitleGlyph is the violet `?` glyph that opens the header title row,
	// mirroring the footer's accent.violet `?` hint (§3.4) — colour reinforces
	// that this is the help surface.
	helpTitleGlyph = "?"
	// helpTitle is the header title text (text.primary), the §8.5 `? Keybindings`.
	helpTitle = "Keybindings"
	// helpDismissHint is the right-aligned header dismiss hint (text.detail) — the
	// §8.1 help-modal exception: the dismiss copy lives in the HEADER, not a
	// contextual footer. The verb has no "to" (the shared modal dismiss grammar).
	helpDismissHint = "esc close"
	// helpColumnGap is the gap between the key-glyph column and the action-label
	// column in the two-column body. Wide enough that the longest key glyph
	// ("^↑/↓") clears the labels.
	helpColumnGap = "   "
	// helpKeyColumnWidth is the fixed width of the left key-glyph column so the
	// action labels start on a common left edge regardless of glyph length
	// (fixed-width slot, the §3.4 alignment convention). Sized for the widest glyph
	// ("^↑/↓").
	helpKeyColumnWidth = 10
)

// renderHelpModalContent composes the §8.5 help modal as a fully HAND-DRAWN
// bordered panel (no lipgloss auto-border). The vertical spacing is FLUSH — ZERO
// blank rows anywhere — and the frame is SINGLE-TONE border.separator (corners,
// sides, divider, and all `─` runs alike). Top to bottom, the panel is:
//
//	top-border · title · divider · ...bodyRows · bottom-border
//
// The title sits directly inside the top border, the divider directly under the
// title, the body rows directly under the divider, the last body row directly
// above the bottom border — no blank rows between any of them (the terminal-native
// flush convention, deliberately diverging from the Paper reference's px title
// padding). The header text + body rows carry a per-row L/R inset (panelRowInset);
// the divider spans the full inner width W so its `├`/`┤` junctions meet both side
// borders. Every assembled line is exactly W+2 cells wide (W = contentWidth +
// 2·panelRowInset), so the frame columns align. Generated entirely from the
// descriptor — no hand-authored copy.
func renderHelpModalContent(entries []keymapEntry, mode theme.Mode, colourless bool) string {
	bodyRows := helpModalBodyRows(entries, mode, colourless)

	// The content width the divider and every inset row share: the widest of the
	// header band and the body rows. The header is then laid out to this width so
	// `esc close` right-aligns to the same edge the longest body row reaches. The
	// title MUST be pre-laid-out here (not inside renderJoinedPanel) because its own
	// width depends on contentWidth — the right-aligned `esc close` fills to it.
	contentWidth := lipgloss.Width(helpModalHeader(0, mode, colourless))
	for _, r := range bodyRows {
		if w := lipgloss.Width(r); w > contentWidth {
			contentWidth = w
		}
	}
	title := helpModalHeader(contentWidth, mode, colourless)

	// Two compartments — the header band over the contiguous keymap rows — drawn by
	// the shared single-tone joined panel (the SAME frame the kill modal uses): one
	// joined ├───┤ divider between them, FLUSH vertical spacing, single-tone
	// border.separator throughout.
	return renderJoinedPanel([][]string{{title}, bodyRows}, theme.MV.BorderSeparator, mode, colourless)
}

// helpModalHeader renders the header row: `? Keybindings` on the LEFT (the `?`
// glyph in accent.violet, "Keybindings" in text.primary) and a right-aligned
// `esc close` in text.detail, filled to width. This is the §8.1 help-modal
// exception — the dismiss hint lives here, not a contextual footer. When width is
// at or below the header's natural width (e.g. width 0, the natural-width probe),
// it renders at its natural width with a single canvas spacer between the title and
// the dismiss hint — never dropping the hint, never wrapping.
func helpModalHeader(width int, mode theme.Mode, colourless bool) string {
	glyph := headerStyle(theme.MV.AccentViolet, mode, colourless).Bold(true).Render(helpTitleGlyph)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	title := headerStyle(theme.MV.TextPrimary, mode, colourless).Bold(true).Render(helpTitle)
	left := lipgloss.JoinHorizontal(lipgloss.Top, glyph, gap, title)
	leftWidth := lipgloss.Width(left)

	dismiss := headerStyle(theme.MV.TextDetail, mode, colourless).Render(helpDismissHint)
	dismissWidth := lipgloss.Width(dismiss)

	// Natural width: left segment + one spacer cell + the dismiss hint. At or below
	// it (incl. the width-0 probe) render at the natural width rather than dropping
	// the hint or overflowing.
	naturalWidth := leftWidth + 1 + dismissWidth
	spacerWidth := 1
	if width > naturalWidth {
		spacerWidth = width - leftWidth - dismissWidth
	}
	spacer := headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", spacerWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, dismiss)
}

// helpModalBody renders the two-column keymap body from the descriptor as a single
// joined block (used by tests that assert the body content/colours). Production
// composition uses helpModalBodyRows so each row can be inset individually.
func helpModalBody(entries []keymapEntry, mode theme.Mode, colourless bool) string {
	return lipgloss.JoinVertical(lipgloss.Left, helpModalBodyRows(entries, mode, colourless)...)
}

// helpModalBodyRows renders the two-column keymap body from the descriptor as one
// string per row: a fixed-width key-glyph column (accent.blue, the destructive
// `kill` key in state.red per §2.9) then the action label (text.strong). It lists
// EVERY descriptor entry — footer-core AND help-only (§8.5 "the full reference") —
// EXCEPT the `?` help self-entry (a help modal does not list its own open key; the
// dismiss hint is in the header). The longer HelpAction label is preferred, falling
// back to the terse footer Action when absent.
func helpModalBodyRows(entries []keymapEntry, mode theme.Mode, colourless bool) []string {
	rows := make([]string, 0, len(entries))
	for _, e := range entries {
		// Skip the ? help self-entry — the open key is not listed in its own modal.
		if e.RightAligned {
			continue
		}
		rows = append(rows, helpModalRow(e, mode, colourless))
	}
	return rows
}

// helpModalRow renders one keymap entry as a two-column line: the key glyph in a
// fixed-width left column (accent.blue, or state.red for the destructive kill/
// delete key — §2.9 reserves red for destructive actions), a fixed gap, then the
// action label in text.strong.
func helpModalRow(e keymapEntry, mode theme.Mode, colourless bool) string {
	keyTok := theme.MV.AccentBlue
	if isDestructiveHelpKey(e) {
		keyTok = theme.MV.StateRed
	}
	key := headerStyle(keyTok, mode, colourless).Bold(true).Render(helpKeyGlyph(e))
	keyWidth := lipgloss.Width(key)
	// Pad the key column to a fixed width so labels share a left edge. The pad is
	// canvas-painted so the column gap is not a terminal-bg island.
	pad := ""
	if keyWidth < helpKeyColumnWidth {
		pad = headerCanvasBg(mode, colourless).Render(strings.Repeat(" ", helpKeyColumnWidth-keyWidth))
	}
	gap := headerCanvasBg(mode, colourless).Render(helpColumnGap)
	label := headerStyle(theme.MV.TextStrong, mode, colourless).Render(helpActionLabel(e))
	return lipgloss.JoinHorizontal(lipgloss.Top, key, pad, gap, label)
}

// helpActionLabel returns the label the help modal shows for an entry: the longer
// HelpAction when set, else the terse footer Action.
func helpActionLabel(e keymapEntry) string {
	if e.HelpAction != "" {
		return e.HelpAction
	}
	return e.Action
}

// helpKeyGlyph returns the key glyph the help modal renders for an entry: the
// glyph-rich HelpKey when set, else the terse footer Key. Post the §3.4
// footer-glyph switch the footer Key forms are glyphs themselves; the surviving
// HelpKey override is nav (footer "↑↓" vs the help body's slashed "↑/↓"), with
// page reading its Key "^↑/↓" directly and enter/space's HelpKey now coinciding
// with their glyph Key. The condensed sessions/projects footer never calls this —
// it reads Key directly; the command-pending footer reuses this resolver to share
// the `enter`→`⏎` encoding.
func helpKeyGlyph(e keymapEntry) string {
	if e.HelpKey != "" {
		return e.HelpKey
	}
	return e.Key
}

// isDestructiveHelpKey reports whether the entry is a destructive action whose key
// glyph renders in state.red in the help body (§2.9 red is destructive-only). It
// reads the structural keymapEntry.Destructive flag (set on the Sessions `k` kill
// and Projects `d` delete entries) rather than matching key glyphs, so a future
// non-destructive `d`/`k` binding cannot accidentally render red.
func isDestructiveHelpKey(e keymapEntry) bool {
	return e.Destructive
}
