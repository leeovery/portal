package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func TestSessionRow_ShowsTheLabelInPlaceOfTheName(t *testing.T) {
	items := flatItems(Session{Name: "dotfiles-PpaTER", Label: "user search", Answered: true})
	vis := ansi.Strip(renderRow(SessionDelegate{Theme: testDarkTheme(t)}, 80, items, 0, 0))

	if !strings.Contains(vis, "user search") {
		t.Errorf("row must show the label: %q", vis)
	}
	if strings.Contains(vis, "dotfiles-PpaTER") {
		t.Errorf("a labelled row must not show the name: %q", vis)
	}
}

func TestSessionRow_ShowsTheNameWhenTheLabelIsEmpty(t *testing.T) {
	items := flatItems(Session{Name: "dotfiles-PpaTER", Answered: true})
	vis := ansi.Strip(renderRow(SessionDelegate{Theme: testDarkTheme(t)}, 80, items, 0, 0))

	if !strings.Contains(vis, "dotfiles-PpaTER") {
		t.Errorf("an unlabelled row must show the name: %q", vis)
	}
}

func TestSessionRow_KeysItsMarksOnTheNameNotTheLabel(t *testing.T) {
	items := flatItems(Session{Name: "dotfiles-PpaTER", Label: "user search", Answered: true})
	d := SessionDelegate{Theme: testDarkTheme(t), Pending: markedSet("dotfiles-PpaTER")}
	vis := ansi.Strip(renderRow(d, 80, items, 0, 0))

	if !strings.Contains(vis, rowIndicatorGlyph) {
		t.Errorf("a labelled row in the pending set (keyed by name) must carry the dot: %q", vis)
	}
}

func TestSessionRow_NotAnsweringIsDimmedAndBadged(t *testing.T) {
	const w = 80
	items := flatItems(Session{Name: "row-zero"}, Session{Name: "row-one"})
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		d := SessionDelegate{Theme: th}
		for i, tok := range []struct{ fg, bg theme.Token }{
			{th.TextSecondary, th.BgSelection},
			{th.TextMuted, th.Canvas},
		} {
			out := renderRow(d, w, items, i, 0)
			dim := lipgloss.NewStyle().Foreground(tok.fg.Color()).Background(tok.bg.Color())
			name := items[i].(SessionItem).Session.Name

			if want := dim.Render(name); !strings.Contains(out, want) {
				t.Errorf("[%v row=%d] name not dimmed (muted, not bold) %q: %q", themeLabel(th), i, escSeq(want), escSeq(out))
			}
			if want := dim.Render(notAnsweringBadge); !strings.Contains(out, want) {
				t.Errorf("[%v row=%d] badge not in the muted token %q: %q", themeLabel(th), i, escSeq(want), escSeq(out))
			}
			if col, want := visibleColOf(out, notAnsweringBadge), w-lipgloss.Width(notAnsweringBadge); col != want {
				t.Errorf("[%v row=%d] badge at col %d, want hard right at %d", themeLabel(th), i, col, want)
			}
			if got := lipgloss.Width(out); got != w {
				t.Errorf("[%v row=%d] row width = %d, want %d", themeLabel(th), i, got, w)
			}
		}
	}
}

func TestSessionRow_NotAnsweringCarriesNoIndicators(t *testing.T) {
	items := flatItems(Session{Name: "alpha", Attached: true})
	for _, colourless := range []bool{false, true} {
		d := SessionDelegate{Theme: testDarkTheme(t), Colourless: colourless, Pending: markedSet("alpha")}
		vis := ansi.Strip(renderRow(d, 80, items, 0, 0))

		if !strings.HasSuffix(vis, " "+notAnsweringBadge) {
			t.Errorf("[col=%v] badge must own the trailing region: %q", colourless, vis)
		}
		if strings.Contains(vis, rowIndicatorGlyph) || strings.Contains(vis, " "+attachedIndicatorLetter+" ") || strings.Contains(vis, pendingIndicatorLetter+" ") {
			t.Errorf("[col=%v] a session that did not answer carries an indicator: %q", colourless, vis)
		}
	}
}

func TestSessionRow_NotAnsweringStaysTextBackedUnderNoColor(t *testing.T) {
	items := flatItems(Session{Name: "alpha"}, Session{Name: "bravo", Answered: true})
	d := SessionDelegate{Theme: testDarkTheme(t), Colourless: true}

	silent := renderRow(d, 80, items, 0, 1)
	if strings.Contains(silent, "38;2;") || strings.Contains(silent, "48;2;") {
		t.Errorf("colourless row paints a colour: %q", escSeq(silent))
	}
	if !strings.HasSuffix(ansi.Strip(silent), notAnsweringBadge) {
		t.Errorf("colourless row lost its worded badge: %q", ansi.Strip(silent))
	}
	if strings.Contains(silent, "\x1b[1m") {
		t.Errorf("colourless dimmed name must not be bold: %q", escSeq(silent))
	}
	if answered := renderRow(d, 80, items, 1, 1); !strings.Contains(answered, "\x1b[1mbravo") {
		t.Errorf("colourless answered name must stay bold, so the dimming shows beside it: %q", escSeq(answered))
	}
}

func TestSessionRow_GoneBadgeTakesPrecedenceOverNotAnswering(t *testing.T) {
	items := flatItems(Session{Name: "alpha"})
	d := SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, GoneFlagged: markedSet("alpha")}
	vis := ansi.Strip(renderRow(d, 80, items, 0, 0))

	if !strings.HasSuffix(vis, goneBadge) || strings.Contains(vis, notAnsweringBadge) {
		t.Errorf("a gone row must carry the gone badge alone: %q", vis)
	}
}
