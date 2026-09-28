package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func nthLine(t *testing.T, frame string, i int) string {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if i >= len(lines) {
		t.Fatalf("frame has %d lines, wanted line %d", len(lines), i)
	}
	return lines[i]
}

func rowIsBlankGutter(t *testing.T, line string) bool {
	t.Helper()
	stripped := stripSGRForTest(line)
	return strings.TrimSpace(stripped) == ""
}

func leadingGutterCells(t *testing.T, line string) int {
	t.Helper()
	stripped := stripSGRForTest(line)
	n := 0
	for _, r := range stripped {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

func stripSGRForTest(s string) string {
	var b strings.Builder
	src := []rune(s)
	for i := 0; i < len(src); i++ {
		if src[i] == '\x1b' {
			for i < len(src) && src[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteRune(src[i])
	}
	return b.String()
}

func TestContentInset_AppliedToSessions(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)

	view := m.View().Content
	lines := strings.Split(view, "\n")

	if len(lines) != h {
		t.Fatalf("frame height = %d, want %d", len(lines), h)
	}

	for i := range Vinset {
		if !rowIsBlankGutter(t, lines[i]) {
			t.Errorf("top gutter row %d is not blank: %q", i, stripSGRForTest(lines[i]))
		}
	}
	for i := h - Vinset; i < h; i++ {
		if !rowIsBlankGutter(t, lines[i]) {
			t.Errorf("bottom gutter row %d is not blank: %q", i, stripSGRForTest(lines[i]))
		}
	}
	firstContent := lines[Vinset]
	if got := leadingGutterCells(t, firstContent); got != Hinset {
		t.Errorf("first content row leading gutter = %d cells, want %d (content not flush): %q",
			got, Hinset, stripSGRForTest(firstContent))
	}
	if !strings.Contains(stripSGRForTest(firstContent), "P O R T A L") {
		t.Errorf("first content row missing wordmark: %q", stripSGRForTest(firstContent))
	}
}

func TestContentInset_FrameDimensionsUnchanged(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)

	view := m.View().Content
	if got := lipgloss.Height(view); got != h {
		t.Errorf("frame height = %d, want %d (outer fill owns full terminal)", got, h)
	}
	for i, line := range strings.Split(view, "\n") {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("line %d width = %d, want %d (outer fill owns full terminal)", i, lw, w)
		}
	}
}

func TestContentInset_GutterPaintedCanvas(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)

	view := m.View().Content
	topGutter := nthLine(t, view, 0)
	if seq := canvasSeq(t, testDarkTheme(t)); !strings.Contains(topGutter, seq) {
		t.Errorf("top gutter row does not carry the canvas SGR %q: %q", seq, topGutter)
	}
}

func TestContentInset_NoColorGutterNativeBg(t *testing.T) {
	const w, h = 90, 24
	m := colourlessTestModel(t, w, h)

	view := m.View().Content

	if got := lipgloss.Height(view); got != h {
		t.Errorf("colourless frame height = %d, want %d", got, h)
	}
	for i, line := range strings.Split(view, "\n") {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("colourless line %d width = %d, want %d", i, lw, w)
		}
	}
	if frameHasAnyBackgroundSGR(t, view) {
		t.Errorf("colourless frame activates a background SGR; want native bg gutter")
	}
	lines := strings.Split(view, "\n")
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("colourless top gutter row is not blank: %q", lines[0])
	}
	if got := leadingGutterCells(t, lines[Vinset]); got != Hinset {
		t.Errorf("colourless first content row leading gutter = %d, want %d", got, Hinset)
	}
}

func TestContentInset_FoldedIntoBudgets(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)

	if got, want := m.sessionList.Width(), m.contentWidth(); got != want {
		t.Errorf("session list width = %d, want content width %d (Hinset folded in)", got, want)
	}
	if m.sessionList.Height() >= m.contentHeight() {
		t.Errorf("session list height %d >= content height %d; header/footer band not reserved",
			m.sessionList.Height(), m.contentHeight())
	}
	if got, want := m.contentWidth(), w-2*Hinset; got != want {
		t.Errorf("contentWidth() = %d, want %d", got, want)
	}
	if got, want := m.contentHeight(), h-2*Vinset; got != want {
		t.Errorf("contentHeight() = %d, want %d", got, want)
	}
}

func TestContentInset_PaginationInvariantPreserved(t *testing.T) {
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
	for i, line := range lines {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("line %d width = %d, want %d", i, lw, w)
		}
	}
	if !rowIsBlankGutter(t, lines[0]) {
		t.Errorf("top gutter row not blank under pagination: %q", stripSGRForTest(lines[0]))
	}
	if !rowIsBlankGutter(t, lines[h-1]) {
		t.Errorf("bottom gutter row not blank under pagination: %q", stripSGRForTest(lines[h-1]))
	}
	if !strings.Contains(stripSGRForTest(view), "Sessions") {
		t.Errorf("title 'Sessions' not visible under pagination")
	}
}

func TestContentInset_GroupedPaginationInvariant(t *testing.T) {
	const w, h = 90, 16

	var sessions []tmux.Session
	for i := range 30 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}
	m := New(fakeLister{}, WithCanvasMode(theme.MemberDark), WithInitialMode(prefs.ModeByProject))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions, nil)

	view := m.View().Content
	if got := lipgloss.Height(view); got != h {
		t.Errorf("grouped frame height = %d, want %d (no overflow)", got, h)
	}
	for i, line := range strings.Split(view, "\n") {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("grouped line %d width = %d, want %d", i, lw, w)
		}
	}
}

func TestContentInset_AppliesOnProjectsLoading(t *testing.T) {
	const w, h = 90, 24

	t.Run("projects", func(t *testing.T) {
		m := newCanvasTestModel(t, w, h, theme.MemberDark)
		m.activePage = PageProjects
		view := m.View().Content
		assertFramedAndInset(t, view, w, h)
	})

	t.Run("loading", func(t *testing.T) {
		m := newCanvasTestModel(t, w, h, theme.MemberDark)
		m.activePage = PageLoading
		view := m.View().Content
		assertFramedAndInset(t, view, w, h)
	})
}

func assertFramedAndInset(t *testing.T, view string, w, h int) {
	t.Helper()
	if got := lipgloss.Height(view); got != h {
		t.Errorf("frame height = %d, want %d", got, h)
	}
	for i, line := range strings.Split(view, "\n") {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("line %d width = %d, want %d", i, lw, w)
		}
	}
	lines := strings.Split(view, "\n")
	for i := range Vinset {
		if !rowIsBlankGutter(t, lines[i]) {
			t.Errorf("top gutter row %d not blank: %q", i, stripSGRForTest(lines[i]))
		}
	}
	for i := h - Vinset; i < h; i++ {
		if !rowIsBlankGutter(t, lines[i]) {
			t.Errorf("bottom gutter row %d not blank: %q", i, stripSGRForTest(lines[i]))
		}
	}
}

func TestContentInset_ClampsAtTinyTerminal(t *testing.T) {
	for _, tc := range []struct{ w, h int }{
		{1, 1},
		{2, 2},
		{3, 2},
		{4, 2},
	} {
		m := newCanvasTestModel(t, tc.w, tc.h, theme.MemberDark)

		if m.contentWidth() < 0 || m.contentHeight() < 0 {
			t.Errorf("[%dx%d] content region negative: w=%d h=%d", tc.w, tc.h, m.contentWidth(), m.contentHeight())
		}
		if got := m.contentWidth(); got != tc.w {
			t.Errorf("[%dx%d] contentWidth = %d, want %d (inset clamped to 0)", tc.w, tc.h, got, tc.w)
		}
		if got := m.contentHeight(); got != tc.h {
			t.Errorf("[%dx%d] contentHeight = %d, want %d (inset clamped to 0)", tc.w, tc.h, got, tc.h)
		}

		view := m.View().Content
		if got := lipgloss.Height(view); got != tc.h {
			t.Errorf("[%dx%d] frame height = %d, want %d (clamp, no overflow)", tc.w, tc.h, got, tc.h)
		}
	}
}

func TestInsetRegion_ClampBoundary(t *testing.T) {
	for _, tc := range []struct {
		dim, inset, want int
	}{
		{90, 2, 86},
		{5, 2, 1},
		{4, 2, 4},
		{3, 2, 3},
		{0, 2, 0},
		{24, 1, 22},
		{2, 1, 2},
		{1, 1, 1},
	} {
		if got := insetRegion(tc.dim, tc.inset); got != tc.want {
			t.Errorf("insetRegion(%d, %d) = %d, want %d", tc.dim, tc.inset, got, tc.want)
		}
		if insetRegion(tc.dim, tc.inset) < 0 {
			t.Errorf("insetRegion(%d, %d) is negative", tc.dim, tc.inset)
		}
	}
}

func TestContentInset_ClampHoldsWhereContentFits(t *testing.T) {
	for _, tc := range []struct{ w, h int }{
		{40, 10},
		{60, 12},
		{50, 8},
	} {
		m := newCanvasTestModel(t, tc.w, tc.h, theme.MemberDark)
		view := m.View().Content
		if got := lipgloss.Height(view); got != tc.h {
			t.Errorf("[%dx%d] frame height = %d, want %d", tc.w, tc.h, got, tc.h)
		}
		for i, line := range strings.Split(view, "\n") {
			if lw := lipgloss.Width(line); lw != tc.w {
				t.Errorf("[%dx%d] line %d width = %d, want %d (clean rectangle)", tc.w, tc.h, i, lw, tc.w)
			}
		}
		if m.contentWidth() != tc.w-2*Hinset {
			t.Errorf("[%dx%d] contentWidth = %d, want %d (inset applied)", tc.w, tc.h, m.contentWidth(), tc.w-2*Hinset)
		}
	}
}

func TestContentInset_ZeroSizeFallback(t *testing.T) {
	m := newCanvasTestModel(t, 0, 0, theme.MemberDark)

	if got, want := m.contentWidth(), 80-2*Hinset; got != want {
		t.Errorf("zero-size contentWidth() = %d, want %d (80 fallback then inset)", got, want)
	}
	if got, want := m.contentHeight(), 24-2*Vinset; got != want {
		t.Errorf("zero-size contentHeight() = %d, want %d (24 fallback then inset)", got, want)
	}
	view := m.View().Content
	if got := lipgloss.Height(view); got != 24 {
		t.Errorf("zero-size frame height = %d, want 24", got)
	}
	for i, line := range strings.Split(view, "\n") {
		if lw := lipgloss.Width(line); lw != 80 {
			t.Errorf("zero-size line %d width = %d, want 80", i, lw)
		}
	}
}

func TestContentInset_NavigationUnchanged(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)

	before, ok := m.selectedSessionItem()
	if !ok {
		t.Fatalf("no initial selection")
	}

	moved, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mm, ok := moved.(Model)
	if !ok {
		t.Fatalf("Update did not return a Model")
	}
	after, ok := mm.selectedSessionItem()
	if !ok {
		t.Fatalf("no selection after moving down")
	}
	if before.Session.Name == after.Session.Name {
		t.Errorf("selection did not change on down key; nav perturbed by inset")
	}
}
