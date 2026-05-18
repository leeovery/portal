package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// TestPreviewView_RendersChromeLineAboveViewportContent pins the v1 layout
// orientation: chrome is the first line, viewport content follows. The
// orientation choice (header on top vs footer on bottom) is fixed in 3-6 to
// header-on-top per the spec's chrome-line conventions.
func TestPreviewView_RendersChromeLineAboveViewportContent(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
			{WindowIndex: 1, WindowName: "other", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("alpha\nbeta\ngamma\n")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}

	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) < 1 {
		t.Fatalf("View() returned no lines: %q", out)
	}

	wantChrome := stripANSI(m.chromeLine())
	gotFirst := stripANSI(lines[0])
	if gotFirst != wantChrome {
		t.Errorf("View() first line = %q; want chrome line %q", gotFirst, wantChrome)
	}

	// Viewport content must appear after the chrome line.
	rest := strings.Join(lines[1:], "\n")
	if !strings.Contains(stripANSI(rest), "alpha") {
		t.Errorf("View() = %q; expected viewport content (containing %q) below chrome", out, "alpha")
	}
}

func TestPreviewWindowSizeMsg_SetsViewportHeightToMsgHeightMinusChrome(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if updated.viewport.Width != 120 {
		t.Errorf("expected viewport.Width=120, got %d", updated.viewport.Width)
	}
	wantHeight := 40 - previewFrameOverhead
	if updated.viewport.Height != wantHeight {
		t.Errorf("expected viewport.Height=%d (msg.Height - previewFrameOverhead), got %d", wantHeight, updated.viewport.Height)
	}
}

func TestPreviewView_ChromeRowCountConstantAcrossTabAndBracketCycles(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
			{WindowIndex: 1, WindowName: "other", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("content\n")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("setup: expected ok=true, got false")
	}

	chromeLineCount := func(s string) int {
		return strings.Count(s, "\n") + 1
	}

	before := chromeLineCount(m.chromeLine())

	// Tab → next pane within current window.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := chromeLineCount(m.chromeLine()); got != before {
		t.Errorf("chrome line count changed after Tab: before=%d after=%d", before, got)
	}

	// `]` → next window.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if got := chromeLineCount(m.chromeLine()); got != before {
		t.Errorf("chrome line count changed after ]: before=%d after=%d", before, got)
	}

	// `[` → previous window.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if got := chromeLineCount(m.chromeLine()); got != before {
		t.Errorf("chrome line count changed after [: before=%d after=%d", before, got)
	}
}

func TestPreviewWindowSizeMsg_SmallHeightDoesNotProduceNegativeViewportHeight(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 1})

	if updated.viewport.Height < 0 {
		t.Errorf("expected viewport.Height >= 0 for small terminal, got %d", updated.viewport.Height)
	}

	updated2, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 0})
	if updated2.viewport.Height < 0 {
		t.Errorf("expected viewport.Height >= 0 for height=0, got %d", updated2.viewport.Height)
	}
}

func TestNewPreviewModel_SizesViewportWithChromeSubtracted(t *testing.T) {
	const initialHeight = 24
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("x\n")}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, initialHeight)
	if !ok {
		t.Fatalf("setup: expected ok=true, got false")
	}

	wantHeight := initialHeight - previewFrameOverhead
	if m.viewport.Height != wantHeight {
		t.Errorf("expected viewport.Height=%d (initialHeight - previewFrameOverhead), got %d", wantHeight, m.viewport.Height)
	}
}
