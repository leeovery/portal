package tui

import "github.com/leeovery/portal/internal/theme"

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

// Drawn as authored, never wrapped or truncated: the copy is fixed and fits the
// minimum inner width, below which the panel refuses to open.
func renderThemePanelMessage(message themePanelMessage, th theme.Theme, colourless bool) string {
	text, token := themePanelMessageContent(message, th)
	if text == "" {
		return ""
	}
	return headerStyle(token, th, colourless).Render(text)
}

// The failed-commit line deliberately takes no bg.attention band — too heavy
// inside a 24–30 column panel.
func themePanelMessageContent(message themePanelMessage, th theme.Theme) (text string, token theme.Token) {
	if message.Kind == themeMessageCommitFailed {
		return themePanelCommitFailedMessage, th.AccentAttention
	}
	return "", theme.Token{}
}

// Measures the real renderer so budget and render cannot drift.
func themePanelMessageHeight(message themePanelMessage) int {
	return blockHeight(renderThemePanelMessage(message, theme.Theme{}, true))
}
