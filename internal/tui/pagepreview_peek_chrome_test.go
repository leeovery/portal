package tui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tmux"
)

func newPeekPreviewModel(t *testing.T, session string, groups []tmux.WindowGroup, payload []byte, width, height int) previewModel {
	t.Helper()
	enum := &stubEnumerator{groups: groups}
	reader := &recordingReader{bytes: payload}
	m, ok := NewPreviewModel(session, enum, reader, nil, width, height)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	return m
}

func TestPreviewPeekChrome_HeaderAndFooterRenderMarkerSessionCountersAndHints(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "server", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "aviva-proxy-qNyfEO", groups, []byte("hello\n"), 120, 24)

	view := m.View()
	header := stripANSI(headerLine(view))
	for _, want := range []string{"◉ preview", "aviva-proxy-qNyfEO", "Window 1/2 · Pane 1/2"} {
		if !strings.Contains(header, want) {
			t.Errorf("header = %q; want substring %q", header, want)
		}
	}

	footer := stripANSI(footerLine(view))
	if want := "←→ window  ⇥ pane  ⏎ attach  ␣ back"; !strings.Contains(footer, want) {
		t.Errorf("footer = %q; want substring %q", footer, want)
	}
}

func TestPreviewPeekChrome_OrdinalsAreOneBasedSlashTotals(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 2, WindowName: "second", Panes: []tmux.WindowPane{{Index: 4}, {Index: 7}}},
		{WindowIndex: 5, WindowName: "third", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)
	m.windowIdx = 1
	m.paneIdx = 1

	top := stripANSI(headerLine(m.View()))

	if !strings.Contains(top, "Window 2/3 · Pane 2/2") {
		t.Errorf("header = %q; want %q", top, "Window 2/3 · Pane 2/2")
	}
	for _, raw := range []string{"/5", "/7", " 5 ", " 7 "} {
		if strings.Contains(top, raw) {
			t.Errorf("header = %q leaked raw tmux index %q", top, raw)
		}
	}
}

func TestPreviewPeekChrome_MarkerStyledAccentCyan(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)

	top := headerLine(m.View())
	if !segmentCarriesForeground(top, "◉ preview", testDarkTheme(t).AccentMode.Color()) {
		t.Errorf("`◉ preview` marker is not styled with accent.mode; top=%q", top)
	}
}

func TestPreviewPeekChrome_SessionStyledTextPrimary(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "aviva-proxy", groups, []byte("x\n"), 120, 24)

	top := headerLine(m.View())
	if !segmentCarriesForeground(top, "aviva-proxy", testDarkTheme(t).TextPrimary.Color()) {
		t.Errorf("session name is not styled with text.primary; top=%q", top)
	}
}

func TestPreviewPeekChrome_CountersStyledTextDetail(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)

	top := headerLine(m.View())
	if !segmentCarriesForeground(top, "Window 1/1 · Pane 1/1", testDarkTheme(t).TextMuted.Color()) {
		t.Errorf("counters are not styled with text.muted; top=%q", top)
	}
}

func TestPreviewPeekChrome_FooterGlyphsAccentBlueLabelsTextDetail(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)

	foot := footerLine(m.View())
	if !segmentCarriesForeground(foot, "←→", testDarkTheme(t).AccentKey.Color()) {
		t.Errorf("footer `←→` glyph is not styled with accent.key; foot=%q", foot)
	}
	if !segmentCarriesForeground(foot, "window", testDarkTheme(t).TextMuted.Color()) {
		t.Errorf("footer `window` label is not styled with text.muted; foot=%q", foot)
	}
}

func segmentCarriesForeground(row, segment string, c color.Color) bool {
	wantSGR := lipgloss.NewStyle().Foreground(c).Render("X")
	open := wantSGR[:strings.Index(wantSGR, "X")]
	core := strings.TrimSuffix(strings.TrimPrefix(open, "\x1b["), "m")
	before, _, ok := strings.Cut(row, segment)
	if !ok {
		return false
	}
	return strings.Contains(before, core)
}

func TestPreviewPeekChrome_ContentFramedByAccentCyanBorder(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("hello\nworld\n"), 80, 24)

	out := m.View()
	cyanOpen := func() string {
		s := lipgloss.NewStyle().Foreground(testDarkTheme(t).AccentMode.Color()).Render("X")
		return s[:strings.Index(s, "X")]
	}()

	for _, glyph := range []string{"╭", "╮", "╰", "╯"} {
		idx := strings.Index(out, glyph)
		if idx < 0 {
			t.Errorf("corner glyph %q missing; out=%q", glyph, out)
			continue
		}
		startOfLine := strings.LastIndexByte(out[:idx], '\n') + 1
		if !strings.Contains(out[startOfLine:idx], cyanOpen) {
			t.Errorf("corner glyph %q not preceded by accent.mode SGR on its line", glyph)
		}
	}
}

func TestPreviewPeekChrome_CapturedContentLeftUntouched(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("\x1b[41mRAWLINE\x1b[0m\n"), 80, 24)

	out := m.View()
	if !strings.Contains(out, "\x1b[41mRAWLINE") {
		t.Errorf("captured ANSI content was altered; expected verbatim '\\x1b[41mRAWLINE' in output:\n%q", out)
	}
}

func TestPreviewPeekChrome_NavHintsInFooterCompartment(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)

	view := m.View()
	const hints = "←→ window  ⇥ pane  ⏎ attach  ␣ back"
	if foot := stripANSI(footerLine(view)); !strings.Contains(foot, hints) {
		t.Errorf("footer nav hints %q not present; footer=%q", hints, foot)
	}
	if header := stripANSI(headerLine(view)); strings.Contains(header, "window") {
		t.Errorf("nav hints leaked into the header; header=%q", header)
	}
}

func TestPreviewPeekChrome_FullScreenOverlayNotBlankScreenModal(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("hello\n"), 80, 24)

	header := stripANSI(headerLine(m.View()))
	if !strings.Contains(header, "◉ preview") {
		t.Errorf("preview overlay header is not the cyan peek-mode bar; got %q", header)
	}
}

func TestPreviewPeekChrome_NarrowWidthDegradesGracefully(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "a-very-long-window-name-that-will-not-fit", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	for _, w := range []int{120, 80, 60, 40, 25, 15, 8, 7} {
		m := newPeekPreviewModel(t, "a-long-session-name-here", groups, []byte("x\n"), w, 24)
		m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		out := m.View()
		for i, line := range strings.Split(out, "\n") {
			if got := lipgloss.Width(line); got != w {
				t.Errorf("width %d: frame line %d width = %d, want %d; line=%q", w, i, got, w, stripANSI(line))
			}
		}
	}
}

func TestPreviewPeekChrome_ColourlessKeepsStructureDropsHue(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	enum := &stubEnumerator{groups: groups}
	reader := &recordingReader{bytes: []byte("hello\n")}
	m, ok := NewPreviewModel("aviva-proxy", enum, reader, nil, 120, 24)
	if !ok {
		t.Fatalf("expected ok=true from NewPreviewModel")
	}
	m.colourless = true

	out := m.View()
	header := stripANSI(headerLine(out))
	for _, want := range []string{
		"◉ preview",
		"aviva-proxy",
		"Window 1/1 · Pane 1/1",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("colourless header = %q; want substring %q (structure must survive)", header, want)
		}
	}
	if foot := stripANSI(footerLine(out)); !strings.Contains(foot, "←→ window  ⇥ pane  ⏎ attach  ␣ back") {
		t.Errorf("colourless footer = %q; want the nav hints (structure must survive)", foot)
	}
	for _, glyph := range []string{"╭", "╮", "╰", "╯", "├", "┤"} {
		if !strings.Contains(out, glyph) {
			t.Errorf("colourless View() missing frame glyph %q", glyph)
		}
	}
	if strings.Contains(out, "\x1b[38;") {
		t.Errorf("colourless View() carries a foreground SGR; chrome must be colourless. out=%q", out)
	}
}
