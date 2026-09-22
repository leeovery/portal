package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

const noticeBarGlyph = "▌"

// Distinguishable by glyph alone, which is what survives NO_COLOR. The
// persistent info band carries none.
const (
	flashWarningGlyph = "⚠"
	flashSuccessGlyph = "✓"
)

// bandInfo and bandCommand are the same element at the base — same bar, same
// tint — with bandCommand only layering a caret + chip on top.
type noticeBandRole int

const (
	bandWarning noticeBandRole = iota
	bandSuccess
	bandInfo
	bandCommand
)

const commandBandCaret = "▸"

const commandBandText = "Pick a project to run"

// The wait a pick taken while the concurrent bootstrap is still running
// announces: the keypress was taken, and the mint follows the bootstrap.
const stagedMintBandText = "Finishing startup — your pick will run when it's done"

func (r noticeBandRole) barToken(th theme.Theme) theme.Token {
	switch r {
	case bandWarning:
		return th.AccentAttention
	case bandSuccess:
		return th.StatePositive
	default:
		return th.AccentPrimary
	}
}

// Both flashes share one tint; the bar colour and ✓/⚠ glyph carry the distinction.
func (r noticeBandRole) tintToken(th theme.Theme) theme.Token {
	switch r {
	case bandInfo, bandCommand:
		return th.BgSelection
	default:
		return th.BgAttention
	}
}

func (r noticeBandRole) statusGlyph() string {
	switch r {
	case bandWarning:
		return flashWarningGlyph
	case bandSuccess:
		return flashSuccessGlyph
	default:
		return ""
	}
}

// Band cells with no foreground paint through this, so the row is one tint with
// no bg island.
func noticeBandTintStyle(tint theme.Token, th theme.Theme, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Background(tint.Color())
}

func noticeBandFgStyle(fg, tint theme.Token, th theme.Theme, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().
		Foreground(fg.Color()).
		Background(tint.Color())
}

// Shared so band renders cannot diverge in bar glyph, bar colour, or tint.
type bandBase struct {
	tint theme.Token
	bar  string
	gap  string
}

func newBandBase(role noticeBandRole, th theme.Theme, colourless bool) bandBase {
	tint := role.tintToken(th)
	return bandBase{
		tint: tint,
		bar:  noticeBandFgStyle(role.barToken(th), tint, th, colourless).Render(noticeBarGlyph),
		gap:  noticeBandTintStyle(tint, th, colourless).Render(" "),
	}
}

// The result is multi-line when the message does not fit the width left after
// the prefix, so a caller budgeting rows must measure the rendered height.
func renderNoticeBand(role noticeBandRole, message string, onBandText theme.Token, width int, th theme.Theme, colourless bool) string {
	w := headerWidthOrFallback(width)
	base := newBandBase(role, th, colourless)
	tint := base.tint
	gap := base.gap
	bar := base.bar
	fg := noticeBandFgStyle(onBandText, tint, th, colourless)

	glyph := role.statusGlyph()
	prefixWidth := lipgloss.Width(noticeBarGlyph) + 1
	if glyph != "" {
		prefixWidth += lipgloss.Width(glyph) + 1
	}

	// A pathologically narrow band degrades to a 1-cell column so the bar still renders.
	avail := max(w-prefixWidth, 1)
	wrapped := wrappedLines(message, avail)

	barGapWidth := lipgloss.Width(noticeBarGlyph) + 1
	var contIndent string
	if prefixWidth > barGapWidth {
		contIndent = noticeBandTintStyle(tint, th, colourless).Render(strings.Repeat(" ", prefixWidth-barGapWidth))
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
		lines = append(lines, noticeBandPadRight(row, lipgloss.Width(row), w, tint, th, colourless))
	}

	return strings.Join(lines, "\n")
}

const commandChipPadX = 1

func renderCommandBand(command []string, width int, th theme.Theme, colourless bool) string {
	w := headerWidthOrFallback(width)
	base := newBandBase(bandCommand, th, colourless)

	caret := noticeBandFgStyle(bandCommand.barToken(th), base.tint, th, colourless).Render(commandBandCaret)
	text := noticeBandFgStyle(th.TextOnSelection, base.tint, th, colourless).Render(commandBandText)
	chip := renderCommandChip(strings.Join(command, " "), th, colourless)

	row := lipgloss.JoinHorizontal(lipgloss.Top, base.bar, base.gap, caret, base.gap, text, base.gap, chip)
	return noticeBandPadRight(row, lipgloss.Width(row), w, base.tint, th, colourless)
}

func renderCommandChip(command string, th theme.Theme, colourless bool) string {
	if colourless {
		pad := strings.Repeat(" ", commandChipPadX)
		return pad + command + pad
	}
	chip := lipgloss.NewStyle().
		Foreground(th.AccentAttention.Color()).
		Background(th.BgAttention.Color()).
		Padding(0, commandChipPadX).
		Render(command)
	return chip
}

func noticeBandPadRight(seg string, segWidth, w int, tint theme.Token, th theme.Theme, colourless bool) string {
	return padRightWithStyle(seg, segWidth, w, noticeBandTintStyle(tint, th, colourless))
}

// The flash arm deliberately ignores m.multiSelectMode so a spawn-failure flash
// co-renders with the `N selected` banner — do not add `&& !m.multiSelectMode`.
// The filter line has no arm here: it claims the section-header row, a different
// physical row, so the two co-render.
func (m Model) activeNoticeBand() (role noticeBandRole, message string, ok bool) {
	if role, message, live := m.flashSlotClaim(); live {
		return role, message, true
	}
	// unsupportedBannerActive is the same predicate applySectionHeader's
	// claimant reads, so the suppression and the swap cannot drift.
	if m.byTagSignpost && !m.multiSelectMode && !m.unsupportedBannerActive() {
		return bandInfo, byTagSignpostText, true
	}
	// ok=false, so the returned role is a never-rendered placeholder.
	return bandWarning, "", false
}

// The slot is deliberately not scoped to a set of flash origins: closing the
// theme panel discharges an outstanding failed-commit report whether or not a
// flash rendered, so filtering by set-site would destroy it rather than defer it.
func (m Model) activeProjectNoticeBand() (role noticeBandRole, message string, ok bool) {
	if role, message, live := m.flashSlotClaim(); live {
		return role, message, true
	}
	// Above the commandPending arm: the banner asking for a pick is displaced
	// once the pick is made.
	if m.stagedMint {
		return bandInfo, stagedMintBandText, true
	}
	if m.commandPending {
		return bandCommand, commandBandText, true
	}
	return bandWarning, "", false
}

// The bandCommand arm composes its own caret + chip from the model's pending
// command, so it does not consume the returned message.
func (m Model) renderActiveProjectNoticeBand() string {
	role, message, ok := m.activeProjectNoticeBand()
	if !ok {
		return ""
	}
	if role == bandCommand {
		return renderCommandBand(m.command, m.contentWidth(), m.themeState.active, m.colourless)
	}
	return renderNoticeBand(role, message, noticeBandOnBandText(role, m.themeState.active), m.contentWidth(), m.themeState.active, m.colourless)
}

func flashBandRole(kind flashKind) noticeBandRole {
	if kind == flashSuccess {
		return bandSuccess
	}
	return bandWarning
}

func (m Model) flashClaim() (role noticeBandRole, message string, ok bool) {
	if m.flashText == "" {
		return bandWarning, "", false
	}
	return flashBandRole(m.flashKind), m.flashText, true
}

// Reads the origin the flash carries and never the message text: precedence
// keyed on wording would move a signal out of the tier on a copy edit.
func (m Model) themeFlashClaim() (role noticeBandRole, message string, ok bool) {
	if m.flashOrigin != flashOriginTheme {
		return bandWarning, "", false
	}
	return m.flashClaim()
}

// A forward guard rather than today's mechanism: the model holds one flashText,
// so both arms return the same claim and this ordering changes no rendered
// outcome. A theme signal reaches the band under any filter state because the
// filter line claims the section-header row instead of contending here. The tier
// is kept rather than folded away because it is granted at set time by
// setThemeFlash: should the slot gain a contender that can stay live for a whole
// panel session, it belongs below this arm, and without the tier it would rank
// above a theme signal unnoticed.
func (m Model) flashSlotClaim() (role noticeBandRole, message string, ok bool) {
	if role, message, live := m.themeFlashClaim(); live {
		return role, message, true
	}
	return m.flashClaim()
}

// text.on-selection is co-tuned for the info band's tint.
func noticeBandOnBandText(role noticeBandRole, th theme.Theme) theme.Token {
	if role == bandInfo {
		return th.TextOnSelection
	}
	return th.TextOnAttention
}

func (m Model) renderActiveNoticeBand() string {
	role, message, ok := m.activeNoticeBand()
	if !ok {
		return ""
	}
	return renderNoticeBand(role, message, noticeBandOnBandText(role, m.themeState.active), m.contentWidth(), m.themeState.active, m.colourless)
}

// Composition and height reserve both consume this, so the reserved row count is
// by construction the rendered height. The blank is an explicit canvas row, not
// "", so the height is deterministic.
func (m Model) renderSessionBandSlot() string {
	band := m.renderActiveNoticeBand()
	if band == "" {
		return ""
	}
	blank := blankCanvasRow(m.contentWidth(), m.themeState.active, m.colourless)
	return lipgloss.JoinVertical(lipgloss.Left, band, blank)
}
