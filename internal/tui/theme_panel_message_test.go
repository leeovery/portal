package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// Verbatim, not re-derived from the production constant.
const messageTestFailedCopy = "⚠ couldn't save theme"

func messageTestFailed() themePanelMessage {
	return themePanelMessage{Kind: themeMessageCommitFailed}
}

func messageTestPanel(th theme.Theme, width int, message themePanelMessage) themePanel {
	return newThemePanelFixture(themePanelFixtureOpts{
		th:      th,
		width:   width,
		rows:    themePanelTestRows(12),
		message: message,
	})
}

func messageTestVisible(message themePanelMessage, th theme.Theme) []string {
	block := renderThemePanelMessage(message, th, false)
	rows := strings.Split(ansi.Strip(block), "\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}
	return rows
}

func TestPanelMessage_CommitFailedPinnedCopy(t *testing.T) {
	th := testDarkTheme(t)

	rows := messageTestVisible(messageTestFailed(), th)
	if len(rows) != 1 {
		t.Fatalf("the failed-commit line rendered %d rows, want 1: %q", len(rows), rows)
	}
	if got := rows[0]; got != messageTestFailedCopy {
		t.Errorf("the failed-commit line reads %q, want %q", got, messageTestFailedCopy)
	}
	if !strings.HasPrefix(rows[0], flashWarningGlyph) {
		t.Errorf("the failed-commit line reads %q, want it glyph-backed with %q", rows[0], flashWarningGlyph)
	}
}

func TestPanelMessage_ClearingEmptiesTheSlot(t *testing.T) {
	th := testDarkTheme(t)
	m := newArrowPanelModel(t, arrowValidRows(t, 4), arrowValidRows(t, 4)[0].Slug)

	(&m).raiseThemePanelCommitFailed()
	if got := strings.Join(messageTestVisible(m.themePanel.message, th), " "); got != messageTestFailedCopy {
		t.Fatalf("the raised slot renders %q, want %q", got, messageTestFailedCopy)
	}

	(&m).clearThemePanelMessage()
	if got := m.themePanel.message; got != (themePanelMessage{}) {
		t.Errorf("the cleared slot holds %+v, want the zero value", got)
	}
	if got := renderThemePanelMessage(m.themePanel.message, th, false); got != "" {
		t.Errorf("the cleared slot rendered %q, want nothing", got)
	}
}

// The slot draws its copy as authored, so copy wider than the panel's narrowest
// inner width would overflow the frame rather than wrap or truncate.
func TestPanelMessage_CopyFitsTheMinimumInnerWidth(t *testing.T) {
	inner := themePanelInnerWidth(themePanelMinWidth)
	block := renderThemePanelMessage(messageTestFailed(), testDarkTheme(t), false)

	if got := lipgloss.Height(block); got != 1 {
		t.Errorf("the failed-commit line renders on %d rows, want 1", got)
	}
	if got := lipgloss.Width(block); got > inner {
		t.Errorf("the failed-commit line is %d cells, past the %d-cell minimum inner width: %q", got, inner, ansi.Strip(block))
	}
}

func TestPanelMessage_UnreservedWhenEmpty(t *testing.T) {
	th := testDarkTheme(t)
	const height = 16

	if got := renderThemePanelMessage(themePanelMessage{}, th, false); got != "" {
		t.Errorf("themeMessageNone rendered %q, want nothing", got)
	}
	if got := themePanelMessageHeight(themePanelMessage{}); got != 0 {
		t.Errorf("themeMessageNone reserved %d rows, want 0", got)
	}

	// The footer is the same either way, so the slot's row is what separates the
	// two layouts.
	_, empty := themePanelListSize(messageTestPanel(th, themePanelPreferredWidth, themePanelMessage{}), height)
	_, live := themePanelListSize(messageTestPanel(th, themePanelPreferredWidth, messageTestFailed()), height)
	if live != empty-1 {
		t.Errorf("the list body is %d rows with a message and %d without, want exactly one fewer", live, empty)
	}
}

func TestPanelMessage_CommitFailedTokens(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		row := renderThemePanelMessage(messageTestFailed(), th, false)

		if got := themeRowRunAfter(t, row, tokenFgSeq(t, th.AccentAttention)); got != messageTestFailedCopy {
			t.Errorf("[%v] the accent.attention run painted %q, want the glyph AND the text (%q)", themeLabel(th), got, messageTestFailedCopy)
		}
		requireNoBand(t, th, row)
	}
}

// bg.attention specifically: the token the failed-commit line would reach for.
func requireNoBand(t *testing.T, th theme.Theme, row string) {
	t.Helper()
	if strings.Contains(row, tokenBgSeq(t, th.BgAttention)) {
		t.Errorf("[%v] the message slot paints a bg.attention band: %q", themeLabel(th), escSeq(row))
	}
	for _, cell := range scanCellBackgrounds(row) {
		if cell.set && cell.params != strings.TrimSuffix(strings.TrimPrefix(canvasSeq(t, th), "\x1b["), "m") {
			t.Errorf("[%v] the message slot paints the background %q, want the canvas alone", themeLabel(th), cell.params)
		}
	}
}

// The panel is blocked under NO_COLOR outright, so this is defence, not the
// daily path.
func TestPanelMessage_Colourless(t *testing.T) {
	th := testDarkTheme(t)

	for _, tc := range []struct {
		name    string
		message themePanelMessage
		want    string
	}{
		{"failed commit", messageTestFailed(), messageTestFailedCopy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			block := renderThemePanelMessage(tc.message, th, true)
			if frameHasAnyBackgroundSGR(t, block) {
				t.Errorf("the colourless slot activates a background SGR: %q", escSeq(block))
			}
			if frameHasAnyForegroundSGR(t, block) {
				t.Errorf("the colourless slot activates a foreground SGR: %q", escSeq(block))
			}
			if got := strings.TrimRight(ansi.Strip(block), " "); got != tc.want {
				t.Errorf("the colourless slot reads %q, want %q — the glyphs carry the state", got, tc.want)
			}
		})
	}
}
