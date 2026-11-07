package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

func TestMultiSelectFooter_ExactCopy(t *testing.T) {
	footer := renderMultiSelectFooter(referenceFooterWidth, testDarkTheme(t), false)
	lines := strings.Split(footer, "\n")
	if len(lines) != 2 {
		t.Fatalf("multi-select footer must be 2 rows (rule + entry row), got %d:\n%s", len(lines), footer)
	}

	const want = "↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel"
	got := strings.TrimRight(footerVisible(lines[1]), " ")
	if got != want {
		t.Errorf("multi-select footer entry row = %q, want exactly %q", got, want)
	}
}

func TestMultiSelectFooter_CopyConstant(t *testing.T) {
	const want = "↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel"
	if multiSelectFooterText != want {
		t.Errorf("multiSelectFooterText = %q, want the spec-exact wording %q", multiSelectFooterText, want)
	}
	footer := renderMultiSelectFooter(referenceFooterWidth, testDarkTheme(t), false)
	lines := strings.Split(footer, "\n")
	got := strings.TrimRight(footerVisible(lines[len(lines)-1]), " ")
	if got != multiSelectFooterText {
		t.Errorf("rendered entry row = %q, want the constant %q", got, multiSelectFooterText)
	}
}

func TestMultiSelectFooter_NoHelpAnchor(t *testing.T) {
	footer := renderMultiSelectFooter(referenceFooterWidth, testDarkTheme(t), false)
	vis := footerVisible(footer)
	if strings.Contains(vis, "? help") {
		t.Errorf("multi-select footer must NOT show a right-aligned '? help' anchor:\n%s", vis)
	}
	if strings.Contains(vis, "help") {
		t.Errorf("multi-select footer must NOT contain the help hint at all:\n%s", vis)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).AccentPrimary); strings.Contains(footer, seq) {
		t.Errorf("multi-select footer must NOT carry the accent.primary ? glyph role sequence %q", seq)
	}
}

func TestMultiSelectFooter_HeightNeutral(t *testing.T) {
	ms := renderMultiSelectFooter(referenceFooterWidth, testDarkTheme(t), false)
	std := renderSessionsFooter(sessionsKeymap(), referenceFooterWidth, testDarkTheme(t), false)
	if got, want := lipgloss.Height(ms), lipgloss.Height(std); got != want {
		t.Errorf("multi-select footer height = %d, want %d (== standard footer, height-neutral)", got, want)
	}
	if got := lipgloss.Height(ms); got != 2 {
		t.Errorf("multi-select footer height = %d, want 2 (1px rule + entry row)", got)
	}
}

func TestMultiSelectFooter_NarrowDegradeOneLineEllipsis(t *testing.T) {
	for _, w := range []int{56, 40, 30, 20, 12} {
		footer := renderMultiSelectFooter(w, testDarkTheme(t), false)
		lines := strings.Split(footer, "\n")
		if len(lines) != 2 {
			t.Errorf("at width %d the footer has %d rows, want 2 (rule + single entry row, no wrap):\n%s", w, len(lines), footer)
			continue
		}
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("at width %d, footer line %d width = %d (overflow)", w, i, lw)
			}
		}
	}

	footer := footerVisible(renderMultiSelectFooter(30, testDarkTheme(t), false))
	if !strings.Contains(footer, "↑↓ navigate") {
		t.Errorf("highest-priority entry 'navigate' must survive narrow truncation:\n%s", footer)
	}
	if !strings.Contains(footer, footerEllipsis) {
		t.Errorf("a truncated multi-select footer must carry the ellipsis drop marker:\n%s", footer)
	}
	if strings.Contains(footer, "esc cancel") {
		t.Errorf("lowest-priority trailing entry 'esc cancel' should drop first at width 30:\n%s", footer)
	}
}

func TestMultiSelectFooter_NoColorKeepsGlyphsDropsHues(t *testing.T) {
	footer := renderMultiSelectFooter(referenceFooterWidth, testDarkTheme(t), true)

	vis := footerVisible(footer)
	for _, want := range []string{"↑↓ navigate", "m toggle", "␣ preview", "⏎ open", "esc cancel"} {
		if !strings.Contains(vis, want) {
			t.Errorf("colourless multi-select footer missing %q:\n%s", want, vis)
		}
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(footer, seq) {
		t.Errorf("colourless footer still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{testDarkTheme(t).AccentKey, testDarkTheme(t).TextMuted, testDarkTheme(t).Border} {
		if seq := tokenFgSeq(t, tok); strings.Contains(footer, seq) {
			t.Errorf("colourless footer still emits a foreground role sequence %q", seq)
		}
	}
}

func TestMultiSelectFooter_TokenColours(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		footer := renderMultiSelectFooter(referenceFooterWidth, th, false)
		if seq := tokenFgSeq(t, th.AccentKey); !strings.Contains(footer, seq) {
			t.Errorf("[%v] footer missing accent.key key-glyph role sequence %q", themeLabel(th), seq)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(footer, seq) {
			t.Errorf("[%v] footer missing text.muted label role sequence %q", themeLabel(th), seq)
		}
		if seq := tokenFgSeq(t, th.Border); !strings.Contains(footer, seq) {
			t.Errorf("[%v] footer missing border rule role sequence %q", themeLabel(th), seq)
		}
	}
}

func TestMultiSelectFooter_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		footer := renderMultiSelectFooter(referenceFooterWidth, th, false)
		if seq := canvasSeq(t, th); !strings.Contains(footer, seq) {
			t.Errorf("[%v] footer does not paint the canvas background sequence %q:\n%s", themeLabel(th), seq, footer)
		}
	}
}

func TestSessionsFooterResolver_MultiSelectMode(t *testing.T) {
	m := NewModelWithSessions([]Session{{Name: "alpha"}, {Name: "bravo"}})
	m.termWidth = 120
	m.multiSelectMode = true

	footer := footerVisible(m.renderSessionsFooterForFilterState())
	if !strings.Contains(footer, "m toggle") {
		t.Errorf("multi-select-mode resolver must render the multi-select footer (missing 'm toggle'):\n%s", footer)
	}
	if strings.Contains(footer, "? help") {
		t.Errorf("multi-select-mode resolver footer must NOT carry a '? help' anchor:\n%s", footer)
	}
	if strings.Contains(footer, "switch view") {
		t.Errorf("multi-select footer must not carry the standard 'switch view' entry:\n%s", footer)
	}
}

func TestSessionsFooterResolver_FilteringOverridesMultiSelect(t *testing.T) {
	m := NewModelWithSessions([]Session{{Name: "alpha"}, {Name: "bravo"}})
	m.termWidth = 120
	m.multiSelectMode = true
	m.sessionList.SetFilterState(list.Filtering)

	footer := footerVisible(m.renderSessionsFooterForFilterState())
	if !strings.Contains(footer, "browse results") {
		t.Errorf("filter-focused-in-mode resolver must render the input-active filter footer:\n%s", footer)
	}
	if strings.Contains(footer, "m toggle") {
		t.Errorf("filter-focused-in-mode resolver must NOT render the multi-select footer:\n%s", footer)
	}
}
