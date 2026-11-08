package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// The `⚠` is part of the string so the report survives NO_COLOR.
const themePanelCommitFailedMessage = flashWarningGlyph + " couldn't save theme"

// themeMessageNone is the zero value, so an empty slot needs no second flag.
type themePanelMessageKind int

const (
	themeMessageNone themePanelMessageKind = iota
	themeMessageCommitFailed
)

type themePanelMessage struct {
	Kind themePanelMessageKind
}

func (m *Model) raiseThemePanelCommitFailed() {
	m.setThemePanelMessage(themePanelMessage{Kind: themeMessageCommitFailed})
}

func (m *Model) clearThemePanelMessage() {
	m.setThemePanelMessage(themePanelMessage{})
}

func (m *Model) clearThemePanelCommitFailed() {
	if m.themePanel.message.Kind == themeMessageCommitFailed {
		m.clearThemePanelMessage()
	}
}

// The re-size is not cosmetic: the slot moves the vertical budget both ways, and
// a stale budget keeps a PerPage the drawn frame does not have. Assumes the panel
// is open — left unguarded rather than defended speculatively.
func (m *Model) setThemePanelMessage(message themePanelMessage) {
	m.themePanel.message = message
	m.applyThemePanelListStyles()
}

// The cap bounds the vertical budget: themePanelBlock cuts overflow from the
// bottom — off the footer, `esc close` first.
const themePanelMessageWrapRows = 2

// Width and height degrade differently on purpose: at minimum width the slot
// wraps; at the height floor it truncates. wrap comes from the caller so budget
// and block resolve it identically.
func renderThemePanelMessage(message themePanelMessage, inner int, wrap bool, th theme.Theme, colourless bool) string {
	text, token := themePanelMessageContent(message, th)
	if text == "" {
		return ""
	}
	return headerStyle(token, th, colourless).Render(themePanelMessageText(text, inner, wrap))
}

// The failed-commit line deliberately takes no bg.attention band — too heavy
// inside a 24–30 column panel.
func themePanelMessageContent(message themePanelMessage, th theme.Theme) (text string, token theme.Token) {
	if message.Kind == themeMessageCommitFailed {
		return themePanelCommitFailedMessage, th.AccentAttention
	}
	return "", theme.Token{}
}

func themePanelMessageText(message string, inner int, wrap bool) string {
	width := max(inner, 0)
	if !wrap || width == 0 {
		return ansi.Truncate(message, width, themeRowEllipsis)
	}
	return strings.Join(wrapCapped(message, width, themePanelMessageWrapRows, themeRowEllipsis), "\n")
}

// Measures the real renderer so budget and render cannot drift.
func themePanelMessageHeight(message themePanelMessage, inner int, wrap bool) int {
	return blockHeight(renderThemePanelMessage(message, inner, wrap, theme.Theme{}, true))
}

// "At or below" truncates because a fixture can hand a sub-floor height. The
// floor is the one for the header shape being drawn: at the page-aligned
// threshold the extra header rows leave the slot as tight as the compact floor.
func themePanelMessageWraps(p themePanel, height int) bool {
	return height > themePanelFloorFor(
		themePanelHeaderShapeFor(height, p.bandRows, p.union.DirUnusable).rows,
		p.union.DirUnusable,
	)
}
