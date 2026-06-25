package tui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// These tests pin the §9.1 cyan "peek mode" preview chrome (the full-screen
// joined panel): a `◉ preview` marker (accent.cyan) + the session name
// (text.primary) + `Window x/y · Pane x/y` counters (text.detail) in the header
// compartment, the footer nav hints `←→ window  ⇥ pane  ⏎ attach  ␣ back`
// (accent.blue glyphs + text.detail labels), all framed by the accent.cyan
// border + dividers. The captured ANSI content stays untouched.

// newPeekPreviewModel builds a previewModel with the given session name,
// groups, and a canned ScrollbackReader payload at the canonical 80x24 size,
// resolved in dark mode (the default canvas) and colourful (NO_COLOR off).
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

// TestPreviewPeekChrome_HeaderAndFooterRenderMarkerSessionCountersAndHints pins
// the §9.1 header content (marker + session + slash counters) and footer content
// (the nav hints), stripped of styling.
func TestPreviewPeekChrome_HeaderAndFooterRenderMarkerSessionCountersAndHints(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "server", PaneIndices: []int{0}},
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

// TestPreviewPeekChrome_OrdinalsAreOneBasedSlashTotals pins that the counters
// render 1-based ordinals over the totals in slash form, never the raw tmux
// indices.
func TestPreviewPeekChrome_OrdinalsAreOneBasedSlashTotals(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 2, WindowName: "second", PaneIndices: []int{4, 7}},
		{WindowIndex: 5, WindowName: "third", PaneIndices: []int{0}},
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

// TestPreviewPeekChrome_MarkerStyledAccentCyan pins that the `◉ preview` marker
// carries the accent.cyan foreground SGR (the mode-resolved dark hex).
func TestPreviewPeekChrome_MarkerStyledAccentCyan(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)

	top := headerLine(m.View())
	if !segmentCarriesForeground(top, "◉ preview", theme.MV.AccentCyan.ColorFor(theme.Dark)) {
		t.Errorf("`◉ preview` marker is not styled with accent.cyan; top=%q", top)
	}
}

// TestPreviewPeekChrome_SessionStyledTextPrimary pins that the session name
// carries the text.primary foreground SGR.
func TestPreviewPeekChrome_SessionStyledTextPrimary(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
	}
	m := newPeekPreviewModel(t, "aviva-proxy", groups, []byte("x\n"), 120, 24)

	top := headerLine(m.View())
	if !segmentCarriesForeground(top, "aviva-proxy", theme.MV.TextPrimary.ColorFor(theme.Dark)) {
		t.Errorf("session name is not styled with text.primary; top=%q", top)
	}
}

// TestPreviewPeekChrome_CountersStyledTextDetail pins that the counters carry
// the text.detail foreground SGR.
func TestPreviewPeekChrome_CountersStyledTextDetail(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)

	top := headerLine(m.View())
	if !segmentCarriesForeground(top, "Window 1/1 · Pane 1/1", theme.MV.TextDetail.ColorFor(theme.Dark)) {
		t.Errorf("counters are not styled with text.detail; top=%q", top)
	}
}

// TestPreviewPeekChrome_FooterGlyphsAccentBlueLabelsTextDetail pins the §9.1
// footer colour roles: each nav-hint glyph carries the accent.blue foreground
// and each label the text.detail foreground.
func TestPreviewPeekChrome_FooterGlyphsAccentBlueLabelsTextDetail(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)

	foot := footerLine(m.View())
	if !segmentCarriesForeground(foot, "←→", theme.MV.AccentBlue.ColorFor(theme.Dark)) {
		t.Errorf("footer `←→` glyph is not styled with accent.blue; foot=%q", foot)
	}
	if !segmentCarriesForeground(foot, "window", theme.MV.TextDetail.ColorFor(theme.Dark)) {
		t.Errorf("footer `window` label is not styled with text.detail; foot=%q", foot)
	}
}

// segmentCarriesForeground reports whether the plain `segment` appears in `row`
// preceded (on its line) by the colour's foreground SGR core. The chrome segments
// set BOTH a foreground and the canvas background, so the emitted SGR is
// `\x1b[38;2;R;G;B;48;2;…m` — a combined run. Matching the foreground CORE
// (`38;2;R;G;B`) as a substring of the styled prefix tolerates the trailing
// background bytes while still pinning the requested foreground colour.
func segmentCarriesForeground(row, segment string, c color.Color) bool {
	wantSGR := lipgloss.NewStyle().Foreground(c).Render("X")
	// Extract the foreground core: strip the leading "\x1b[" and the trailing "m"
	// from the fg-only open sequence, leaving "38;2;R;G;B".
	open := wantSGR[:strings.Index(wantSGR, "X")]
	core := strings.TrimSuffix(strings.TrimPrefix(open, "\x1b["), "m")
	before, _, ok := strings.Cut(row, segment)
	if !ok {
		return false
	}
	return strings.Contains(before, core)
}

// TestPreviewPeekChrome_ContentFramedByAccentCyanBorder pins that the content
// frame border (corners + body) carries the accent.cyan foreground.
func TestPreviewPeekChrome_ContentFramedByAccentCyanBorder(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("hello\nworld\n"), 80, 24)

	out := m.View()
	cyanOpen := func() string {
		s := lipgloss.NewStyle().Foreground(theme.MV.AccentCyan.ColorFor(theme.Dark)).Render("X")
		return s[:strings.Index(s, "X")]
	}()

	// Every corner glyph must be preceded on its line by the cyan foreground SGR.
	for _, glyph := range []string{"╭", "╮", "╰", "╯"} {
		idx := strings.Index(out, glyph)
		if idx < 0 {
			t.Errorf("corner glyph %q missing; out=%q", glyph, out)
			continue
		}
		startOfLine := strings.LastIndexByte(out[:idx], '\n') + 1
		if !strings.Contains(out[startOfLine:idx], cyanOpen) {
			t.Errorf("corner glyph %q not preceded by accent.cyan SGR on its line", glyph)
		}
	}
}

// TestPreviewPeekChrome_CapturedContentLeftUntouched pins that the captured
// ANSI body is rendered verbatim — the chrome restyle never themes the
// content. A distinctive raw SGR (`\x1b[41m`, red background) embedded in the
// payload must survive into the output unchanged (the content is real ANSI,
// not theme tokens).
func TestPreviewPeekChrome_CapturedContentLeftUntouched(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("\x1b[41mRAWLINE\x1b[0m\n"), 80, 24)

	out := m.View()
	if !strings.Contains(out, "\x1b[41mRAWLINE") {
		t.Errorf("captured ANSI content was altered; expected verbatim '\\x1b[41mRAWLINE' in output:\n%q", out)
	}
}

// TestPreviewPeekChrome_NavHintsInFooterCompartment pins that the §9.1 nav hints
// live in their own FOOTER compartment (between the footer divider and the bottom
// border), left-aligned and space-separated — NOT in the header.
func TestPreviewPeekChrome_NavHintsInFooterCompartment(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("x\n"), 120, 24)

	view := m.View()
	const hints = "←→ window  ⇥ pane  ⏎ attach  ␣ back"
	if foot := stripANSI(footerLine(view)); !strings.Contains(foot, hints) {
		t.Errorf("footer nav hints %q not present; footer=%q", hints, foot)
	}
	// The hints must NOT appear in the header compartment.
	if header := stripANSI(headerLine(view)); strings.Contains(header, "window") {
		t.Errorf("nav hints leaked into the header; header=%q", header)
	}
}

// TestPreviewPeekChrome_FullScreenOverlayNotBlankScreenModal pins that Space
// on a session opens the preview as a full-screen overlay (activePage flips to
// pagePreview), NOT through the §8.1 blank-screen modal path — m.modal stays
// modalNone and the preview View() renders the cyan chrome (not a centred
// blanked modal panel).
func TestPreviewPeekChrome_FullScreenOverlayNotBlankScreenModal(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
	}
	m := newPeekPreviewModel(t, "work", groups, []byte("hello\n"), 80, 24)

	// The preview's own View must be the cyan-framed overlay: the header
	// compartment carries "◉ preview", not a blank canvas row.
	header := stripANSI(headerLine(m.View()))
	if !strings.Contains(header, "◉ preview") {
		t.Errorf("preview overlay header is not the cyan peek-mode bar; got %q", header)
	}
}

// TestPreviewPeekChrome_NarrowWidthDegradesGracefully drives a narrow terminal
// and asserts the full-screen joined panel never overflows — EVERY frame line
// (border, header, body, footer) is exactly the terminal width — and carries no
// embedded newline corruption (the header and footer cascade, never expand the
// panel beyond the terminal).
func TestPreviewPeekChrome_NarrowWidthDegradesGracefully(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "a-very-long-window-name-that-will-not-fit", PaneIndices: []int{0}},
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

// TestPreviewPeekChrome_ColourlessKeepsStructureDropsHue pins the §2.5 / §9.2
// NO_COLOR carve-out: the chrome renders colourless (no foreground SGR) but
// the structure — marker, session, counters, footer hints, frame glyphs — stays
// present.
func TestPreviewPeekChrome_ColourlessKeepsStructureDropsHue(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "editor", PaneIndices: []int{0}},
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
	// No foreground colour SGR anywhere in the chrome (frame + header + footer).
	if strings.Contains(out, "\x1b[38;") {
		t.Errorf("colourless View() carries a foreground SGR; chrome must be colourless. out=%q", out)
	}
}
