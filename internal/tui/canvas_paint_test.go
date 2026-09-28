package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func canvasSeq(t *testing.T, th theme.Theme) string {
	t.Helper()
	return "\x1b[" + sgrParams(t, lipgloss.NewStyle().Background(th.Canvas.Color())) + "m"
}

func TestCanvasMode_DefaultsToDark(t *testing.T) {
	m := New(fakeLister{})
	if m.themeState.inForceMode() != theme.MemberDark {
		t.Errorf("inForceMode() = %v, want testDarkTheme(t) (default)", themeLabel(m.themeState.active))
	}
}

func TestWithCanvasMode(t *testing.T) {
	t.Run("injects Light", func(t *testing.T) {
		m := New(fakeLister{}, WithCanvasMode(theme.MemberLight))
		if m.themeState.inForceMode() != theme.MemberLight {
			t.Errorf("inForceMode() = %v, want testLightTheme(t)", themeLabel(m.themeState.active))
		}
	})

	t.Run("injects Dark explicitly", func(t *testing.T) {
		m := New(fakeLister{}, WithCanvasMode(theme.MemberDark))
		if m.themeState.inForceMode() != theme.MemberDark {
			t.Errorf("inForceMode() = %v, want testDarkTheme(t)", themeLabel(m.themeState.active))
		}
	})
}

func TestOuterFill_PaintsEveryCellTheCanvas(t *testing.T) {
	forEachCanvasMode(t, func(t *testing.T, mode theme.Member) {
		const w, h = 90, 24
		m := newCanvasTestModel(t, w, h, mode)

		view := m.View().Content

		if got := lipgloss.Height(view); got != h {
			t.Errorf("rendered frame height = %d, want exactly %d (filled to termH)", got, h)
		}
		lines := strings.Split(view, "\n")
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw != w {
				t.Errorf("line %d width = %d, want exactly %d (padded to termW, no edge bleed)", i, lw, w)
			}
		}
		if seq := canvasSeq(t, themeForAppearance(t, mode)); !strings.Contains(view, seq) {
			t.Errorf("rendered frame does not contain the canvas background sequence %q", seq)
		}
	})
}

func TestOuterFill_OutsideListHeightBudget(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)

	innerHeight := lipgloss.Height(m.viewSessionList())
	wantPerPage := m.sessionList.Paginator.PerPage

	full := m.View().Content

	if got := m.sessionList.Paginator.PerPage; got != wantPerPage {
		t.Errorf("list PerPage = %d, want %d (fill must not perturb the list budget)", got, wantPerPage)
	}
	if innerHeight > h {
		t.Fatalf("inner view height %d already exceeds termH %d before the fill", innerHeight, h)
	}
	if got := lipgloss.Height(full); got != h {
		t.Errorf("filled frame height = %d, want %d", got, h)
	}
}

func TestOuterFill_RePadsToTermHOnVerticalChange(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)

	baseFrame := lipgloss.Height(m.View().Content)

	const flash = "session \"x\" no longer exists"
	m.setFlash(flash)

	if inner := lipgloss.Height(m.viewSessionList()); inner > h {
		t.Errorf("composed view height with the band = %d, want <= %d (list must shrink underneath the fill)", inner, h)
	}
	bandFrame := lipgloss.Height(m.View().Content)
	if baseFrame != h || bandFrame != h {
		t.Errorf("filled frame height changed with the band: base=%d band=%d, want both %d", baseFrame, bandFrame, h)
	}
	if !strings.Contains(m.View().Content, flash) {
		t.Errorf("band text %q not present in the filled frame", flash)
	}
}

func TestOuterFill_PaginationInvariantPreserved(t *testing.T) {
	const w, h = 90, 14

	var sessions []tmux.Session
	for i := range 40 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}
	m := New(fakeLister{}, WithCanvasMode(theme.MemberDark))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions, nil)

	view := m.View().Content
	if got := lipgloss.Height(view); got != h {
		t.Errorf("frame height = %d, want exactly %d (no overflow)", got, h)
	}
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Sessions") {
		t.Errorf("title 'Sessions' not visible in the rendered frame under pagination")
	}
}

func TestOuterFill_ZeroSizeFallback(t *testing.T) {
	m := newCanvasTestModel(t, 0, 0, theme.MemberDark)

	view := m.View().Content

	if got := lipgloss.Height(view); got != 24 {
		t.Errorf("zero-size frame height = %d, want 24 fallback", got)
	}
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if lw := lipgloss.Width(line); lw != 80 {
			t.Errorf("zero-size line %d width = %d, want 80 fallback", i, lw)
		}
	}
}

func newCanvasTestModel(t *testing.T, w, h int, appearance theme.Member) Model {
	t.Helper()
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
		{Name: "charlie", Windows: 2, Attached: false},
	}
	m := New(fakeLister{}, WithThemeNomination(testBuiltinPair(t)), WithCanvasMode(appearance))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions, nil)
	return m
}

func nameN(i int) string {
	return "sess-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
}
