package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// nthLine returns line i (0-based) of a rendered frame, failing the test if the
// frame has too few lines.
func nthLine(t *testing.T, frame string, i int) string {
	t.Helper()
	lines := strings.Split(frame, "\n")
	if i >= len(lines) {
		t.Fatalf("frame has %d lines, wanted line %d", len(lines), i)
	}
	return lines[i]
}

// rowIsBlankGutter reports whether a frame row is an entirely blank gutter row —
// every cell a space once SGR codes are stripped. Used to assert the Vinset top
// and bottom gutter rows carry no content.
func rowIsBlankGutter(t *testing.T, line string) bool {
	t.Helper()
	stripped := stripSGRForTest(line)
	return strings.TrimSpace(stripped) == ""
}

// leadingGutterCells returns the number of leading blank (space) cells in a
// frame row once SGR codes are stripped — the left gutter width on a content row.
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

// stripSGRForTest removes SGR escape sequences from s, leaving only the printed
// cells, so a test can reason about the visible glyph layout.
func stripSGRForTest(s string) string {
	var b strings.Builder
	src := []rune(s)
	for i := 0; i < len(src); i++ {
		if src[i] == '\x1b' {
			// Skip until the terminating 'm' (SGR) — every escape this renderer
			// emits is a CSI ... m sequence.
			for i < len(src) && src[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteRune(src[i])
	}
	return b.String()
}

// TestContentInset_AppliedToSessions asserts the global content gutter: the
// composed Sessions view is inset by Hinset cells L/R and Vinset rows T/B. The
// top Vinset rows and bottom Vinset rows are blank gutter, and content rows lead
// with exactly Hinset blank cells (content is no longer flush to the edges).
func TestContentInset_AppliedToSessions(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

	view := m.View().Content
	lines := strings.Split(view, "\n")

	if len(lines) != h {
		t.Fatalf("frame height = %d, want %d", len(lines), h)
	}

	// Top Vinset rows are blank gutter.
	for i := range Vinset {
		if !rowIsBlankGutter(t, lines[i]) {
			t.Errorf("top gutter row %d is not blank: %q", i, stripSGRForTest(lines[i]))
		}
	}
	// Bottom Vinset rows are blank gutter.
	for i := h - Vinset; i < h; i++ {
		if !rowIsBlankGutter(t, lines[i]) {
			t.Errorf("bottom gutter row %d is not blank: %q", i, stripSGRForTest(lines[i]))
		}
	}
	// The first content row (just below the top gutter) carries the header
	// wordmark inset by exactly Hinset cells — not flush to the left edge.
	firstContent := lines[Vinset]
	if got := leadingGutterCells(t, firstContent); got != Hinset {
		t.Errorf("first content row leading gutter = %d cells, want %d (content not flush): %q",
			got, Hinset, stripSGRForTest(firstContent))
	}
	// The header wordmark must be present in that row, after the gutter. The
	// full wordmark is letter-spaced ("P O R T A L"), so match the leading caps
	// with the renderer's spacing rather than the bare string.
	if !strings.Contains(stripSGRForTest(firstContent), "P O R T A L") {
		t.Errorf("first content row missing wordmark: %q", stripSGRForTest(firstContent))
	}
}

// TestContentInset_FrameDimensionsUnchanged asserts the inset does not change the
// outer frame dimensions: the §1 outer fill still owns the FULL terminal canvas
// (termW × termH) underneath the inset — every line is exactly termW, the frame
// is exactly termH.
func TestContentInset_FrameDimensionsUnchanged(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

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

// TestContentInset_GutterPaintedCanvas asserts the gutter cells (the top/bottom
// rows and the L/R columns) are painted the owned canvas in the coloured path —
// the canvas SGR is present on the top gutter row.
func TestContentInset_GutterPaintedCanvas(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

	view := m.View().Content
	topGutter := nthLine(t, view, 0)
	if seq := canvasSeq(t, theme.Dark); !strings.Contains(topGutter, seq) {
		t.Errorf("top gutter row does not carry the canvas SGR %q: %q", seq, topGutter)
	}
}

// TestContentInset_NoColorGutterNativeBg asserts that under NO_COLOR the gutter
// is the terminal native bg — no background SGR is ever activated across the
// whole frame (mirroring the colourless fill path).
func TestContentInset_NoColorGutterNativeBg(t *testing.T) {
	const w, h = 90, 24
	m := colourlessTestModel(t, w, h)

	view := m.View().Content

	// Frame geometry is preserved (full terminal) ...
	if got := lipgloss.Height(view); got != h {
		t.Errorf("colourless frame height = %d, want %d", got, h)
	}
	for i, line := range strings.Split(view, "\n") {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("colourless line %d width = %d, want %d", i, lw, w)
		}
	}
	// ... but no background SGR is ever painted (native bg gutter).
	if frameHasAnyBackgroundSGR(t, view) {
		t.Errorf("colourless frame activates a background SGR; want native bg gutter")
	}
	// Content is still inset: top row blank, first content row inset by Hinset.
	lines := strings.Split(view, "\n")
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("colourless top gutter row is not blank: %q", lines[0])
	}
	if got := leadingGutterCells(t, lines[Vinset]); got != Hinset {
		t.Errorf("colourless first content row leading gutter = %d, want %d", got, Hinset)
	}
}

// TestContentInset_FoldedIntoBudgets asserts the inset is folded into BOTH the
// width and height budgets at every SetSize site: the session list is sized to
// the content-region width (termW − 2·Hinset) and a content-region height that
// also subtracts the vertical inset (2·Vinset) on top of the header/footer band.
func TestContentInset_FoldedIntoBudgets(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

	if got, want := m.sessionList.Width(), m.contentWidth(); got != want {
		t.Errorf("session list width = %d, want content width %d (Hinset folded in)", got, want)
	}
	// The list height must be strictly less than the content height by at least
	// the header + footer + vertical inset already removed — i.e. the list never
	// reaches the content-region height.
	if m.sessionList.Height() >= m.contentHeight() {
		t.Errorf("session list height %d >= content height %d; header/footer band not reserved",
			m.sessionList.Height(), m.contentHeight())
	}
	// contentWidth must be exactly termW − 2·Hinset, contentHeight termH − 2·Vinset.
	if got, want := m.contentWidth(), w-2*Hinset; got != want {
		t.Errorf("contentWidth() = %d, want %d", got, want)
	}
	if got, want := m.contentHeight(), h-2*Vinset; got != want {
		t.Errorf("contentHeight() = %d, want %d", got, want)
	}
}

// TestContentInset_PaginationInvariantPreserved asserts the one-row-per-delegate
// invariant holds under the inset at a short terminal where pagination kicks in:
// the frame is exactly termH, every line exactly termW, the title is visible, and
// no content overflows into the gutter rows.
func TestContentInset_PaginationInvariantPreserved(t *testing.T) {
	const w, h = 90, 14

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
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("line %d width = %d, want %d", i, lw, w)
		}
	}
	// Top and bottom gutter rows must be blank — no content bled into them.
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

// TestContentInset_GroupedPaginationInvariant asserts the inset holds the
// invariant in a grouped mode too (By Project): the frame is exactly termH and
// every line exactly termW with grouped headers + rows.
func TestContentInset_GroupedPaginationInvariant(t *testing.T) {
	const w, h = 90, 16

	var sessions []tmux.Session
	for i := range 30 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}
	m := New(fakeLister{}, WithCanvasMode(theme.Dark), WithInitialMode(prefs.ModeByProject))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions)

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

// TestContentInset_AppliesOnProjectsLoading asserts the inset composes on the
// Projects and Loading pages — each renders to exactly termW × termH with the
// top/bottom gutter rows blank, proving the single global wrap covers every page
// (no per-page inset). Preview's inset is covered separately by
// TestModelViewRoutesPagePreviewToPreviewModel.
func TestContentInset_AppliesOnProjectsLoading(t *testing.T) {
	const w, h = 90, 24

	t.Run("projects", func(t *testing.T) {
		m := newCanvasTestModel(t, w, h, theme.Dark)
		m.activePage = PageProjects
		view := m.View().Content
		assertFramedAndInset(t, view, w, h)
	})

	t.Run("loading", func(t *testing.T) {
		m := newCanvasTestModel(t, w, h, theme.Dark)
		m.activePage = PageLoading
		view := m.View().Content
		assertFramedAndInset(t, view, w, h)
	})
}

// assertFramedAndInset asserts a rendered frame is exactly w×h with the top and
// bottom Vinset gutter rows blank — the shared composition check for every page.
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

// TestContentInset_ClampsAtTinyTerminal asserts the inset clamps to 0 when the
// terminal is too small to hold the content region: the content region never
// goes negative, it equals the full terminal dim (inset == 0), and the frame
// height clamps to termH (no vertical overflow). At sizes that intrinsically
// cannot fit the header's minimum width the content can still be wider than the
// terminal — that is the pre-existing §2.7 horizontal-overflow degrade, unchanged
// by this task — so this test asserts the inset contribution clamps to 0, not the
// intrinsic content width.
func TestContentInset_ClampsAtTinyTerminal(t *testing.T) {
	// Each dim clamps the inset to 0 only when it cannot hold 2× the inset:
	// width clamps at w <= 2·Hinset (4), height at h <= 2·Vinset (2).
	for _, tc := range []struct{ w, h int }{
		{1, 1},
		{2, 2}, // h == 2·Vinset → height inset clamps; w == 2 < 4 → width clamps
		{3, 2},
		{4, 2}, // w == 2·Hinset (4) → width inset clamps to 0
	} {
		m := newCanvasTestModel(t, tc.w, tc.h, theme.Dark)

		// The inset clamps to 0: the content region is the full terminal dim,
		// never negative.
		if m.contentWidth() < 0 || m.contentHeight() < 0 {
			t.Errorf("[%dx%d] content region negative: w=%d h=%d", tc.w, tc.h, m.contentWidth(), m.contentHeight())
		}
		if got := m.contentWidth(); got != tc.w {
			t.Errorf("[%dx%d] contentWidth = %d, want %d (inset clamped to 0)", tc.w, tc.h, got, tc.w)
		}
		if got := m.contentHeight(); got != tc.h {
			t.Errorf("[%dx%d] contentHeight = %d, want %d (inset clamped to 0)", tc.w, tc.h, got, tc.h)
		}

		// The frame height clamps to termH (no vertical overflow from the inset).
		view := m.View().Content
		if got := lipgloss.Height(view); got != tc.h {
			t.Errorf("[%dx%d] frame height = %d, want %d (clamp, no overflow)", tc.w, tc.h, got, tc.h)
		}
	}
}

// TestInsetRegion_ClampBoundary pins the pure inset-region derivation: it
// subtracts 2× the inset, clamping to the full dimension (inset 0) exactly when
// the dimension cannot hold 2× the inset, never producing a negative region.
func TestInsetRegion_ClampBoundary(t *testing.T) {
	for _, tc := range []struct {
		dim, inset, want int
	}{
		{90, 2, 86},
		{5, 2, 1},
		{4, 2, 4}, // dim == 2·inset → clamp to full dim
		{3, 2, 3}, // dim < 2·inset → clamp
		{0, 2, 0}, // degenerate → clamp
		{24, 1, 22},
		{2, 1, 2}, // dim == 2·inset → clamp
		{1, 1, 1}, // clamp
	} {
		if got := insetRegion(tc.dim, tc.inset); got != tc.want {
			t.Errorf("insetRegion(%d, %d) = %d, want %d", tc.dim, tc.inset, got, tc.want)
		}
		if insetRegion(tc.dim, tc.inset) < 0 {
			t.Errorf("insetRegion(%d, %d) is negative", tc.dim, tc.inset)
		}
	}
}

// TestContentInset_ClampHoldsWhereContentFits asserts that at small-but-viable
// terminals (large enough to hold the inset and the content) the frame is a clean
// termW × termH rectangle: the inset applies and no line overflows.
func TestContentInset_ClampHoldsWhereContentFits(t *testing.T) {
	for _, tc := range []struct{ w, h int }{
		{40, 10},
		{60, 12},
		{50, 8},
	} {
		m := newCanvasTestModel(t, tc.w, tc.h, theme.Dark)
		view := m.View().Content
		if got := lipgloss.Height(view); got != tc.h {
			t.Errorf("[%dx%d] frame height = %d, want %d", tc.w, tc.h, got, tc.h)
		}
		for i, line := range strings.Split(view, "\n") {
			if lw := lipgloss.Width(line); lw != tc.w {
				t.Errorf("[%dx%d] line %d width = %d, want %d (clean rectangle)", tc.w, tc.h, i, lw, tc.w)
			}
		}
		// Inset is non-zero here.
		if m.contentWidth() != tc.w-2*Hinset {
			t.Errorf("[%dx%d] contentWidth = %d, want %d (inset applied)", tc.w, tc.h, m.contentWidth(), tc.w-2*Hinset)
		}
	}
}

// TestContentInset_ZeroSizeFallback asserts the zero/unset dims fall back to
// 80×24 first, THEN inset — so the content region is (80−2·Hinset)×(24−2·Vinset)
// and the frame is exactly 80×24.
func TestContentInset_ZeroSizeFallback(t *testing.T) {
	m := newCanvasTestModel(t, 0, 0, theme.Dark)

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

// TestContentInset_NavigationUnchanged asserts behaviour parity: the inset is a
// composition-only change. Cursor movement down the session list still advances
// the selection exactly as before — the inset does not perturb nav/selection.
func TestContentInset_NavigationUnchanged(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

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
