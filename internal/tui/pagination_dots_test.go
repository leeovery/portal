package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// appearanceForMode pins the persisted appearance that resolves to the given
// canvas mode, so Build paints that mode from frame one (no OSC 11 detection).
func appearanceForMode(mode theme.Mode) prefs.Appearance {
	if mode == theme.Light {
		return prefs.AppearanceLight
	}
	return prefs.AppearanceDark
}

// newMultiPageSessionModel builds a production-shaped Sessions model with enough
// deterministic sessions to span >1 page at the given terminal size, so the
// height-driven paginator renders the dot row. The session set is built through
// the production applySessions path so pagination is sized exactly as in prod.
func newMultiPageSessionModel(t *testing.T, w, h int, mode theme.Mode, colourless bool) Model {
	t.Helper()
	var sessions []tmux.Session
	for i := 0; i < 60; i++ {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}
	m := Build(Deps{Lister: fakeLister{}, Appearance: appearanceForMode(mode), NoColor: colourless})
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions)
	if m.sessionList.Paginator.TotalPages < 2 {
		t.Fatalf("test setup: want a multi-page list, got TotalPages=%d", m.sessionList.Paginator.TotalPages)
	}
	return m
}

// dotRowLine locates the rendered pagination dot row in the composed Sessions
// view: the single line whose visible (ANSI-stripped) content is exactly the run
// of dot glyphs (one per page), ignoring surrounding whitespace. Returns the raw
// (styled) line and its index, or fails if no dot row is present.
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

// TestSessionsPaginationDots_ActiveVioletInactiveFaint asserts the §3.5/§2.9
// requirement: the active page dot renders in accent.violet and the inactive
// dots in text.faint — the exact mode-resolved foreground SGR for each token.
func TestSessionsPaginationDots_ActiveVioletInactiveFaint(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMultiPageSessionModel(t, 120, 24, tc.mode, false)
			row, _ := dotRowLine(t, m.viewSessionList())

			// Active dot: accent.violet foreground role sequence present.
			if seq := tokenFgSeq(t, theme.MV.AccentViolet, tc.mode); !strings.Contains(row, seq) {
				t.Errorf("dot row missing active-dot accent.violet role sequence %q:\n%q", seq, row)
			}
			// Inactive dots: text.faint foreground role sequence present.
			if seq := tokenFgSeq(t, theme.MV.TextFaint, tc.mode); !strings.Contains(row, seq) {
				t.Errorf("dot row missing inactive-dot text.faint role sequence %q:\n%q", seq, row)
			}
		})
	}
}

// TestSessionsPaginationDots_ActiveDotIsViolet pins the active page dot
// specifically to accent.violet: the SGR run opening the active (current-page)
// dot glyph carries the violet foreground. On page 0 the FIRST dot is active, so
// the run preceding the first dot glyph must be accent.violet — distinct from the
// text.faint used by the inactive dots that follow.
func TestSessionsPaginationDots_ActiveDotIsViolet(t *testing.T) {
	m := newMultiPageSessionModel(t, 120, 24, theme.Dark, false)
	row, _ := dotRowLine(t, m.viewSessionList())
	violet := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark)

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
		t.Errorf("the active (page-0) dot's SGR run %q is not accent.violet (%q)", run, violet)
	}
}

// TestSessionsPaginationDots_CentredAboveFooter asserts the dot row sits between
// the list body and the condensed footer (above the footer per §3.5) and is
// centred across the list width — its visible glyph run is offset from the left
// edge by a non-trivial leading pad, not flush-left.
func TestSessionsPaginationDots_CentredAboveFooter(t *testing.T) {
	const w, h = 120, 24
	m := newMultiPageSessionModel(t, w, h, theme.Dark, false)
	view := m.viewSessionList()
	lines := strings.Split(view, "\n")

	dotRow, dotIdx := dotRowLine(t, view)

	// The dot row precedes the footer: the footer's 1px top rule + key row are the
	// LAST two lines, so the dot row index must be strictly before them.
	footer := renderSessionsFooter(m.contentWidth(), m.canvasMode, m.colourless)
	footerLines := lipgloss.Height(footer)
	if dotIdx >= len(lines)-footerLines {
		t.Errorf("dot row at line %d is not above the footer (footer occupies the last %d of %d lines)", dotIdx, footerLines, len(lines))
	}

	// Centred across the list width: the visible glyph run has a non-trivial
	// leading pad (it is not flush-left at column 0). With one dot per page the run
	// is short relative to the width, so a centred row carries a clear left margin.
	vis := ansi.Strip(dotRow)
	leading := len(vis) - len(strings.TrimLeft(vis, " "))
	if leading == 0 {
		t.Errorf("dot row is flush-left (no leading pad); want centred across the list width:\n%q", vis)
	}
	// Centred (not right-aligned): the leading pad and trailing pad are within one
	// cell of each other across the list width.
	trailing := len(vis) - len(strings.TrimRight(vis, " "))
	if diff := leading - trailing; diff < -1 || diff > 1 {
		t.Errorf("dot row not centred: leading pad %d vs trailing pad %d (want within 1):\n%q", leading, trailing, vis)
	}
}

// TestSessionsPaginationDots_SuppressedOnSinglePage asserts the built-in
// suppression behaviour is preserved: a list that fits on a single page renders
// NO dot row (bubbles/list returns "" from paginationView when TotalPages < 2).
func TestSessionsPaginationDots_SuppressedOnSinglePage(t *testing.T) {
	const w, h = 120, 40
	// A small session set that fits comfortably on one page at this size.
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 1},
		{Name: "charlie", Windows: 1},
	}
	m := New(fakeLister{}, WithCanvasMode(theme.Dark))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions)
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

// TestSessionsPaginationDots_PageCountAndPagingUnchanged asserts parity: the
// number of dots equals the built-in paginator's TotalPages, and the paging keys
// (Ctrl+↓/↑) still advance/retreat the page exactly as the engine computes — the
// restyle is glyph styling only, never a count/behaviour change.
func TestSessionsPaginationDots_PageCountAndPagingUnchanged(t *testing.T) {
	const w, h = 120, 24
	m := newMultiPageSessionModel(t, w, h, theme.Dark, false)

	// The dot count equals the paginator's TotalPages (one dot per page).
	row, _ := dotRowLine(t, m.viewSessionList())
	gotDots := strings.Count(ansi.Strip(row), paginationDotGlyph)
	if gotDots != m.sessionList.Paginator.TotalPages {
		t.Errorf("rendered %d dots, want %d (one per page, parity with the built-in paginator)", gotDots, m.sessionList.Paginator.TotalPages)
	}

	// Paging behaviour unchanged: Ctrl+↓ advances the page, Ctrl+↑ retreats it.
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

// TestSessionsPaginationDots_NoFullScreenFrame asserts §3.6: the dots are a
// per-element treatment, not a framed box. No box-drawing border glyph (the four
// corners / the vertical/horizontal box rules) wraps the composed view.
func TestSessionsPaginationDots_NoFullScreenFrame(t *testing.T) {
	const w, h = 120, 24
	m := newMultiPageSessionModel(t, w, h, theme.Dark, false)
	vis := ansi.Strip(m.viewSessionList())
	// Box-drawing corners / sides that a full-screen frame would introduce.
	for _, frameGlyph := range []string{"┌", "┐", "└", "┘", "│", "├", "┤"} {
		if strings.Contains(vis, frameGlyph) {
			t.Errorf("composed view contains box-frame glyph %q — §3.6 forbids a full-screen frame:\n%s", frameGlyph, vis)
		}
	}
}

// TestSessionsPaginationDots_PaintsCanvasNoEdgeBleed asserts the dot row carries
// the owned canvas background (leaf .Background(canvas)) so the centred row's pad
// cells are not a terminal-bg island.
func TestSessionsPaginationDots_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		m := newMultiPageSessionModel(t, 120, 24, mode, false)
		row, _ := dotRowLine(t, m.viewSessionList())
		if seq := canvasSeq(t, mode); !strings.Contains(row, seq) {
			t.Errorf("dot row does not paint the canvas background sequence %q:\n%q", seq, row)
		}
	}
}

// TestSessionsPaginationDots_ColourlessDropsHueAndCanvas asserts the NO_COLOR
// carve-out (§2.5): the dot row carries no canvas background SGR and no
// foreground hue — the dots render on the terminal's native fg/bg, glyphs intact.
func TestSessionsPaginationDots_ColourlessDropsHueAndCanvas(t *testing.T) {
	m := newMultiPageSessionModel(t, 120, 24, theme.Dark, true)
	row, _ := dotRowLine(t, m.viewSessionList())

	// Structure preserved: the dot glyphs still print (one per page).
	if got := strings.Count(ansi.Strip(row), paginationDotGlyph); got != m.sessionList.Paginator.TotalPages {
		t.Errorf("colourless dot row glyph count = %d, want %d", got, m.sessionList.Paginator.TotalPages)
	}
	// No canvas background painted.
	if seq := canvasSeq(t, theme.Dark); strings.Contains(row, seq) {
		t.Errorf("colourless dot row still paints the canvas background sequence %q:\n%q", seq, row)
	}
	// No foreground hue from either dot role.
	for _, tok := range []theme.Token{theme.MV.AccentViolet, theme.MV.TextFaint} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(row, seq) {
			t.Errorf("colourless dot row still emits a foreground role sequence %q:\n%q", seq, row)
		}
	}
}
