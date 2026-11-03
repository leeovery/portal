package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPreviewView_JoinedPanelLayoutHeaderBodyFooter(t *testing.T) {
	m := openedPreview(t, "work", historyFake(t, "alpha\nbeta\ngamma\n", "work"), 80, 24)

	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) < 4 {
		t.Fatalf("View() returned %d lines, want >= 4 (border/header/body/footer): %q", len(lines), out)
	}

	top := stripANSI(lines[0])
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Errorf("View() first line = %q; want the rounded top border ╭…╮", top)
	}
	if strings.Contains(top, "◉ preview") {
		t.Errorf("top border must not carry header content; got %q", top)
	}

	header := stripANSI(lines[1])
	if !strings.HasSuffix(strings.TrimRight(header, " │"), "◉ preview work") {
		t.Errorf("View() second line = %q; want the header compartment with marker and session, nothing after", header)
	}

	if !strings.Contains(stripANSI(out), "alpha") {
		t.Errorf("View() = %q; expected viewport content (containing %q)", out, "alpha")
	}

	footer := stripANSI(lines[len(lines)-2])
	if !strings.Contains(footer, "↑↓ session") || strings.Contains(footer, "window") {
		t.Errorf("View() penultimate line = %q; want the footer nav hints", footer)
	}
	bottom := stripANSI(lines[len(lines)-1])
	if !strings.HasPrefix(bottom, "╰") || !strings.HasSuffix(bottom, "╯") {
		t.Errorf("View() last line = %q; want the rounded bottom border ╰…╯", bottom)
	}
}

func TestPreviewView_FillsFullTerminalHeight(t *testing.T) {
	const termH = 24
	m := openedPreview(t, "work", historyFake(t, "only-one-line\n", "work"), 80, termH)

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
	m := openedPreview(t, "work", historyFake(t, "x", "work"), 80, initialHeight)

	wantHeight := initialHeight - previewFrameOverhead
	if m.viewport.Height() != wantHeight {
		t.Errorf("expected viewport.Height=%d (initialHeight - previewFrameOverhead), got %d", wantHeight, m.viewport.Height())
	}
}
