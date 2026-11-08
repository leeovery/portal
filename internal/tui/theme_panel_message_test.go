package tui

import (
	"slices"
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

func messageTestVisible(message themePanelMessage, inner int, wrap bool, th theme.Theme) []string {
	block := renderThemePanelMessage(message, inner, wrap, th, false)
	rows := strings.Split(ansi.Strip(block), "\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, " ")
	}
	return rows
}

func TestPanelMessage_CommitFailedPinnedCopy(t *testing.T) {
	th := testDarkTheme(t)
	inner := themePanelInnerWidth(themePanelPreferredWidth)

	rows := messageTestVisible(messageTestFailed(), inner, false, th)
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
	inner := themePanelInnerWidth(themePanelPreferredWidth)
	m := newArrowPanelModel(t, arrowValidRows(t, 4), arrowValidRows(t, 4)[0].Slug)

	(&m).raiseThemePanelCommitFailed()
	if got := strings.Join(messageTestVisible(m.themePanel.message, inner, false, th), " "); got != messageTestFailedCopy {
		t.Fatalf("the raised slot renders %q, want %q", got, messageTestFailedCopy)
	}

	(&m).clearThemePanelMessage()
	if got := m.themePanel.message; got != (themePanelMessage{}) {
		t.Errorf("the cleared slot holds %+v, want the zero value", got)
	}
	if got := renderThemePanelMessage(m.themePanel.message, inner, false, th, false); got != "" {
		t.Errorf("the cleared slot rendered %q, want nothing", got)
	}
}

// The shipped copy fits the minimum inner width, so the slot's own degradation is
// out of the panel's reach today; the wrap threshold is what decides it for a
// message that one day does not fit.
func TestPanelMessage_WrapsOnlyAboveTheFloor(t *testing.T) {
	th := testDarkTheme(t)
	floor := themePanelMinHeight(false)
	p := messageTestPanel(th, themePanelMinWidth, messageTestFailed())

	if themePanelMessageWraps(p, floor) {
		t.Error("the slot wraps at the floor; it truncates there")
	}
	if !themePanelMessageWraps(p, floor+1) {
		t.Error("the slot truncates one row above the floor; it wraps there")
	}
	if got, inner := lipgloss.Width(messageTestFailedCopy), themePanelInnerWidth(themePanelMinWidth); got > inner {
		t.Errorf("the failed-commit copy is %d cells, past the %d-cell minimum inner width", got, inner)
	}
}

func TestPanelMessage_UnreservedWhenEmpty(t *testing.T) {
	th := testDarkTheme(t)
	inner := themePanelInnerWidth(themePanelPreferredWidth)
	const height = 16

	for _, wrap := range []bool{false, true} {
		if got := renderThemePanelMessage(themePanelMessage{}, inner, wrap, th, false); got != "" {
			t.Errorf("themeMessageNone rendered %q with wrap=%v, want nothing", got, wrap)
		}
		if got := themePanelMessageHeight(themePanelMessage{}, inner, wrap); got != 0 {
			t.Errorf("themeMessageNone reserved %d rows with wrap=%v, want 0", got, wrap)
		}
	}

	// The failed-commit contender leaves the standing footer in place, so the slot's
	// row is what separates the two layouts.
	_, empty := themePanelListSize(messageTestPanel(th, themePanelPreferredWidth, themePanelMessage{}), height)
	_, live := themePanelListSize(messageTestPanel(th, themePanelPreferredWidth, messageTestFailed()), height)
	if live != empty-1 {
		t.Errorf("the list body is %d rows with a message and %d without, want exactly one fewer", live, empty)
	}
}

func TestPanelMessage_CommitFailedTokens(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		inner := themePanelInnerWidth(themePanelPreferredWidth)
		row := renderThemePanelMessage(messageTestFailed(), inner, false, th, false)

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
	inner := themePanelInnerWidth(themePanelPreferredWidth)

	for _, tc := range []struct {
		name    string
		message themePanelMessage
		want    string
	}{
		{"failed commit", messageTestFailed(), messageTestFailedCopy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			block := renderThemePanelMessage(tc.message, inner, false, th, true)
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

// A drop-in slug the wrap over-packs: it stands in for the first panel message
// to carry a slug the copy does not budget for.
const messageTestOverPackingCopy = flashWarningGlyph + " gruvbox_material_dark-hard"

const (
	messageTestMinInner = 20
	messageTestMaxInner = 47
)

func messageTestRows(message string, inner int, wrap bool) []string {
	return strings.Split(themePanelMessageText(message, inner, wrap), "\n")
}

func TestPanelMessage_RowsFitTheInnerWidth(t *testing.T) {
	t.Run("it renders no panel message row wider than the inner width", func(t *testing.T) {
		for inner := messageTestMinInner; inner <= messageTestMaxInner; inner++ {
			rows := messageTestRows(messageTestOverPackingCopy, inner, true)
			for i, row := range rows {
				if got := lipgloss.Width(row); got > inner {
					t.Errorf("at an inner width of %d row %d renders at %d cells: %q", inner, i, got, row)
				}
			}
			want := 1
			if lipgloss.Width(messageTestOverPackingCopy) > inner {
				want = themePanelMessageWrapRows
			}
			if len(rows) != want {
				t.Errorf("at an inner width of %d the slot costs %d rows, want %d: %q", inner, len(rows), want, rows)
			}
		}
	})

	t.Run("it leaves today's shipped copy reading the same", func(t *testing.T) {
		for _, tc := range []struct{ name, message string }{
			{"the failed commit", messageTestFailedCopy},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for inner := messageTestMinInner; inner <= messageTestMaxInner; inner++ {
					want := bareWrapRows(tc.message, inner)
					if len(want) > themePanelMessageWrapRows {
						t.Fatalf("fixture: at an inner width of %d the copy wraps to %d rows, past the cap of %d — it no longer stands for copy the cap leaves alone",
							inner, len(want), themePanelMessageWrapRows)
					}
					if got := messageTestRows(tc.message, inner, true); !slices.Equal(got, want) {
						t.Errorf("at an inner width of %d the copy reads\n got: %q\nwant: %q", inner, got, want)
					}
				}
			})
		}
	})

	t.Run("it reads the capped row back as the message from where it starts", func(t *testing.T) {
		const message = flashWarningGlyph + " couldn't save theme gruvbox_material-dark-hard-contrast-extended"
		for inner := messageTestMinInner; inner <= messageTestMaxInner; inner++ {
			assertCappedRowReadsSource(t, messageTestRows(message, inner, true), wrappedLines, message, inner, themePanelMessageWrapRows)
		}
	})

	t.Run("it truncates rather than wraps below the wrap threshold", func(t *testing.T) {
		inner := themePanelInnerWidth(themePanelMinWidth)
		if got := messageTestRows(messageTestOverPackingCopy, inner, false); len(got) != 1 || !strings.HasSuffix(got[0], themeRowEllipsis) {
			t.Errorf("with wrap off the slot reads %q, want one row truncated with %q", got, themeRowEllipsis)
		}
		if got := themePanelMessageText(messageTestOverPackingCopy, 0, true); got != ansi.Truncate(messageTestOverPackingCopy, 0, themeRowEllipsis) {
			t.Errorf("at an inner width of 0 the slot reads %q, want the truncation %q", got, ansi.Truncate(messageTestOverPackingCopy, 0, themeRowEllipsis))
		}
	})
}
