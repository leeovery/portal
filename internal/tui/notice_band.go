package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

// noticeBandRole is one of the §11 MV notice-band role variants. The role selects
// the left-bar colour token and the band's tint:
//
//   - bandWarning → accent.orange (transient / warning flash, bg.warning tint)
//   - bandSuccess → state.green   (transient / success flash, bg.warning tint)
//   - bandInfo    → accent.violet (persistent §11.3 info band, bg.selection tint)
//   - bandCommand → accent.violet (persistent §11.4 command-pending banner, bg.selection tint)
//
// The two INFO bands (bandInfo §11.3 signpost, bandCommand §11.4 command-pending)
// are the SAME info-message element: an identical violet `▌` left-bar on the SAME
// bg.selection tint. They share one render base (renderNoticeBand) so their bar +
// tint can never drift; bandCommand layers a `▸` caret + an orange command chip on
// top (renderCommandBand). The flash roles keep their own bg.warning/glyph treatment.
type noticeBandRole int

const (
	// bandWarning is the transient warning flash role — an accent.orange left-bar.
	bandWarning noticeBandRole = iota
	// bandSuccess is the transient success flash role — a state.green left-bar.
	bandSuccess
	// bandInfo is the persistent §11.3 info band role — an accent.violet left-bar on
	// the subtle bg.selection tint (the SAME tint as the §11.4 command-pending band:
	// the signpost and the command banner are one info-message element).
	bandInfo
	// bandCommand is the persistent §11.4 command-pending banner role — an
	// accent.violet left-bar on the bg.selection tint, identical to bandInfo's base;
	// it layers a `▸` caret + an orange command chip on top (renderCommandBand).
	bandCommand
)

// commandBandCaret is the §11.4 `▸` lead glyph that follows the `▌` left-bar on the
// command-pending banner, just before the fixed text. It survives the NO_COLOR
// carve-out (its position/glyph carry the banner's intent colourlessly).
const commandBandCaret = "▸"

// commandBandText is the spec-exact §11.4 fixed banner wording, sourced once here
// as the single source of truth. The joined pending command renders beside it in an
// accent.orange chip (renderCommandBand).
const commandBandText = "Pick a project to run"

// barToken returns the §2.9 role token whose foreground paints the role's
// left-bar. No literal hex survives here — the colour is sourced from the closed
// MV vocabulary so a re-theme moves the band bars with every other element.
func (r noticeBandRole) barToken() theme.Token {
	switch r {
	case bandWarning:
		return theme.MV.AccentOrange
	case bandSuccess:
		return theme.MV.StateGreen
	default: // bandInfo, bandCommand
		return theme.MV.AccentViolet
	}
}

// tintToken returns the §2.9 surface token whose background fills the band's row.
// The transient flashes (warning AND success) sit on the single co-tuned
// bg.warning tint (§2.9 — no invented success-specific tint; the bar colour + ✓
// glyph carry the success distinction, §11.2). The TWO persistent info bands —
// bandInfo (§11.3 signpost) AND bandCommand (§11.4 command-pending) — share the
// SAME violet-anchored bg.selection surface, because they are one info-message
// element (the signpost and the command banner must look identical at the base:
// same `▌` bar, same tint). This single shared mapping is the regression guard
// that keeps the two info bands from drifting apart. The pairing is
// closed-vocabulary only (no invented token, no literal hex).
func (r noticeBandRole) tintToken() theme.Token {
	switch r {
	case bandInfo, bandCommand:
		return theme.MV.BgSelection
	default: // bandWarning, bandSuccess
		return theme.MV.BgWarning
	}
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
// for the role's surface token (bg.warning for the flashes, bg.selection for the
// info bands), or a bare style under the NO_COLOR carve-out (§2.5) so the band
// renders on the terminal's native bg. Every band cell (bar, glyph, message, the
// gaps, and the right pad) is painted through this so the whole row is one
// uniform tint with no terminal-bg island.
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

// bandBase is the §11 info-band base treatment shared by EVERY band render path:
// the role's resolved tint token plus the two pre-rendered cells that paint that
// tint — the `▌` left-bar (in the role colour) and a single tint-painted gap cell.
// Both renderNoticeBand (the §11.3 signpost + the §11.2 flashes) and
// renderCommandBand (the §11.4 command-pending banner) derive their bar + tint from
// here, so the two info bands (signpost, command) can never diverge in bar glyph,
// bar colour, or tint — there is exactly ONE place that assembles them.
type bandBase struct {
	tint theme.Token // the role's surface tint (bg.selection for the info bands)
	bar  string      // the `▌` left-bar cell, painted in the role colour on the tint
	gap  string      // a single tint-painted space cell
}

// newBandBase builds the shared info-band base for a role: the tint token + the
// tint-painted `▌` bar and gap cells. The NO_COLOR carve-out (§2.5) is honoured by
// the underlying style helpers, so under colourless the bar/gap carry no SGR.
func newBandBase(role noticeBandRole, mode theme.Mode, colourless bool) bandBase {
	tint := role.tintToken()
	return bandBase{
		tint: tint,
		bar:  noticeBandFgStyle(role.barToken(), tint, mode, colourless).Render(noticeBarGlyph),
		gap:  noticeBandTintStyle(tint, mode, colourless).Render(" "),
	}
}

// renderNoticeBand renders the §11 shared notice band: a far-left `▌` left-bar in
// the role colour, then — for the §11.2 flashes — a `⚠`/`✓` status glyph, then the
// message in the supplied on-band text token, all on the role's tint and padded to
// exactly width cells so each line spans the full row like the section header it
// sits above (single-line when the message fits, wrapping to multi-line otherwise
// — see below). The flash bands (warning / success) fill the row with the bg.warning
// tint (§11.2); the persistent info band (§11.3 signpost) carries no status glyph
// and sits on the bg.selection tint — the SAME tint as the §11.4 command-pending
// banner, since the two are one info-message element. This is the shared base both
// info bands render through (renderCommandBand layers its caret + chip on top), so
// the bar + tint can never diverge between them.
//
// The message WRAPS to the available content width (= width − the prefix width: the
// `▌` bar + its gap + (for flashes) the `⚠`/`✓` glyph + its gap). When the message
// is longer than that width the band returns a MULTI-LINE string (lines joined with
// "\n"); when it fits it stays a single line. The `▌` bar repeats on EVERY wrapped
// line (in the role colour) so the bar spans the band's full height, and
// continuation lines indent their text under line 1's message start (the `⚠`/`✓`
// glyph appears only on line 1). Wrapping is on word boundaries; a word longer than
// the available width is hard-broken. Every line is padded to exactly width cells
// with the role's tint (noticeBandPadRight) so the tint spans all wrapped lines with
// no terminal-bg island on any line.
//
// onBandText is the §2.9 text token the consuming task selects for the message
// (e.g. text.on-warning for the flashes, text.on-selection for the signpost). The
// role selects the tint and the status glyph (role.tintToken / role.statusGlyph).
//
// Under the NO_COLOR carve-out (§2.5) the bar colour, the on-band text hue, and
// the tint all drop; the `▌` bar, its position, the `⚠`/`✓` glyph (line 1), and the
// message text survive on the terminal's native fg/bg so the band's STATE stays
// legible colourlessly (§2.2 — glyph-distinct, never colour-only). The bar survives
// on every wrapped line.
func renderNoticeBand(role noticeBandRole, message string, onBandText theme.Token, width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	base := newBandBase(role, mode, colourless)
	tint := base.tint
	gap := base.gap
	bar := base.bar
	fg := noticeBandFgStyle(onBandText, tint, mode, colourless)

	// Prefix laid out before the message on line 1: the `▌` bar + gap, plus (for the
	// flashes) the status glyph + gap. The bar (1) + gap (1) [+ glyph (1) + gap (1)]
	// fixes the cell width the message starts at — the available content width and
	// the continuation-line indent both derive from it so the wrapped text lines up
	// under line 1's message start.
	glyph := role.statusGlyph()
	prefixWidth := lipgloss.Width(noticeBarGlyph) + 1 // bar + its gap
	if glyph != "" {
		prefixWidth += lipgloss.Width(glyph) + 1 // glyph + its gap
	}

	// Wrap the message to the width remaining after the prefix on word boundaries,
	// hard-breaking only a word longer than the available width (ansi.Wrap breaks a
	// word that does not fit). A non-positive available width (pathologically narrow
	// band) degrades to a 1-cell column so the bar still renders.
	avail := max(w-prefixWidth, 1)
	wrapped := strings.Split(ansi.Wrap(message, avail, ""), "\n")

	// Continuation lines lay the bar + gap (2 cells), then pad the REMAINING prefix
	// cells (prefixWidth − bar − gap = the glyph + gap slot, when present) so the
	// wrapped text aligns under line 1's message start; the glyph itself is line 1
	// only. With no glyph (prefixWidth == 2) the pad is empty and continuation text
	// already aligns under line 1's message.
	barGapWidth := lipgloss.Width(noticeBarGlyph) + 1
	var contIndent string
	if prefixWidth > barGapWidth {
		contIndent = noticeBandTintStyle(tint, mode, colourless).Render(strings.Repeat(" ", prefixWidth-barGapWidth))
	}

	lines := make([]string, 0, len(wrapped))
	for i, text := range wrapped {
		segs := []string{bar, gap}
		if i == 0 {
			if glyph != "" {
				segs = append(segs, fg.Render(glyph), gap)
			}
		} else if contIndent != "" {
			segs = append(segs, contIndent)
		}
		segs = append(segs, fg.Render(text))

		row := lipgloss.JoinHorizontal(lipgloss.Top, segs...)
		lines = append(lines, noticeBandPadRight(row, lipgloss.Width(row), w, tint, mode, colourless))
	}

	return strings.Join(lines, "\n")
}

// commandChipPadX is the §11.4 command chip's horizontal padding (cells each side)
// inside its tinted box — so the chip reads `│ npm run dev │` compactly, matching
// the reference's compact orange command chip.
const commandChipPadX = 1

// renderCommandBand renders the §11.4 command-pending banner: the §11 info-band
// BASE (the bandCommand role's `▌` violet left-bar on the bg.selection tint, sourced
// from the SAME newBandBase used by the §11.3 signpost so the bar + tint cannot
// diverge between the two info bands) WITH a `▸` violet caret + the fixed `Pick a
// project to run` text (text.on-selection) + the joined pending command in an
// accent.orange chip layered on, padded to exactly width cells so the band occupies
// the full row like the section header it sits above.
//
// The chip is a small box treatment (§11.4): the joined command in accent.orange on
// a bg.warning surface fill with one cell of horizontal padding each side, so it
// reads as a distinct orange chip ON the violet-tinted band. Both tints are existing
// §2.9 surface tokens (no invented token, no literal hex).
//
// Under the NO_COLOR carve-out (§2.5) the bar/caret/text/chip colours and both tints
// drop; the `▌` bar, the `▸` caret, the text, and the chip command survive on the
// terminal's native fg/bg, so the chip degrades to a colourless padded box still
// distinguishable by position (§2.2 — never colour-only).
func renderCommandBand(command []string, width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	base := newBandBase(bandCommand, mode, colourless)

	caret := noticeBandFgStyle(bandCommand.barToken(), base.tint, mode, colourless).Render(commandBandCaret)
	text := noticeBandFgStyle(theme.MV.TextOnSelection, base.tint, mode, colourless).Render(commandBandText)
	chip := renderCommandChip(strings.Join(command, " "), mode, colourless)

	row := lipgloss.JoinHorizontal(lipgloss.Top, base.bar, base.gap, caret, base.gap, text, base.gap, chip)
	return noticeBandPadRight(row, lipgloss.Width(row), w, base.tint, mode, colourless)
}

// renderCommandChip renders the §11.4 command chip: the joined command in
// accent.orange on a bg.warning surface fill (the orange-anchored subtle tint) with
// commandChipPadX cells of horizontal padding each side, so it reads as a distinct
// orange box ON the violet-tinted band. Under NO_COLOR all colours + the fill drop,
// leaving a padded colourless box distinguishable by position.
func renderCommandChip(command string, mode theme.Mode, colourless bool) string {
	if colourless {
		pad := strings.Repeat(" ", commandChipPadX)
		return pad + command + pad
	}
	chip := lipgloss.NewStyle().
		Foreground(theme.MV.AccentOrange.ColorFor(mode)).
		Background(theme.MV.BgWarning.ColorFor(mode)).
		Padding(0, commandChipPadX).
		Render(command)
	return chip
}

// noticeBandPadRight pads the assembled band row out to exactly w cells with
// tint-painted spaces, so the band carries its tint on every cell to the right
// edge with no terminal-bg island. A row already at/over w is returned unchanged
// (the band is clamped to w by construction at the call site). It binds the band's
// tint fill and delegates the pad geometry to the shared padRightWithStyle (the
// same core headerPadRight routes through, which pads with the canvas instead).
func noticeBandPadRight(seg string, segWidth, w int, tint theme.Token, mode theme.Mode, colourless bool) string {
	return padRightWithStyle(seg, segWidth, w, noticeBandTintStyle(tint, mode, colourless))
}

// activeNoticeBand is the §11 single-slot arbiter for the Sessions page: it
// returns the ONE band that owns the notice slot, if any. The single-slot rule —
// a transient flash wins over any persistent band while shown; otherwise the
// active persistent band occupies the slot:
//
//   - flashText != ""  → the transient flash takes the slot; its flashKind selects
//     the warning (bandWarning) or success (bandSuccess) styling (§11.2).
//   - byTagSignpost     → the persistent no-tags info band (bandInfo) — §11.3 —
//     UNLESS §5 multi-select mode is active, which replaces the section header with
//     the `N selected` banner and suppresses the signpost (a flash still wins).
//
// At most one is ever returned, so the two independent band sources collapse to a
// single arbitrated insert (no double-band). The role/message the arbiter returns
// are consumed by viewSessionList's single insertion step; the on-band text token
// is selected at the render site so each band keeps its existing on-band colour.
func (m Model) activeNoticeBand() (role noticeBandRole, message string, ok bool) {
	if m.flashText != "" {
		return flashBandRole(m.flashKind), m.flashText, true
	}
	// §5 multi-select mode replaces the section header with the `N selected` banner
	// and owns the row region; the persistent By-Tag signpost is suppressed for the
	// mode's duration (a transient flash, handled above, still wins the slot).
	if m.byTagSignpost && !m.multiSelectMode {
		return bandInfo, byTagSignpostText, true
	}
	// No active band: ok=false, so the role is don't-care (callers gate on ok); the
	// returned bandWarning is an arbitrary placeholder that is never rendered.
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
// info signpost carries text.on-selection — the bright white co-tuned for the
// bg.selection tint the info band sits on (the same token the selected
// session-row name uses on that surface), so the message stays legible.
func noticeBandOnBandText(role noticeBandRole) theme.Token {
	if role == bandInfo {
		return theme.MV.TextOnSelection
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
