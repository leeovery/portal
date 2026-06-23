package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Lipgloss v2 changed where the colour profile is applied: Style.Render now
// always emits the full TrueColor SGR sequences in-string, and palette
// downsampling / NO_COLOR suppression happen at the OUTPUT-writer layer (the
// Bubble Tea renderer / colorprofile.Writer when content is flushed to the
// terminal), not inside Render. So the preview-frame tests below — which
// assert on the emitted '\x1b[38;...m' foreground bytes — see those bytes by
// default under `go test`, with no profile override needed. The v1 TestMain
// here called lipgloss.SetColorProfile(termenv.TrueColor) to force SGRs under
// the non-TTY test environment; that API was removed in v2 (Render is
// unconditionally TrueColor), so the override is gone.

// TestPreviewView_FrameContainsAllFourRoundedCorners pins the acceptance
// criterion that View() output contains the rounded corner glyphs sourced
// from lipgloss.RoundedBorder(): ╭ ╮ ╰ ╯ — all four must appear.
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

// TestPreviewView_TopRowWidthEqualsOuterTerminalWidth pins that the
// hand-composed top edge spans the full outer terminal width.
func TestPreviewView_TopRowWidthEqualsOuterTerminalWidth(t *testing.T) {
	m := newFramePreviewModel(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	out := m.View()
	topRow := firstLine(out)

	if got := lipgloss.Width(topRow); got != 80 {
		t.Errorf("top row width = %d; want 80; row=%q", got, topRow)
	}
}

// TestPreviewView_HeaderContainsMarkerSessionCounters_FooterContainsHints pins
// the §9.1 chrome content surfaced inside the View at a width wide enough for the
// header cascade to land at tier 1 (marker + full session + counters) and the
// footer to render the full labelled nav hints.
func TestPreviewView_HeaderContainsMarkerSessionCounters_FooterContainsHints(t *testing.T) {
	const wideWidth = 120
	m := newFramePreviewModelAt(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"), wideWidth, 24)
	m, _ = m.Update(tea.WindowSizeMsg{Width: wideWidth, Height: 24})

	out := stripANSI(m.View())

	// Session is fixed to "work" in newFramePreviewModelAt.
	if !strings.Contains(out, "◉ preview work Window 1/1 · Pane 1/1") {
		t.Errorf("View() missing tier-1 header content; got:\n%s", out)
	}
	if !strings.Contains(out, "←→ window  ⇥ pane  ⏎ attach  ␣ back") {
		t.Errorf("View() missing footer nav hints; got:\n%s", out)
	}
}

// TestPreviewView_HeaderSegmentsCarryRoleForegrounds pins the §9.1 colour
// application: the header counters segment is styled text.detail and the session
// text.primary — the chrome content IS themed (only the captured CONTENT is left
// untouched, §9.2).
func TestPreviewView_HeaderSegmentsCarryRoleForegrounds(t *testing.T) {
	const wideWidth = 120
	m := newFramePreviewModelAt(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"), wideWidth, 24)
	m, _ = m.Update(tea.WindowSizeMsg{Width: wideWidth, Height: 24})

	header := headerLine(m.View())
	if !segmentCarriesForeground(header, "Window 1/1 · Pane 1/1", theme.MV.TextDetail.ColorFor(theme.Dark)) {
		t.Errorf("counters segment lacks text.detail foreground SGR; row=%q", header)
	}
	if !segmentCarriesForeground(header, "work", theme.MV.TextPrimary.ColorFor(theme.Dark)) {
		t.Errorf("session segment lacks text.primary foreground SGR; row=%q", header)
	}
}

// TestPreviewView_AllFourCornerGlyphsPrecededByForegroundSGR pins that
// every corner glyph is part of a BorderForeground-styled segment.
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
		// Search the bytes preceding this glyph (back to the most recent
		// '\n' or start of string) for a foreground SGR prefix.
		startOfLine := strings.LastIndexByte(out[:idx], '\n') + 1
		preceding := out[startOfLine:idx]
		if !strings.Contains(preceding, "\x1b[38;") {
			t.Errorf("glyph %q is not preceded by a foreground SGR on its line; preceding=%q", glyph, preceding)
		}
	}
}

// TestPreviewView_AppliesSGRResetToEveryNonEmptyViewportRow pins
// § SGR reset injection — every non-empty viewport content row must
// carry the '\x1b[0m' reset at row-end (before lipgloss composes the
// right border). With viewport padding the reset lands at end-of-padded-row;
// per-line, the assertion is that the row's payload contains '\x1b[0m'
// AND that the unterminated SGR ('\x1b[41m' in the fixture) cannot bleed
// past row-end into the right border without an intervening reset.
func TestPreviewView_AppliesSGRResetToEveryNonEmptyViewportRow(t *testing.T) {
	m := newFramePreviewModel(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"))
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	out := m.View()
	lines := strings.Split(out, "\n")

	// Find the rows containing "hello" and "world" and assert each carries
	// '\x1b[0m' somewhere between the content and the right border.
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
		// The unterminated '\x1b[41m' from the fixture must not extend
		// unchecked into the right border — i.e. a reset must appear
		// between the '\x1b[41m' opener on this row and the row's end.
		if strings.Contains(row, "\x1b[41m") && !strings.Contains(afterPayload, "\x1b[0m") {
			t.Errorf("row containing %q has unterminated '\\x1b[41m' bleeding to row-end; row=%q", payload, row)
		}
	}
}

// TestPreviewView_FirstFrameCorrectnessAtConstruction pins
// § Initial sizing and preview-open ordering — the very first View() call
// on the freshly-constructed previewModel, with no prior WindowSizeMsg,
// must already paint the top row at the constructor-provided outer width.
func TestPreviewView_FirstFrameCorrectnessAtConstruction(t *testing.T) {
	m := newFramePreviewModel(t, "nvim-editor", []byte("\x1b[41mhello\nworld\n"))

	out := m.View()
	topRow := firstLine(out)

	if got := lipgloss.Width(topRow); got != 80 {
		t.Errorf("first-frame top row width = %d; want 80; row=%q", got, topRow)
	}
}

// TestPreviewView_AtDegenerateWidth2Height4RendersWithoutPanic pins the
// degenerate-width contract — handing width=2, height=4 to lipgloss must
// not panic. The output shape is left to lipgloss's clipping behaviour.
func TestPreviewView_AtDegenerateWidth2Height4RendersWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View() panicked at degenerate width=2 height=4: %v", r)
		}
	}()

	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "nvim-editor", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello\n")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 2, 4)
	if !ok {
		t.Fatalf("expected ok=true from NewPreviewModel, got false")
	}
	_ = m.View()

	// Also verify resize-to-degenerate doesn't panic.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 2, Height: 4})
	_ = m.View()
}

// TestPreviewView_RecomputesChromeEveryTickNoCachedField pins that View()
// reads structural fields directly on each call — mutating m.windowIdx
// between two View() calls must shift the chrome ordinal accordingly.
func TestPreviewView_RecomputesChromeEveryTickNoCachedField(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "alpha", PaneIndices: []int{0}},
			{WindowIndex: 1, WindowName: "beta", PaneIndices: []int{0}},
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

	// Bypass cycle handlers (which trigger a Tail read); mutate the
	// structural index directly and re-render. If chrome were cached on
	// the model, the second View() would still show "Window 1/2".
	m.windowIdx = 1

	second := stripANSI(m.View())
	if !strings.Contains(second, "Window 2/2") {
		t.Errorf("second View() after windowIdx mutation missing 'Window 2/2'; chrome may be cached. got:\n%s", second)
	}
}
