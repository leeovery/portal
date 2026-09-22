package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// The slot's two contenders are exclusive by construction: the confirm gates the
// write, so by the time a write can fail it has resolved.

const (
	// The two spaces before `y` are deliberate. One format string, so the width
	// derives from the copy rather than a restated literal.
	themePanelConfirmFormat = "clear constant %s?  y / n"

	// The `⚠` is part of the string so the report survives NO_COLOR.
	themePanelCommitFailedMessage = flashWarningGlyph + " couldn't save theme"
)

// themeMessageNone is the zero value, so an empty slot needs no second flag.
type themePanelMessageKind int

const (
	themeMessageNone themePanelMessageKind = iota
	themeMessageConfirm
	themeMessageCommitFailed
)

// Writers assign the whole value, so a confirm's slug cannot survive into a
// failed-commit line as residue.
type themePanelMessage struct {
	Kind themePanelMessageKind
	// The raw persisted key, not the resolved slug.
	Slug string
}

// Reads the raw keys, not the nomination: under a fallback the persisted slug
// may be the one that failed to load, and naming the resolution would ask the
// user to confirm clearing a theme they never set.
func (m *Model) raiseThemePanelConfirm() {
	m.setThemePanelMessage(themePanelMessage{Kind: themeMessageConfirm, Slug: m.themeState.keys.Theme})
}

func (m *Model) raiseThemePanelCommitFailed() {
	m.setThemePanelMessage(themePanelMessage{Kind: themeMessageCommitFailed})
}

func (m *Model) clearThemePanelMessage() {
	m.setThemePanelMessage(themePanelMessage{})
}

// The kind check is the point: this runs ahead of every key, and the confirm's
// answers are keys — an unconditional clear would take the question down.
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

// The confirm cannot advertise the standing footer — none of those keys would
// act. It substitutes a scope, not a renderer.
func themePanelFooterScope(message themePanelMessage) []keymapEntry {
	if message.Kind == themeMessageConfirm {
		return themePanelConfirmKeymap()
	}
	return themePanelKeymap()
}

// The cap bounds the vertical budget: themePanelBlock cuts overflow from the
// bottom — off the footer, `esc close` first.
const themePanelMessageWrapRows = 2

// Width and height degrade differently on purpose: at minimum width the slot
// wraps; at the height floor it truncates. wrap comes from the caller so budget
// and block resolve it identically.
func renderThemePanelMessage(message themePanelMessage, inner int, wrap bool, th theme.Theme, colourless bool) string {
	text, token := themePanelMessageContent(message, inner, th)
	if text == "" {
		return ""
	}
	return headerStyle(token, th, colourless).Render(themePanelMessageText(text, inner, wrap))
}

// The failed-commit line deliberately takes no bg.attention band — too heavy
// inside a 24–30 column panel.
func themePanelMessageContent(message themePanelMessage, inner int, th theme.Theme) (text string, token theme.Token) {
	switch message.Kind {
	case themeMessageConfirm:
		return themePanelConfirmText(message.Slug, inner), th.TextSecondary
	case themeMessageCommitFailed:
		return themePanelCommitFailedMessage, th.AccentAttention
	}
	return "", theme.Token{}
}

// The slug is truncated ahead of the tail, so `?  y / n` — the tail the user must
// answer — survives wherever the composed line fits. At the height floor
// themePanelMessageText truncates the composed line itself, so the tail can go
// with it; the substituted themePanelConfirmKeymap footer is what names the
// answers there.
func themePanelConfirmText(slug string, inner int) string {
	return fmt.Sprintf(themePanelConfirmFormat, ansi.Truncate(slug, themePanelConfirmSlugBudget(inner), themeRowEllipsis))
}

func themePanelConfirmSlugBudget(inner int) int {
	return max(inner-themePanelConfirmFixedWidth(), themeRowLabelFloor)
}

// Derived from the copy so a reworded confirm cannot leave a stale literal.
func themePanelConfirmFixedWidth() int {
	return lipgloss.Width(fmt.Sprintf(themePanelConfirmFormat, ""))
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
		themePanelKeymap(),
		p.union.DirUnusable,
	)
}
