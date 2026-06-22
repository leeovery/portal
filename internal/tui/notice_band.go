package tui

import (
	"strings"

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

// flashWarningGlyph / flashSuccessGlyph are the §11.2 flash status glyphs. They
// follow the message on the warning / success flash band so the two variants stay
// distinguishable by GLYPH alone (§2.2 — never colour-only), which is what keeps
// the NO_COLOR carve-out legible: the ⚠ / ✓ survives even when the tint and bar
// colour drop. The persistent info band (§11.3 signpost) carries NO status glyph.
const (
	flashWarningGlyph = "⚠"
	flashSuccessGlyph = "✓"
)

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

// tintToken returns the §2.9 surface token whose background fills the band's row.
// The transient flashes (warning AND success) sit on the single co-tuned
// bg.warning tint (§2.9 — no invented success-specific tint; the bar colour + ✓
// glyph carry the success distinction, §11.2). The persistent info band carries
// NO tint: it renders on the owned canvas (Canvas), matching the §11.3 signpost's
// existing flush-on-canvas treatment. The pairing is closed-vocabulary only.
func (r noticeBandRole) tintToken() theme.Token {
	if r == bandInfo {
		return theme.MV.Canvas
	}
	return theme.MV.BgWarning
}

// statusGlyph returns the §11.2 status glyph that follows the bar on the role's
// band, or "" for the persistent info band (which carries no status glyph). The
// glyph is what keeps the warning / success variants distinguishable without
// relying on colour (§2.2) and is what survives the NO_COLOR carve-out (§2.5).
func (r noticeBandRole) statusGlyph() string {
	switch r {
	case bandWarning:
		return flashWarningGlyph
	case bandSuccess:
		return flashSuccessGlyph
	default: // bandInfo
		return ""
	}
}

// noticeBandTintStyle returns the band's BACKGROUND-fill style: Background(tint)
// for the role's surface token (bg.warning for the flashes, the owned canvas for
// the info band), or a bare style under the NO_COLOR carve-out (§2.5) so the band
// renders on the terminal's native bg. Every band cell (bar, glyph, message, the
// gaps, and the right pad) is painted through this so the whole row is one
// uniform tint with no terminal-bg island and no canvas island mid-band.
func noticeBandTintStyle(tint theme.Token, mode theme.Mode, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Background(tint.ColorFor(mode))
}

// noticeBandFgStyle returns the on-band FOREGROUND style: the supplied role token
// over the band's tint background — the bar (role colour) and the on-band text /
// glyph (onBandText) leg both paint through this so their cells carry the same
// tint as the gaps. Under NO_COLOR it is a bare style (no hue, no tint).
func noticeBandFgStyle(fg, tint theme.Token, mode theme.Mode, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().
		Foreground(fg.ColorFor(mode)).
		Background(tint.ColorFor(mode))
}

// renderNoticeBand renders the §11 shared notice band: a far-left `▌` left-bar in
// the role colour, then — for the §11.2 flashes — a `⚠`/`✓` status glyph, then the
// message in the supplied on-band text token, all on the role's tint and padded to
// exactly width cells so the band occupies the full row like the section header it
// sits above. The flash bands (warning / success) fill the row with the bg.warning
// tint (§11.2); the persistent info band (§11.3 signpost) carries no status glyph
// and renders on the owned canvas (its tint token is Canvas), preserving its
// existing flush treatment.
//
// onBandText is the §2.9 text token the consuming task selects for the message
// (e.g. text.on-warning for the flashes, text.strong for the signpost). The role
// selects the tint and the status glyph (role.tintToken / role.statusGlyph).
//
// Under the NO_COLOR carve-out (§2.5) the bar colour, the on-band text hue, and
// the tint all drop; the `▌` bar, its position, the `⚠`/`✓` glyph, and the message
// text survive on the terminal's native fg/bg so the band's STATE stays legible
// colourlessly (§2.2 — glyph-distinct, never colour-only).
func renderNoticeBand(role noticeBandRole, message string, onBandText theme.Token, width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	tint := role.tintToken()
	gap := noticeBandTintStyle(tint, mode, colourless).Render(" ")

	bar := noticeBandFgStyle(role.barToken(), tint, mode, colourless).Render(noticeBarGlyph)
	segs := []string{bar, gap}
	if glyph := role.statusGlyph(); glyph != "" {
		segs = append(segs,
			noticeBandFgStyle(onBandText, tint, mode, colourless).Render(glyph),
			gap,
		)
	}
	segs = append(segs, noticeBandFgStyle(onBandText, tint, mode, colourless).Render(message))

	row := lipgloss.JoinHorizontal(lipgloss.Top, segs...)
	return noticeBandPadRight(row, lipgloss.Width(row), w, tint, mode, colourless)
}

// noticeBandPadRight pads the assembled band row out to exactly w cells with
// tint-painted spaces, so the band carries its tint on every cell to the right
// edge with no terminal-bg island. A row already at/over w is returned unchanged
// (the band is clamped to w by construction at the call site). It mirrors
// headerPadRight but pads with the band's tint instead of the canvas.
func noticeBandPadRight(seg string, segWidth, w int, tint theme.Token, mode theme.Mode, colourless bool) string {
	if segWidth >= w {
		return seg
	}
	pad := noticeBandTintStyle(tint, mode, colourless).Render(strings.Repeat(" ", w-segWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, seg, pad)
}

// activeNoticeBand is the §11 single-slot arbiter for the Sessions page: it
// returns the ONE band that owns the notice slot, if any. The single-slot rule —
// a transient flash wins over any persistent band while shown; otherwise the
// active persistent band occupies the slot:
//
//   - flashText != ""  → the transient flash takes the slot; its flashKind selects
//     the warning (bandWarning) or success (bandSuccess) styling (§11.2).
//   - byTagSignpost     → the persistent no-tags info band (bandInfo) — §11.3.
//
// At most one is ever returned, so the dual independent insertRowBelowTitle calls
// collapse to a single arbitrated insert (no double-band). The role/message the
// arbiter returns are consumed by viewSessionList's single insertion step; the
// on-band text token is selected at the render site so each band keeps its
// existing on-band colour.
func (m Model) activeNoticeBand() (role noticeBandRole, message string, ok bool) {
	if m.flashText != "" {
		return flashBandRole(m.flashKind), m.flashText, true
	}
	if m.byTagSignpost {
		return bandInfo, byTagSignpostText, true
	}
	return bandWarning, "", false
}

// flashBandRole maps an inline-flash kind (§11.2) to the shared notice-band role
// that selects its bar colour, tint, and status glyph. flashWarning is the
// default, so an unparameterised setFlash routes to bandWarning.
func flashBandRole(kind flashKind) noticeBandRole {
	if kind == flashSuccess {
		return bandSuccess
	}
	return bandWarning
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
// point the band SLOT (renderSessionBandSlot) composes beneath the title
// separator, and from which the slot's height is measured, so the budget and the
// render agree.
func (m Model) renderActiveNoticeBand() string {
	role, message, ok := m.activeNoticeBand()
	if !ok {
		return ""
	}
	return renderNoticeBand(role, message, noticeBandOnBandText(role), m.contentWidth(), m.canvasMode, m.colourless)
}

// renderSessionBandSlot renders the FULL §11 Sessions notice slot for the model's
// current width / canvas mode — the arbitrated band PLUS one canvas-painted
// full-width blank row BENEATH it (the band→section-header breathing gap), or the
// empty string when no band owns the slot. The blank sits ONLY below the band: the
// band stays flush under the title separator, the blank separates it from the
// section header (line 0 of listView), so the slot composes as band → blank →
// listView.
//
// This is the SINGLE source of truth for what the slot inserts: both
// viewSessionList (composition) and sessionBandHeight (the F10 height reserve)
// consume it, so the reserved row count is, by construction, exactly the rendered
// height of what is composed — the two can never drift. The blank is an explicit
// canvas-painted full-width row (blankCanvasRow) rather than a bare "" left for
// the outer fill to pad, so the slot's height is deterministic and the row paints
// to the owned canvas with no ragged/zero-width gap (and survives NO_COLOR §2.5).
func (m Model) renderSessionBandSlot() string {
	band := m.renderActiveNoticeBand()
	if band == "" {
		return ""
	}
	blank := blankCanvasRow(m.contentWidth(), m.canvasMode, m.colourless)
	return lipgloss.JoinVertical(lipgloss.Left, band, blank)
}
