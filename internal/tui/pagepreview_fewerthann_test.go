package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// fewer-than-N regression anchors: a successful (bytes, nil) read whose
// payload contains FEWER than N=1000 newline-terminated lines must be
// rendered as content — never as the placeholder. The dispatcher in
// readFocusedPaneIntoViewport already does the right thing; these tests
// pin that contract per § Read-Failure Handling > Placeholder >
// Non-triggering condition and § Acceptance Criteria > Edge cases.

// buildLines returns lineCount newline-terminated lines of the form
// "lineK\n" with K running 1..lineCount, so each line is unique and the
// first/last line can be located deterministically in the rendered
// viewport.
func buildLines(lineCount int) []byte {
	var b strings.Builder
	for i := 1; i <= lineCount; i++ {
		b.WriteString("line")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\n")
	}
	return []byte(b.String())
}

func newFewerThanNModel(t *testing.T, lineCount int) (previewModel, *recordingReader) {
	t.Helper()
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: buildLines(lineCount)}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	return m, reader
}

func TestPreviewFewerThanN_OneLineFileRendersTheSingleLineAndNotThePlaceholder(t *testing.T) {
	m, _ := newFewerThanNModel(t, 1)

	view := m.viewport.View()
	if !strings.Contains(view, "line1") {
		t.Errorf("expected viewport.View() to contain %q, got %q", "line1", view)
	}
	if strings.Contains(view, previewPlaceholder) {
		t.Errorf("expected placeholder absent for 1-line content; got view=%q", view)
	}
	if got := m.viewport.TotalLineCount(); got < 1 {
		t.Errorf("expected TotalLineCount >= 1 for 1-line content, got %d", got)
	}
}

func TestPreviewFewerThanN_FiftyLineFileRendersAllFiftyLines(t *testing.T) {
	m, _ := newFewerThanNModel(t, 50)

	// All 50 distinct line tokens must be addressable in the loaded buffer.
	// View() shows only the visible window — anchor at top, then walk a
	// PgDown/End sweep that visits every line and assert each token shows
	// up at some point. Simpler: TotalLineCount() must reflect 50 loaded
	// lines, and the bottom view (anchor on initial open) must contain the
	// last line.
	if got := m.viewport.TotalLineCount(); got < 50 {
		t.Errorf("expected TotalLineCount >= 50 for 50-line content, got %d", got)
	}

	// Initial open anchored at bottom: the last line must be visible.
	bottomView := m.viewport.View()
	if !strings.Contains(bottomView, "line50") {
		t.Errorf("expected bottom-anchored View() to contain %q, got %q", "line50", bottomView)
	}

	// Scroll to the top and assert the first line is now visible — proves
	// the buffer holds line 1 too.
	m.viewport.GotoTop()
	topView := m.viewport.View()
	if !strings.Contains(topView, "line1") {
		t.Errorf("expected top-anchored View() to contain %q, got %q", "line1", topView)
	}

	// Placeholder absent in both anchored views.
	if strings.Contains(bottomView, previewPlaceholder) || strings.Contains(topView, previewPlaceholder) {
		t.Errorf("expected placeholder absent for 50-line content; bottom=%q top=%q", bottomView, topView)
	}
}

func TestPreviewFewerThanN_Exactly999LinesRendersAllNineHundredNinetyNineLines(t *testing.T) {
	// 999 is one below the N=1000 boundary — the largest fewer-than-N
	// fixture. View() shows only the visible window; assert via
	// TotalLineCount() that all 999 are loaded into the buffer regardless
	// of current scroll position.
	m, _ := newFewerThanNModel(t, 999)

	if got := m.viewport.TotalLineCount(); got < 999 {
		t.Errorf("expected TotalLineCount >= 999 for 999-line content, got %d", got)
	}

	// Initial open anchored at bottom: last line visible.
	bottomView := m.viewport.View()
	if !strings.Contains(bottomView, "line999") {
		t.Errorf("expected bottom-anchored View() to contain %q, got %q", "line999", bottomView)
	}

	// Scroll to the top and assert line 1 surfaces — the buffer is loaded
	// from the very first line.
	m.viewport.GotoTop()
	topView := m.viewport.View()
	if !strings.Contains(topView, "line1") {
		t.Errorf("expected top-anchored View() to contain %q, got %q", "line1", topView)
	}

	// Placeholder absent in both anchored views.
	if strings.Contains(bottomView, previewPlaceholder) || strings.Contains(topView, previewPlaceholder) {
		t.Errorf("expected placeholder absent for 999-line content; bottom=%q top=%q", bottomView, topView)
	}
}

func TestPreviewFewerThanN_ViewportOpensAtScrollTailNotScrollTopForFewerThanNContent(t *testing.T) {
	// Build content with more lines than the viewport height so AtBottom()
	// can only be true if GotoBottom explicitly ran during construction.
	m, _ := newFewerThanNModel(t, 50)

	if !m.viewport.AtBottom() {
		t.Errorf("expected viewport.AtBottom()=true immediately after construction (scroll-tail), got false (YOffset=%d)", m.viewport.YOffset)
	}
}

func TestPreviewFewerThanN_ScrollUpAtTopBoundaryIsSilentNoOp(t *testing.T) {
	// Use 50 lines so the content is taller than the 24-line viewport and
	// scroll up has somewhere to be at the top.
	m, _ := newFewerThanNModel(t, 50)

	m.viewport.GotoTop()
	if m.viewport.YOffset != 0 {
		t.Fatalf("setup: expected YOffset=0 at top, got %d", m.viewport.YOffset)
	}
	beforeView := m.viewport.View()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})

	if updated.viewport.YOffset != 0 {
		t.Errorf("expected YOffset still 0 after Up at top, got %d", updated.viewport.YOffset)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd on silent no-op Up at top, got non-nil")
	}
	afterView := updated.viewport.View()
	if afterView != beforeView {
		t.Errorf("expected viewport content unchanged after Up at top boundary;\nbefore=%q\nafter =%q", beforeView, afterView)
	}
	if strings.Contains(afterView, previewPlaceholder) {
		t.Errorf("expected placeholder absent after Up at top; got view=%q", afterView)
	}
}

func TestPreviewFewerThanN_NeverTriggersThePlaceholderBranch(t *testing.T) {
	// Sweep across the full fewer-than-N range — 1, 2, 50, 500, 999 — and
	// assert the placeholder string is absent from the rendered output of
	// every fixture. This is the regression-anchor test for the
	// "successful partial read = content, not placeholder" contract.
	cases := []int{1, 2, 50, 500, 999}

	for _, lineCount := range cases {
		t.Run("lines="+strconv.Itoa(lineCount), func(t *testing.T) {
			m, _ := newFewerThanNModel(t, lineCount)

			// Anchored at bottom by initial-open: check the bottom view.
			bottomView := m.viewport.View()
			if strings.Contains(bottomView, previewPlaceholder) {
				t.Errorf("placeholder appeared in bottom view for %d-line content: %q", lineCount, bottomView)
			}

			// And at the top, after explicitly scrolling — covers the full
			// content range that View() can possibly surface.
			m.viewport.GotoTop()
			topView := m.viewport.View()
			if strings.Contains(topView, previewPlaceholder) {
				t.Errorf("placeholder appeared in top view for %d-line content: %q", lineCount, topView)
			}

			// Combined View (chrome + viewport) is what the user actually
			// sees — assert there too.
			combined := m.View()
			if strings.Contains(combined, previewPlaceholder) {
				t.Errorf("placeholder appeared in combined View() for %d-line content: %q", lineCount, combined)
			}
		})
	}
}
