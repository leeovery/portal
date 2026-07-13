package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Tests for task 5-4: the §5 multi-select mode footer. In the mode (filter not
// focused) the footer swaps the standard Sessions footer for the spec-exact copy
// fixed by the delivered Paper frame (design/sessions-multi-select-active.png):
// `↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel` — five entries, NO
// right-aligned `? help` anchor. It reuses the filter-footer entry machinery for
// per-glyph colouring and the shared 1px border.footer top rule, so it is
// height-neutral against the reserved sessionFooterHeight budget.
//
// No t.Parallel() — the package's shared canvas/mock helpers make parallelism
// unsafe across these tests.

// TestMultiSelectFooter_ExactCopy asserts the multi-select footer entry row reads
// EXACTLY the spec-exact copy (the delivered Paper frame), separators as the shared
// footerEntrySeparator ` · `. The footer is two rows (rule + entry row), so the entry
// row is the LAST line.
func TestMultiSelectFooter_ExactCopy(t *testing.T) {
	footer := renderMultiSelectFooter(referenceFooterWidth, theme.Dark, false)
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

// TestMultiSelectFooter_CopyConstant pins the copy to the spec-exact single-source
// constant (mirroring TestCommandBand_FixedTextConstant), so the wording cannot
// drift from a paraphrase.
func TestMultiSelectFooter_CopyConstant(t *testing.T) {
	const want = "↑↓ navigate · m toggle · ␣ preview · ⏎ open · esc cancel"
	if multiSelectFooterText != want {
		t.Errorf("multiSelectFooterText = %q, want the spec-exact wording %q", multiSelectFooterText, want)
	}
	// And the rendered footer's stripped entry row equals that same constant — the
	// render is tied to the spec-pinned copy.
	footer := renderMultiSelectFooter(referenceFooterWidth, theme.Dark, false)
	lines := strings.Split(footer, "\n")
	got := strings.TrimRight(footerVisible(lines[len(lines)-1]), " ")
	if got != multiSelectFooterText {
		t.Errorf("rendered entry row = %q, want the constant %q", got, multiSelectFooterText)
	}
}

// TestMultiSelectFooter_NoHelpAnchor asserts the multi-select footer carries NO
// right-aligned `? help` anchor (unlike the standard/filter footers) — the delivered
// frame has none, so neither the `? help` text nor the accent.violet `?` glyph appears.
func TestMultiSelectFooter_NoHelpAnchor(t *testing.T) {
	footer := renderMultiSelectFooter(referenceFooterWidth, theme.Dark, false)
	vis := footerVisible(footer)
	if strings.Contains(vis, "? help") {
		t.Errorf("multi-select footer must NOT show a right-aligned '? help' anchor:\n%s", vis)
	}
	if strings.Contains(vis, "help") {
		t.Errorf("multi-select footer must NOT contain the help hint at all:\n%s", vis)
	}
	// The ? help glyph is the only accent.violet run in the standard footer; the
	// multi-select footer drops the anchor, so no accent.violet must appear.
	if seq := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark); strings.Contains(footer, seq) {
		t.Errorf("multi-select footer must NOT carry the accent.violet ? glyph role sequence %q", seq)
	}
}

// TestMultiSelectFooter_HeightNeutral asserts the footer is exactly two rows (the 1px
// border.footer top rule + the entry row) — the SAME height as the standard Sessions
// footer, so swapping it in is height-neutral against the reserved sessionFooterHeight
// budget (the swap must not change the list height).
func TestMultiSelectFooter_HeightNeutral(t *testing.T) {
	ms := renderMultiSelectFooter(referenceFooterWidth, theme.Dark, false)
	std := renderSessionsFooter(referenceFooterWidth, theme.Dark, false)
	if got, want := lipgloss.Height(ms), lipgloss.Height(std); got != want {
		t.Errorf("multi-select footer height = %d, want %d (== standard footer, height-neutral)", got, want)
	}
	if got := lipgloss.Height(ms); got != 2 {
		t.Errorf("multi-select footer height = %d, want 2 (1px rule + entry row)", got)
	}
}

// TestMultiSelectFooter_NarrowDegradeOneLineEllipsis asserts §2.7: below the width at
// which the full cluster fits, the footer truncates gracefully (leading entries kept,
// trailing dropped, ellipsis marker) on ONE line — never wrapping to a second row and
// never overflowing the width.
func TestMultiSelectFooter_NarrowDegradeOneLineEllipsis(t *testing.T) {
	for _, w := range []int{56, 40, 30, 20, 12} {
		footer := renderMultiSelectFooter(w, theme.Dark, false)
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

	// At a width that truncates the full cluster, the ellipsis marks the drop, the
	// highest-priority leading entry survives, and the lowest-priority trailing entry
	// drops first.
	footer := footerVisible(renderMultiSelectFooter(30, theme.Dark, false))
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

// TestMultiSelectFooter_NoColorKeepsGlyphsDropsHues asserts the NO_COLOR carve-out
// (§2.5): a colourless footer carries no canvas background SGR and no foreground hue —
// it renders on the terminal's native fg/bg, the glyphs structurally intact.
func TestMultiSelectFooter_NoColorKeepsGlyphsDropsHues(t *testing.T) {
	footer := renderMultiSelectFooter(referenceFooterWidth, theme.Dark, true)

	vis := footerVisible(footer)
	for _, want := range []string{"↑↓ navigate", "m toggle", "␣ preview", "⏎ open", "esc cancel"} {
		if !strings.Contains(vis, want) {
			t.Errorf("colourless multi-select footer missing %q:\n%s", want, vis)
		}
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(footer, seq) {
		t.Errorf("colourless footer still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{theme.MV.AccentBlue, theme.MV.TextDetail, theme.MV.BorderFooter} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(footer, seq) {
			t.Errorf("colourless footer still emits a foreground role sequence %q", seq)
		}
	}
}

// TestMultiSelectFooter_TokenColours asserts key glyphs render in accent.blue and
// labels in text.detail (the standard MV footer convention), over a border.footer top
// rule — every colour via its §2.9 token, in both canvas modes.
func TestMultiSelectFooter_TokenColours(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		footer := renderMultiSelectFooter(referenceFooterWidth, mode, false)
		if seq := tokenFgSeq(t, theme.MV.AccentBlue, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] footer missing accent.blue key-glyph role sequence %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] footer missing text.detail label role sequence %q", mode, seq)
		}
		if seq := tokenFgSeq(t, theme.MV.BorderFooter, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] footer missing border.footer rule role sequence %q", mode, seq)
		}
	}
}

// TestMultiSelectFooter_PaintsCanvasNoEdgeBleed asserts the footer cells carry the
// owned canvas background (leaf .Background(canvas)) so the pad gap is not a
// terminal-bg island.
func TestMultiSelectFooter_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		footer := renderMultiSelectFooter(referenceFooterWidth, mode, false)
		if seq := canvasSeq(t, mode); !strings.Contains(footer, seq) {
			t.Errorf("[%v] footer does not paint the canvas background sequence %q:\n%s", mode, seq, footer)
		}
	}
}

// TestSessionsFooterResolver_MultiSelectMode asserts renderSessionsFooterForFilterState
// renders the multi-select footer while in the mode and NOT filter-focused (the
// unfiltered case): the entry row carries the mode copy and NOT the standard footer's
// entries or the `? help` anchor.
func TestSessionsFooterResolver_MultiSelectMode(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}, {Name: "bravo", Windows: 1}})
	m.termWidth = 120
	m.multiSelectMode = true

	footer := footerVisible(m.renderSessionsFooterForFilterState())
	if !strings.Contains(footer, "m toggle") {
		t.Errorf("multi-select-mode resolver must render the multi-select footer (missing 'm toggle'):\n%s", footer)
	}
	if strings.Contains(footer, "? help") {
		t.Errorf("multi-select-mode resolver footer must NOT carry a '? help' anchor:\n%s", footer)
	}
	// The standard footer's 'switch view' entry must NOT leak into the mode footer.
	if strings.Contains(footer, "switch view") {
		t.Errorf("multi-select footer must not carry the standard 'switch view' entry:\n%s", footer)
	}
}

// TestSessionsFooterResolver_FilteringOverridesMultiSelect asserts precedence: while
// the filter input is focused within the mode (FilterState == Filtering), the
// input-active filter footer renders — the multi-select footer steps aside.
func TestSessionsFooterResolver_FilteringOverridesMultiSelect(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}, {Name: "bravo", Windows: 1}})
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
