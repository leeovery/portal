package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// TestPreviewView_JoinedPanelLayoutHeaderBodyFooter pins the §9.1 full-screen
// joined panel layout, top to bottom: top border → header (with the `◉ preview`
// marker + cascaded counters) → body (the captured content) → footer (the nav
// hints). The header sits on the SECOND line (line 1) directly under the top
// border (line 0); the footer is the last content line above the bottom border.
func TestPreviewView_JoinedPanelLayoutHeaderBodyFooter(t *testing.T) {
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
	if len(lines) < 4 {
		t.Fatalf("View() returned %d lines, want >= 4 (border/header/body/footer): %q", len(lines), out)
	}

	// Line 0 is the top border (corners only, no chrome content).
	top := stripANSI(lines[0])
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Errorf("View() first line = %q; want the rounded top border ╭…╮", top)
	}
	if strings.Contains(top, "◉ preview") {
		t.Errorf("top border must not carry header content; got %q", top)
	}

	// Line 1 is the header compartment carrying the marker + counters.
	header := stripANSI(lines[1])
	if !strings.Contains(header, "◉ preview work Window 1/2 · Pane 1/2") {
		t.Errorf("View() second line = %q; want the header compartment with marker + counters", header)
	}

	// The captured body content appears between header and footer.
	if !strings.Contains(stripANSI(out), "alpha") {
		t.Errorf("View() = %q; expected viewport content (containing %q)", out, "alpha")
	}

	// The footer sits on the last content line (above the bottom border).
	footer := stripANSI(lines[len(lines)-2])
	if !strings.Contains(footer, "←→ window") {
		t.Errorf("View() penultimate line = %q; want the footer nav hints", footer)
	}
	bottom := stripANSI(lines[len(lines)-1])
	if !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
		t.Errorf("View() last line = %q; want the rounded bottom border ╰…╯", bottom)
	}
}

// TestPreviewView_FillsFullTerminalHeight pins the §9.1 full-screen contract:
// the composed panel spans the full terminal height (the body fills, the footer
// sits flush at the bottom).
func TestPreviewView_FillsFullTerminalHeight(t *testing.T) {
	const termH = 24
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("only-one-line\n")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, termH)
	if !ok {
		t.Fatalf("setup: expected ok=true, got false")
	}

	out := m.View()
	if got := strings.Count(out, "\n") + 1; got != termH {
		t.Errorf("composed View() height = %d rows, want %d (full terminal height)", got, termH)
	}
}

func TestPreviewWindowSizeMsg_SetsViewportHeightToMsgHeightMinusChrome(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	wantWidth := 120 - previewFrameOverhead
	if updated.viewport.Width() != wantWidth {
		t.Errorf("expected viewport.Width=%d (msg.Width - previewFrameOverhead), got %d", wantWidth, updated.viewport.Width())
	}
	wantHeight := 40 - previewFrameOverhead
	if updated.viewport.Height() != wantHeight {
		t.Errorf("expected viewport.Height=%d (msg.Height - previewFrameOverhead), got %d", wantHeight, updated.viewport.Height())
	}
}

func TestPreviewView_ChromeRowCountConstantAcrossWindowAndPaneCycles(t *testing.T) {
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

	before := chromeLineCount(chromeLineForTest(m))

	// Tab → next pane within current window.
	m, _ = m.Update(nextPaneKey)
	if got := chromeLineCount(chromeLineForTest(m)); got != before {
		t.Errorf("header row count changed after Tab: before=%d after=%d", before, got)
	}

	// → → next window.
	m, _ = m.Update(nextWindowKey)
	if got := chromeLineCount(chromeLineForTest(m)); got != before {
		t.Errorf("header row count changed after →: before=%d after=%d", before, got)
	}

	// ← → previous window.
	m, _ = m.Update(prevWindowKey)
	if got := chromeLineCount(chromeLineForTest(m)); got != before {
		t.Errorf("header row count changed after ←: before=%d after=%d", before, got)
	}
}

func TestPreviewWindowSizeMsg_SmallHeightDoesNotProduceNegativeViewportHeight(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 1})

	if updated.viewport.Height() < 0 {
		t.Errorf("expected viewport.Height >= 0 for small terminal, got %d", updated.viewport.Height())
	}

	updated2, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 0})
	if updated2.viewport.Height() < 0 {
		t.Errorf("expected viewport.Height >= 0 for height=0, got %d", updated2.viewport.Height())
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
	if m.viewport.Height() != wantHeight {
		t.Errorf("expected viewport.Height=%d (initialHeight - previewFrameOverhead), got %d", wantHeight, m.viewport.Height())
	}
}
