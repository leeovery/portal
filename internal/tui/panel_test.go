package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// TestJoinedPanel_SingleToneJoinedFrame asserts the shared single-tone joined
// panel: a hand-drawn rounded frame (╭─╮ / │…│ / ╰─╯) whose every glyph — corners,
// sides, dividers — renders in border.separator (single-tone), with the
// compartment dividers joined to the side borders via real ├/┤ junctions. The
// help modal and the kill modal both compose through this helper.
func TestJoinedPanel_SingleToneJoinedFrame(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compartments := [][]string{
				{"header"},
				{"body line one", "body line two"},
				{"footer"},
			}
			panel := renderJoinedPanel(compartments, tc.mode, false)

			// Frame glyphs present.
			if !strings.ContainsAny(panel, "╭╮╰╯") {
				t.Errorf("panel must carry the rounded corner glyphs; got:\n%s", panel)
			}
			// Single-tone: every frame glyph is border.separator.
			sepSeq := tokenFgSeq(t, theme.MV.BorderSeparator, tc.mode)
			if !strings.Contains(panel, sepSeq) {
				t.Errorf("panel frame must be drawn in border.separator SGR core %q; missing in:\n%s", sepSeq, panel)
			}
			// NO border.footer hue. The two tokens collide in LIGHT mode (both
			// #C9CDDB per §2.9), so the discrimination is only meaningful in dark
			// (#292E42 separator vs #20232E footer) — mirror the help modal's
			// TestHelpModalDividerToken which checks dark only for the same reason.
			if tc.mode == theme.Dark {
				footerSeq := tokenFgSeq(t, theme.MV.BorderFooter, tc.mode)
				if strings.Contains(panel, footerSeq) {
					t.Errorf("single-tone panel must NOT use border.footer SGR core %q; found in:\n%s", footerSeq, panel)
				}
			}
		})
	}
}

// TestJoinedPanel_DividersJoinSideBorders asserts the helper interleaves a joined
// ├───┤ divider between EACH adjacent pair of compartments (two compartments → one
// divider, three → two), each running flush junction-to-junction (no inset gap), so
// they meet both side borders. There must be exactly len(compartments)-1 dividers.
func TestJoinedPanel_DividersJoinSideBorders(t *testing.T) {
	compartments := [][]string{
		{"header"},
		{"body"},
		{"footer"},
	}
	panel := renderJoinedPanel(compartments, theme.Dark, false)

	dividerCount := 0
	for _, raw := range strings.Split(panel, "\n") {
		line := strings.TrimSpace(ansi.Strip(raw))
		if strings.HasPrefix(line, helpFrameTeeLeft) && strings.HasSuffix(line, helpFrameTeeRight) {
			dividerCount++
			interior := strings.TrimSuffix(strings.TrimPrefix(line, helpFrameTeeLeft), helpFrameTeeRight)
			if interior == "" || strings.Trim(interior, helpRuleGlyph) != "" {
				t.Errorf("divider interior between ├ and ┤ must be all rule glyphs; got %q", interior)
			}
			if strings.HasPrefix(interior, " ") || strings.HasSuffix(interior, " ") {
				t.Errorf("divider must run flush junction-to-junction (no inset gap); interior = %q", interior)
			}
		}
	}
	if dividerCount != len(compartments)-1 {
		t.Errorf("panel must carry exactly %d joined dividers (between adjacent compartments); got %d", len(compartments)-1, dividerCount)
	}
}

// TestJoinedPanel_UniformWidth asserts every assembled frame line is exactly the
// same width — the frame columns align top to bottom (the pagination/alignment
// invariant the help modal relies on).
func TestJoinedPanel_UniformWidth(t *testing.T) {
	compartments := [][]string{
		{"a short row"},
		{"a much longer body row that sets the width"},
		{"foot"},
	}
	panel := renderJoinedPanel(compartments, theme.Dark, false)
	lines := strings.Split(panel, "\n")
	want := ansi.StringWidth(lines[0])
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != want {
			t.Errorf("frame line %d width = %d, want %d (uniform):\n%s", i, got, want, panel)
		}
	}
}

// TestJoinedPanel_RowsAreInsetDividersAreNot asserts content rows carry the L/R
// inset (helpRowInset) inside the side borders while the dividers run flush — the
// same inset contract the help modal uses.
func TestJoinedPanel_RowsAreInsetDividersAreNot(t *testing.T) {
	compartments := [][]string{
		{"title row"},
		{"content row"},
	}
	panel := renderJoinedPanel(compartments, theme.Dark, false)

	var contentRow string
	for _, raw := range strings.Split(panel, "\n") {
		line := ansi.Strip(raw)
		if strings.Contains(line, "content row") {
			contentRow = strings.TrimRight(line, " ")
			break
		}
	}
	if contentRow == "" {
		t.Fatalf("content row not found; panel:\n%s", panel)
	}
	l := strings.IndexRune(contentRow, '│')
	interior := contentRow[l+len("│"):]
	if !strings.HasPrefix(interior, strings.Repeat(" ", helpRowInset)) {
		t.Errorf("content row must carry the helpRowInset L inset inside the border; interior = %q", interior)
	}
}

// TestJoinedPanel_Colourless asserts the NO_COLOR carve-out: the frame keeps its
// glyphs but paints no border.separator hue (native fg).
func TestJoinedPanel_Colourless(t *testing.T) {
	compartments := [][]string{{"header"}, {"body"}, {"footer"}}
	panel := renderJoinedPanel(compartments, theme.Dark, true)
	if !strings.ContainsAny(panel, "╭╮╰╯├┤") {
		t.Errorf("colourless panel must keep the frame glyphs; got:\n%s", panel)
	}
	if seq := tokenFgSeq(t, theme.MV.BorderSeparator, theme.Dark); strings.Contains(panel, seq) {
		t.Errorf("colourless panel must NOT paint the border.separator hue %q", seq)
	}
}
