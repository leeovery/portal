package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §11 shared notice-band primitive + single-slot arbiter.
//
// §11 intro pins one convention for inline notices: a `▌` left-bar accent line
// directly under the title separator, above the section header (full-width), the
// section header + list shifting down. The notice slot holds AT MOST ONE band —
// a transient flash (§11.2) takes the slot temporarily, replacing any persistent
// band (no-tags signpost §11.3, command-pending banner §11.4) for its duration,
// then the persistent band returns. A persistent (violet info) band and a
// transient flash never display at once — the transient flash wins while shown.
//
// This file owns the band ROLE vocabulary, the shared render primitive, and the
// Sessions-page arbiter that funnels both notice sources through a single slot.
// The per-band tint, on-band text token, message wording, and (for flashes) the
// ⚠/✓ glyph + bold/dim are selected by the CONSUMING tasks (4-2/4-3/4-4) — this
// task supplies the structural slot + the one-row-per-delegate height recompute
// (F10).

// noticeBarGlyph is the §11 left-bar accent glyph — a solid block pinned far-left
// in the band's role colour (accent.orange / state.green / accent.violet). Under
// NO_COLOR (§2.5) the glyph + its position survive; only the colour drops.
const noticeBarGlyph = "▌"

// noticeBandRole is one of the three §11 MV notice-band role variants. The role
// selects the left-bar colour token:
//
//   - bandWarning → accent.orange (transient / warning flash)
//   - bandSuccess → state.green   (transient / success flash)
//   - bandInfo    → accent.violet (persistent mode / info band)
type noticeBandRole int

const (
	// bandWarning is the transient warning flash role — an accent.orange left-bar.
	bandWarning noticeBandRole = iota
	// bandSuccess is the transient success flash role — a state.green left-bar.
	bandSuccess
	// bandInfo is the persistent mode/info band role — an accent.violet left-bar.
	bandInfo
)

// barToken returns the §2.9 role token whose foreground paints the role's
// left-bar. No literal hex survives here — the colour is sourced from the closed
// MV vocabulary so a re-theme moves the band bars with every other element.
func (r noticeBandRole) barToken() theme.Token {
	switch r {
	case bandWarning:
		return theme.MV.AccentOrange
	case bandSuccess:
		return theme.MV.StateGreen
	default: // bandInfo
		return theme.MV.AccentViolet
	}
}

// renderNoticeBand renders the §11 shared notice band: a far-left `▌` left-bar in
// the role colour, a single space, then the message in the supplied on-band text
// token — padded to exactly width cells so the band occupies the full row like
// the section header it sits above. It reuses the shared chrome paint helpers
// (headerStyle / headerCanvasBg / headerPadRight) so the band aligns under the
// title separator on the owned canvas (§1) with no terminal-bg island.
//
// onBandText is the §2.9 text token the consuming task selects for the message
// (e.g. text.on-warning for the warning flash, text.strong for the signpost).
//
// Under the NO_COLOR carve-out (§2.5) the bar colour, the on-band text hue, and
// the canvas all drop; the `▌` glyph, its position, and the message text survive
// on the terminal's native fg/bg so the band stays present and legible.
func renderNoticeBand(role noticeBandRole, message string, onBandText theme.Token, width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	bar := headerStyle(role.barToken(), mode, colourless).Render(noticeBarGlyph)
	gap := headerCanvasBg(mode, colourless).Render(" ")
	msg := headerStyle(onBandText, mode, colourless).Render(message)
	row := lipgloss.JoinHorizontal(lipgloss.Top, bar, gap, msg)
	return headerPadRight(row, lipgloss.Width(row), w, mode, colourless)
}

// activeNoticeBand is the §11 single-slot arbiter for the Sessions page: it
// returns the ONE band that owns the notice slot, if any. The single-slot rule —
// a transient flash wins over any persistent band while shown; otherwise the
// active persistent band occupies the slot:
//
//   - flashText != ""  → the transient warning flash (bandWarning) takes the slot.
//   - byTagSignpost     → the persistent no-tags info band (bandInfo) — §11.3.
//
// At most one is ever returned, so the dual independent insertRowBelowTitle calls
// collapse to a single arbitrated insert (no double-band). The role/message the
// arbiter returns are consumed by viewSessionList's single insertion step; the
// on-band text token is selected at the render site so each band keeps its
// existing on-band colour.
func (m Model) activeNoticeBand() (role noticeBandRole, message string, ok bool) {
	if m.flashText != "" {
		return bandWarning, m.flashText, true
	}
	if m.byTagSignpost {
		return bandInfo, byTagSignpostText, true
	}
	return bandWarning, "", false
}

// noticeBandOnBandText selects the §2.9 on-band text token for the arbitrated
// band role. The warning/success flash carries text.on-warning; the persistent
// info signpost carries text.strong — preserving each band's existing on-band
// colour as the arbiter funnels both through the shared primitive.
func noticeBandOnBandText(role noticeBandRole) theme.Token {
	if role == bandInfo {
		return theme.MV.TextStrong
	}
	return theme.MV.TextOnWarning
}

// renderActiveNoticeBand renders the arbitrated Sessions-page notice band for the
// model's current width / resolved canvas mode (and the NO_COLOR carve-out), or
// the empty string when no band owns the slot. It is the single render entry
// point viewSessionList inserts beneath the title separator, and the single
// height source sessionBandHeight measures, so the budget and the render agree.
func (m Model) renderActiveNoticeBand() string {
	role, message, ok := m.activeNoticeBand()
	if !ok {
		return ""
	}
	return renderNoticeBand(role, message, noticeBandOnBandText(role), m.contentWidth(), m.canvasMode, m.colourless)
}
