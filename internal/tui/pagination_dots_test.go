package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func newMultiPageSessionModel(t *testing.T, w, h int, appearance theme.Member, colourless bool) Model {
	t.Helper()
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}
	m := Build(Deps{Lister: fakeLister{}, Theme: testConstantFor(t, appearance), NoColor: colourless})
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions, nil)
	if m.sessionList.Paginator.TotalPages < 2 {
		t.Fatalf("test setup: want a multi-page list, got TotalPages=%d", m.sessionList.Paginator.TotalPages)
	}
	return m
}

func dotRowLine(t *testing.T, view string) (string, int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		vis := strings.TrimSpace(ansi.Strip(line))
		if vis == "" {
			continue
		}
		if strings.Trim(vis, paginationDotGlyph) == "" {
			return line, i
		}
	}
	t.Fatalf("no pagination dot row found in view:\n%s", view)
	return "", -1
}

func TestSessionsPaginationDots_ActiveVioletInactiveFaint(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		m := newMultiPageSessionModel(t, 120, 24, appearanceForTheme(t, th), false)
		row, _ := dotRowLine(t, m.viewSessionList())

		if seq := tokenFgSeq(t, th.AccentPrimary); !strings.Contains(row, seq) {
			t.Errorf("dot row missing active-dot accent.primary role sequence %q:\n%q", seq, row)
		}
		if seq := tokenFgSeq(t, th.TextFaint); !strings.Contains(row, seq) {
			t.Errorf("dot row missing inactive-dot text.faint role sequence %q:\n%q", seq, row)
		}
	})
}

func TestSessionsPaginationDots_ActiveDotIsViolet(t *testing.T) {
	m := newMultiPageSessionModel(t, 120, 24, theme.MemberDark, false)
	row, _ := dotRowLine(t, m.viewSessionList())
	violet := tokenFgSeq(t, testDarkTheme(t).AccentPrimary)

	firstDot := strings.IndexByte(row, paginationDotGlyph[0])
	if firstDot < 0 {
		t.Fatalf("dot row has no dot glyph:\n%q", row)
	}
	prefix := row[:firstDot]
	lastEsc := strings.LastIndex(prefix, "\x1b[")
	if lastEsc < 0 {
		t.Fatalf("no SGR run precedes the first (active) dot:\n%q", row)
	}
	run := row[lastEsc:firstDot]
	if !strings.Contains(run, violet) {
		t.Errorf("the active (page-0) dot's SGR run %q is not accent.primary (%q)", run, violet)
	}
}

func TestSessionsPaginationDots_CentredAboveFooter(t *testing.T) {
	const w, h = 120, 24
	m := newMultiPageSessionModel(t, w, h, theme.MemberDark, false)
	view := m.viewSessionList()
	lines := strings.Split(view, "\n")

	dotRow, dotIdx := dotRowLine(t, view)

	footer := renderSessionsFooter(m.sessionsHelpKeymap(), m.contentWidth(), m.themeState.active, m.colourless)
	footerLines := lipgloss.Height(footer)
	if dotIdx >= len(lines)-footerLines {
		t.Errorf("dot row at line %d is not above the footer (footer occupies the last %d of %d lines)", dotIdx, footerLines, len(lines))
	}

	vis := ansi.Strip(dotRow)
	leading := len(vis) - len(strings.TrimLeft(vis, " "))
	if leading == 0 {
		t.Errorf("dot row is flush-left (no leading pad); want centred across the list width:\n%q", vis)
	}
	trailing := len(vis) - len(strings.TrimRight(vis, " "))
	if diff := leading - trailing; diff < -1 || diff > 1 {
		t.Errorf("dot row not centred: leading pad %d vs trailing pad %d (want within 1):\n%q", leading, trailing, vis)
	}
}

func TestSessionsPaginationDots_SuppressedOnSinglePage(t *testing.T) {
	const w, h = 120, 40
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	m := New(fakeLister{}, WithCanvasMode(theme.MemberDark))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions, nil)
	if m.sessionList.Paginator.TotalPages != 1 {
		t.Fatalf("test setup: want single page, got TotalPages=%d", m.sessionList.Paginator.TotalPages)
	}

	view := m.viewSessionList()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		vis := strings.TrimSpace(ansi.Strip(line))
		if vis == "" {
			continue
		}
		if strings.Trim(vis, paginationDotGlyph) == "" {
			t.Errorf("single-page list must suppress the dot row, found one at line %d:\n%q", i, vis)
		}
	}
}

func TestSessionsPaginationDots_PageCountAndPagingUnchanged(t *testing.T) {
	const w, h = 120, 24
	m := newMultiPageSessionModel(t, w, h, theme.MemberDark, false)

	row, _ := dotRowLine(t, m.viewSessionList())
	gotDots := strings.Count(ansi.Strip(row), paginationDotGlyph)
	if gotDots != m.sessionList.Paginator.TotalPages {
		t.Errorf("rendered %d dots, want %d (one per page, parity with the built-in paginator)", gotDots, m.sessionList.Paginator.TotalPages)
	}

	startPage := m.sessionList.Paginator.Page
	next, _ := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})
	mNext := next.(Model)
	if mNext.sessionList.Paginator.Page != startPage+1 {
		t.Errorf("Ctrl+↓ page = %d, want %d (next page)", mNext.sessionList.Paginator.Page, startPage+1)
	}
	prev, _ := mNext.updateSessionList(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})
	mPrev := prev.(Model)
	if mPrev.sessionList.Paginator.Page != startPage {
		t.Errorf("Ctrl+↑ page = %d, want %d (prev page)", mPrev.sessionList.Paginator.Page, startPage)
	}
}

func TestSessionsPaginationDots_NoFullScreenFrame(t *testing.T) {
	const w, h = 120, 24
	m := newMultiPageSessionModel(t, w, h, theme.MemberDark, false)
	vis := ansi.Strip(m.viewSessionList())
	for _, frameGlyph := range []string{"┌", "┐", "└", "┘", "│", "├", "┤"} {
		if strings.Contains(vis, frameGlyph) {
			t.Errorf("composed view contains box-frame glyph %q — a full-screen frame is forbidden:\n%s", frameGlyph, vis)
		}
	}
}

func TestSessionsPaginationDots_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		m := newMultiPageSessionModel(t, 120, 24, appearanceForTheme(t, th), false)
		row, _ := dotRowLine(t, m.viewSessionList())
		if seq := canvasSeq(t, th); !strings.Contains(row, seq) {
			t.Errorf("dot row does not paint the canvas background sequence %q:\n%q", seq, row)
		}
	}
}

func TestSessionsPaginationDots_ColourlessDropsHueAndCanvas(t *testing.T) {
	m := newMultiPageSessionModel(t, 120, 24, theme.MemberDark, true)
	row, _ := dotRowLine(t, m.viewSessionList())

	if got := strings.Count(ansi.Strip(row), paginationDotGlyph); got != m.sessionList.Paginator.TotalPages {
		t.Errorf("colourless dot row glyph count = %d, want %d", got, m.sessionList.Paginator.TotalPages)
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(row, seq) {
		t.Errorf("colourless dot row still paints the canvas background sequence %q:\n%q", seq, row)
	}
	for _, tok := range []theme.Token{testDarkTheme(t).AccentPrimary, testDarkTheme(t).TextFaint} {
		if seq := tokenFgSeq(t, tok); strings.Contains(row, seq) {
			t.Errorf("colourless dot row still emits a foreground role sequence %q:\n%q", seq, row)
		}
	}
}
