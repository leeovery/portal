package tui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func newPeekPreviewModel(t *testing.T, session, vt string, width, height int) previewModel {
	t.Helper()
	m, ok := NewPreviewModel(session, newPreviewHistory(historyFake(t, vt, session)), width, height)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	return m
}

func TestPreviewPeekChrome_HeaderAndFooterRenderMarkerSessionAndHints(t *testing.T) {
	m := newPeekPreviewModel(t, "aviva-proxy-qNyfEO", "hello\n", 120, 24)

	view := m.View()
	header := stripANSI(headerLine(view))
	for _, want := range []string{"◉ preview", "aviva-proxy-qNyfEO"} {
		if !strings.Contains(header, want) {
			t.Errorf("header = %q; want substring %q", header, want)
		}
	}

	footer := stripANSI(footerLine(view))
	if want := "↑↓ session  ⏎ attach  ␣ back"; !strings.Contains(footer, want) {
		t.Errorf("footer = %q; want substring %q", footer, want)
	}
}

func TestPreviewPeekChrome_MarkerStyledAccentCyan(t *testing.T) {
	m := newPeekPreviewModel(t, "work", "x\n", 120, 24)

	top := headerLine(m.View())
	if !segmentCarriesForeground(top, "◉ preview", testDarkTheme(t).AccentMode.Color()) {
		t.Errorf("`◉ preview` marker is not styled with accent.mode; top=%q", top)
	}
}

func TestPreviewPeekChrome_SessionStyledTextPrimary(t *testing.T) {
	m := newPeekPreviewModel(t, "aviva-proxy", "x\n", 120, 24)

	top := headerLine(m.View())
	if !segmentCarriesForeground(top, "aviva-proxy", testDarkTheme(t).TextPrimary.Color()) {
		t.Errorf("session name is not styled with text.primary; top=%q", top)
	}
}

func TestPreviewPeekChrome_FooterGlyphsAccentBlueLabelsTextDetail(t *testing.T) {
	m := newPeekPreviewModel(t, "work", "x\n", 120, 24)

	foot := footerLine(m.View())
	if !segmentCarriesForeground(foot, "↑↓", testDarkTheme(t).AccentKey.Color()) {
		t.Errorf("footer `↑↓` glyph is not styled with accent.key; foot=%q", foot)
	}
	if !segmentCarriesForeground(foot, "session", testDarkTheme(t).TextMuted.Color()) {
		t.Errorf("footer `session` label is not styled with text.muted; foot=%q", foot)
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
	m := newPeekPreviewModel(t, "work", "hello\nworld\n", 80, 24)

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

func TestPreviewPeekChrome_SessionContentKeepsItsColours(t *testing.T) {
	m := newPeekPreviewModel(t, "work", "\x1b[41mRAWLINE\x1b[0m\n", 80, 24)

	out := m.View()
	if !strings.Contains(out, "\x1b[41mRAWLINE") {
		t.Errorf("the session's colours were lost; expected '\\x1b[41mRAWLINE' in output:\n%q", out)
	}
}

func TestPreviewPeekChrome_NavHintsInFooterCompartment(t *testing.T) {
	m := newPeekPreviewModel(t, "work", "x\n", 120, 24)

	view := m.View()
	const hints = "↑↓ session  ⏎ attach  ␣ back"
	if foot := stripANSI(footerLine(view)); !strings.Contains(foot, hints) {
		t.Errorf("footer nav hints %q not present; footer=%q", hints, foot)
	}
	if header := stripANSI(headerLine(view)); strings.Contains(header, "attach") {
		t.Errorf("nav hints leaked into the header; header=%q", header)
	}
}

func TestPreviewPeekChrome_FullScreenOverlayNotBlankScreenModal(t *testing.T) {
	m := newPeekPreviewModel(t, "work", "hello\n", 80, 24)

	header := stripANSI(headerLine(m.View()))
	if !strings.Contains(header, "◉ preview") {
		t.Errorf("preview overlay header is not the cyan peek-mode bar; got %q", header)
	}
}

func TestPreviewPeekChrome_NarrowWidthDegradesGracefully(t *testing.T) {
	for _, w := range []int{120, 80, 60, 40, 25, 15, 8, 7} {
		m := newPeekPreviewModel(t, "a-long-session-name-here", "x\n", w, 24)
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
	m, ok := NewPreviewModel("aviva-proxy", newPreviewHistory(historyFake(t, "hello", "aviva-proxy")), 120, 24)
	if !ok {
		t.Fatalf("expected ok=true from NewPreviewModel")
	}
	m.colourless = true

	out := m.View()
	header := stripANSI(headerLine(out))
	for _, want := range []string{
		"◉ preview",
		"aviva-proxy",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("colourless header = %q; want substring %q (structure must survive)", header, want)
		}
	}
	if foot := stripANSI(footerLine(out)); !strings.Contains(foot, "↑↓ session  ⏎ attach  ␣ back") {
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
