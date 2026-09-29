package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

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
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
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

	if got := m.viewport.TotalLineCount(); got < 50 {
		t.Errorf("expected TotalLineCount >= 50 for 50-line content, got %d", got)
	}

	bottomView := m.viewport.View()
	if !strings.Contains(bottomView, "line50") {
		t.Errorf("expected bottom-anchored View() to contain %q, got %q", "line50", bottomView)
	}

	m.viewport.GotoTop()
	topView := m.viewport.View()
	if !strings.Contains(topView, "line1") {
		t.Errorf("expected top-anchored View() to contain %q, got %q", "line1", topView)
	}

	if strings.Contains(bottomView, previewPlaceholder) || strings.Contains(topView, previewPlaceholder) {
		t.Errorf("expected placeholder absent for 50-line content; bottom=%q top=%q", bottomView, topView)
	}
}

func TestPreviewFewerThanN_Exactly999LinesRendersAllNineHundredNinetyNineLines(t *testing.T) {
	m, _ := newFewerThanNModel(t, 999)

	if got := m.viewport.TotalLineCount(); got < 999 {
		t.Errorf("expected TotalLineCount >= 999 for 999-line content, got %d", got)
	}

	bottomView := m.viewport.View()
	if !strings.Contains(bottomView, "line999") {
		t.Errorf("expected bottom-anchored View() to contain %q, got %q", "line999", bottomView)
	}

	m.viewport.GotoTop()
	topView := m.viewport.View()
	if !strings.Contains(topView, "line1") {
		t.Errorf("expected top-anchored View() to contain %q, got %q", "line1", topView)
	}

	if strings.Contains(bottomView, previewPlaceholder) || strings.Contains(topView, previewPlaceholder) {
		t.Errorf("expected placeholder absent for 999-line content; bottom=%q top=%q", bottomView, topView)
	}
}

func TestPreviewFewerThanN_ViewportOpensAtScrollTailNotScrollTopForFewerThanNContent(t *testing.T) {
	m, _ := newFewerThanNModel(t, 50)

	if !m.viewport.AtBottom() {
		t.Errorf("expected viewport.AtBottom()=true immediately after construction (scroll-tail), got false (YOffset=%d)", m.viewport.YOffset())
	}
}

func TestPreviewFewerThanN_ScrollUpAtTopBoundaryIsSilentNoOp(t *testing.T) {
	m, _ := newFewerThanNModel(t, 50)

	m.viewport.GotoTop()
	if m.viewport.YOffset() != 0 {
		t.Fatalf("setup: expected YOffset=0 at top, got %d", m.viewport.YOffset())
	}
	beforeView := m.viewport.View()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	if updated.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset still 0 after Up at top, got %d", updated.viewport.YOffset())
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
	cases := []int{1, 2, 50, 500, 999}

	for _, lineCount := range cases {
		t.Run("lines="+strconv.Itoa(lineCount), func(t *testing.T) {
			m, _ := newFewerThanNModel(t, lineCount)

			bottomView := m.viewport.View()
			if strings.Contains(bottomView, previewPlaceholder) {
				t.Errorf("placeholder appeared in bottom view for %d-line content: %q", lineCount, bottomView)
			}

			m.viewport.GotoTop()
			topView := m.viewport.View()
			if strings.Contains(topView, previewPlaceholder) {
				t.Errorf("placeholder appeared in top view for %d-line content: %q", lineCount, topView)
			}

			combined := m.View()
			if strings.Contains(combined, previewPlaceholder) {
				t.Errorf("placeholder appeared in combined View() for %d-line content: %q", lineCount, combined)
			}
		})
	}
}
