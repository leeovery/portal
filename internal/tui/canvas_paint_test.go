package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// canvasSeq is the SGR background sequence for a given mode's canvas token. The
// outer fill and the leaf styles must both paint THIS exact background, so the
// rendered frame is searched for it.
func canvasSeq(t *testing.T, m theme.Mode) string {
	t.Helper()
	probe := lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(m)).Render(" ")
	// Strip the trailing reset + the space so we keep just the opening SGR.
	idx := strings.IndexByte(probe, ' ')
	if idx <= 0 {
		t.Fatalf("could not derive canvas SGR from %q", probe)
	}
	return probe[:idx]
}

// TestCanvasMode_DefaultsToDark pins the injectable-mode seam default. With no
// option applied the resolved canvas mode is Dark (the §2.6 no-answer fallback
// and the zero value of theme.Mode), so an unconfigured model paints the dark
// canvas it was tuned for.
func TestCanvasMode_DefaultsToDark(t *testing.T) {
	m := New(fakeLister{})
	if m.canvasMode != theme.Dark {
		t.Errorf("canvasMode = %v, want theme.Dark (default)", m.canvasMode)
	}
}

// TestWithCanvasMode pins the injectable-mode option: 1-7 swaps the resolved
// mode in here without touching the View() wrap point.
func TestWithCanvasMode(t *testing.T) {
	t.Run("injects Light", func(t *testing.T) {
		m := New(fakeLister{}, WithCanvasMode(theme.Light))
		if m.canvasMode != theme.Light {
			t.Errorf("canvasMode = %v, want theme.Light", m.canvasMode)
		}
	})

	t.Run("injects Dark explicitly", func(t *testing.T) {
		m := New(fakeLister{}, WithCanvasMode(theme.Dark))
		if m.canvasMode != theme.Dark {
			t.Errorf("canvasMode = %v, want theme.Dark", m.canvasMode)
		}
	})
}

// TestOuterFill_PaintsEveryCellTheCanvas asserts the outer fill is the LAST
// layer over the assembled Sessions view: every rendered line is the full
// terminal width and the frame is exactly the terminal height, with the canvas
// background sequence present (no edge bleed, no unpainted rows).
func TestOuterFill_PaintsEveryCellTheCanvas(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const w, h = 90, 24
			m := newCanvasTestModel(t, w, h, tc.mode)

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
			if seq := canvasSeq(t, tc.mode); !strings.Contains(view, seq) {
				t.Errorf("rendered frame does not contain the canvas background sequence %q", seq)
			}
		})
	}
}

// TestOuterFill_OutsideListHeightBudget asserts the outer fill never
// participates in the list's height budget: the list row count (the number of
// session/header rows the paginator renders per page) is identical with and
// without the outer fill applied. The fill is an outer wrap, not per-delegate
// painting.
func TestOuterFill_OutsideListHeightBudget(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

	// The inner composed view (no outer fill) and the list's page item count
	// are the budget the paginator computed. Applying the outer fill must not
	// change either.
	innerHeight := lipgloss.Height(m.viewSessionList())
	wantPerPage := m.sessionList.Paginator.PerPage

	full := m.View().Content

	// The list's per-page budget is untouched by the fill.
	if got := m.sessionList.Paginator.PerPage; got != wantPerPage {
		t.Errorf("list PerPage = %d, want %d (fill must not perturb the list budget)", got, wantPerPage)
	}
	// The inner composed view is <= termH (it always was), and the outer fill
	// pads UP to exactly termH — never by truncating or adding list rows.
	if innerHeight > h {
		t.Fatalf("inner view height %d already exceeds termH %d before the fill", innerHeight, h)
	}
	if got := lipgloss.Height(full); got != h {
		t.Errorf("filled frame height = %d, want %d", got, h)
	}
}

// TestOuterFill_RePadsToTermHOnVerticalChange asserts a dynamic vertical change
// (a notice band appearing) drives the list-height recompute UNDERNEATH the
// fill: the list reserves a row for the band so the composed view stays within
// termH, and the outer fill simply re-pads to exactly termH. The frame height is
// unchanged (always termH), the composed view never overflows, and the band's
// text is present.
func TestOuterFill_RePadsToTermHOnVerticalChange(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

	baseFrame := lipgloss.Height(m.View().Content)

	// A flash band is the §11.2 dynamic vertical change. setFlash inserts one
	// row beneath the title AND re-syncs the list size so the composed view
	// stays within termH (the list height recompute underneath the fill).
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

// TestOuterFill_PaginationInvariantPreserved asserts the one-row-per-delegate
// pagination invariant holds under the fill: with more sessions than fit on a
// page, the rendered frame is exactly termH, every line is exactly termW, and
// the visible content never overflows the viewport (the original
// cursor-invisible / missing-title / left-shift overflow class must not
// regress).
func TestOuterFill_PaginationInvariantPreserved(t *testing.T) {
	const w, h = 90, 14 // deliberately short so pagination kicks in

	var sessions []tmux.Session
	for i := range 40 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}
	m := New(fakeLister{}, WithCanvasMode(theme.Dark))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions)

	view := m.View().Content
	if got := lipgloss.Height(view); got != h {
		t.Errorf("frame height = %d, want exactly %d (no overflow)", got, h)
	}
	// The title row must be visible within the bottom-h window a terminal shows.
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Sessions") {
		t.Errorf("title 'Sessions' not visible in the rendered frame under pagination")
	}
}

// TestOuterFill_ZeroSizeFallback asserts the pre-first-WindowSizeMsg zero-size
// case falls back to exactly 80x24 (matching viewLoading) so the fill never
// sizes to zero and blanks the screen.
func TestOuterFill_ZeroSizeFallback(t *testing.T) {
	m := newCanvasTestModel(t, 0, 0, theme.Dark)

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

// newCanvasTestModel builds a production-shaped Sessions model with the given
// terminal size and canvas mode, loaded with the deterministic flat session set
// through the production applySessions path (SetItems → re-size) so pagination
// is sized exactly as it is in production.
func newCanvasTestModel(t *testing.T, w, h int, mode theme.Mode) Model {
	t.Helper()
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
		{Name: "charlie", Windows: 2, Attached: false},
	}
	m := New(fakeLister{}, WithCanvasMode(mode))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions)
	return m
}

func nameN(i int) string {
	return "sess-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
}
