package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tmux"
)

func TestPreviewView_FrameContainsAllFourRoundedCorners(t *testing.T) {
	m := newFramePreviewModel(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	out := m.View()

	for _, glyph := range []string{"╭", "╮", "╰", "╯"} {
		if !strings.Contains(out, glyph) {
			t.Errorf("View() missing corner glyph %q; got:\n%s", glyph, out)
		}
	}
}

func TestPreviewView_TopRowWidthEqualsOuterTerminalWidth(t *testing.T) {
	m := newFramePreviewModel(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	out := m.View()
	topRow := firstLine(out)

	if got := lipgloss.Width(topRow); got != 80 {
		t.Errorf("top row width = %d; want 80; row=%q", got, topRow)
	}
}

func TestPreviewView_HeaderContainsMarkerSessionCounters_FooterContainsHints(t *testing.T) {
	const wideWidth = 120
	m := newFramePreviewModelAt(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"), wideWidth, 24)
	m, _ = m.Update(tea.WindowSizeMsg{Width: wideWidth, Height: 24})

	out := stripANSI(m.View())

	if !strings.Contains(out, "◉ preview work Window 1/1 · Pane 1/1") {
		t.Errorf("View() missing tier-1 header content; got:\n%s", out)
	}
	if !strings.Contains(out, "←→ window  ⇥ pane  ⏎ attach  ␣ back") {
		t.Errorf("View() missing footer nav hints; got:\n%s", out)
	}
}

func TestPreviewView_HeaderSegmentsCarryRoleForegrounds(t *testing.T) {
	const wideWidth = 120
	m := newFramePreviewModelAt(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"), wideWidth, 24)
	m, _ = m.Update(tea.WindowSizeMsg{Width: wideWidth, Height: 24})

	header := headerLine(m.View())
	if !segmentCarriesForeground(header, "Window 1/1 · Pane 1/1", testDarkTheme(t).TextMuted.Color()) {
		t.Errorf("counters segment lacks text.muted foreground SGR; row=%q", header)
	}
	if !segmentCarriesForeground(header, "work", testDarkTheme(t).TextPrimary.Color()) {
		t.Errorf("session segment lacks text.primary foreground SGR; row=%q", header)
	}
}

func TestPreviewView_AllFourCornerGlyphsPrecededByForegroundSGR(t *testing.T) {
	m := newFramePreviewModel(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	out := m.View()

	for _, glyph := range []string{"╭", "╮", "╰", "╯"} {
		idx := strings.Index(out, glyph)
		if idx < 0 {
			t.Errorf("glyph %q not found in View() output", glyph)
			continue
		}
		startOfLine := strings.LastIndexByte(out[:idx], '\n') + 1
		preceding := out[startOfLine:idx]
		if !strings.Contains(preceding, "\x1b[38;") {
			t.Errorf("glyph %q is not preceded by a foreground SGR on its line; preceding=%q", glyph, preceding)
		}
	}
}

func TestPreviewView_AppliesSGRResetToEveryNonEmptyViewportRow(t *testing.T) {
	m := newFramePreviewModel(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	out := m.View()
	lines := strings.Split(out, "\n")

	for _, payload := range []string{"hello", "world"} {
		var row string
		for _, l := range lines {
			if strings.Contains(l, payload) {
				row = l
				break
			}
		}
		if row == "" {
			t.Fatalf("could not locate row containing %q in output:\n%s", payload, out)
		}
		payloadIdx := strings.Index(row, payload)
		afterPayload := row[payloadIdx+len(payload):]
		if !strings.Contains(afterPayload, "\x1b[0m") {
			t.Errorf("row containing %q lacks SGR reset after payload; row=%q", payload, row)
		}
		if strings.Contains(row, "\x1b[41m") && !strings.Contains(afterPayload, "\x1b[0m") {
			t.Errorf("row containing %q has unterminated '\\x1b[41m' bleeding to row-end; row=%q", payload, row)
		}
	}
}

func TestPreviewView_FirstFrameCorrectnessAtConstruction(t *testing.T) {
	m := newFramePreviewModel(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"))

	out := m.View()
	topRow := firstLine(out)

	if got := lipgloss.Width(topRow); got != 80 {
		t.Errorf("first-frame top row width = %d; want 80; row=%q", got, topRow)
	}
}

func TestPreviewView_AtDegenerateWidth2Height4RendersWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View() panicked at degenerate width=2 height=4: %v", r)
		}
	}()

	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "nvim-editor", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello\n")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 2, 4)
	if !ok {
		t.Fatalf("expected ok=true from NewPreviewModel, got false")
	}
	_ = m.View()

	m, _ = m.Update(tea.WindowSizeMsg{Width: 2, Height: 4})
	_ = m.View()
}

func TestPreviewView_RecomputesChromeEveryTickNoCachedField(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "alpha", Panes: []tmux.WindowPane{{Index: 0}}},
			{WindowIndex: 1, WindowName: "beta", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("content\n")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	first := stripANSI(m.View())
	if !strings.Contains(first, "Window 1/2") {
		t.Fatalf("first View() missing 'Window 1/2'; got:\n%s", first)
	}

	m.windowIdx = 1

	second := stripANSI(m.View())
	if !strings.Contains(second, "Window 2/2") {
		t.Errorf("second View() after windowIdx mutation missing 'Window 2/2'; chrome may be cached. got:\n%s", second)
	}
}
