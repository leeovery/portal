package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// borderFgSeq returns the bare `38;2;r;g;b` foreground SGR parameter substring a
// border drawn in tok renders with. lipgloss paints a border glyph's colour as a
// FOREGROUND, so the panel frame's border.separator colour appears as that
// token's foreground SGR core in the rendered modal — the same probe shape as
// tokenFgSeq.
func borderFgSeq(t *testing.T, tok theme.Token, m theme.Mode) string {
	t.Helper()
	return tokenFgSeq(t, tok, m)
}

// TestHelpModalPanelBorderColour asserts FIX 3 for the help modal specifically:
// its own panel frame is drawn in border.separator (not white), dark + light.
func TestHelpModalPanelBorderColour(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), 100, 30, tc.mode, false)
			if seq := borderFgSeq(t, theme.MV.BorderSeparator, tc.mode); !strings.Contains(panel, seq) {
				t.Errorf("help modal panel border must be border.separator SGR core %q (not white); missing in:\n%s", seq, panel)
			}
		})
	}
}

// TestHelpModalDividerToken asserts the SINGLE-TONE frame: the header divider
// under `? Keybindings` (and the whole frame) is drawn in border.separator, NOT
// border.footer (the 2-tone footer leg was dropped). The two tokens are distinct
// in dark mode (#292E42 vs #20232E), so the discrimination is done in dark.
func TestHelpModalDividerToken(t *testing.T) {
	content := renderHelpModalContent(sessionsKeymap(), theme.Dark, false)
	sepSeq := tokenFgSeq(t, theme.MV.BorderSeparator, theme.Dark)
	if !strings.Contains(content, sepSeq) {
		t.Errorf("help frame + divider must be drawn in border.separator SGR core %q; missing in:\n%s", sepSeq, content)
	}
	// Single-tone: NO border.footer hue anywhere in the help panel.
	footerSeq := tokenFgSeq(t, theme.MV.BorderFooter, theme.Dark)
	if strings.Contains(content, footerSeq) {
		t.Errorf("single-tone help frame must NOT use border.footer SGR core %q; found in:\n%s", footerSeq, content)
	}
}

// TestHelpModalDividerJoined asserts the divider line visibly JOINS the side
// borders via real `├`/`┤` junctions (the hand-drawn frame). On the composed panel
// the divider line starts with `├` and ends with `┤`, with `─` between.
func TestHelpModalDividerJoined(t *testing.T) {
	panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), 120, 36, theme.Dark, false)
	var dividerRow string
	for _, raw := range strings.Split(panel, "\n") {
		// lipgloss.Place centres the panel, so trim BOTH the leading centring pad and
		// the trailing fill before inspecting the frame line.
		line := strings.TrimSpace(ansi.Strip(raw))
		if strings.HasPrefix(line, panelFrameTeeLeft) && strings.HasSuffix(line, panelFrameTeeRight) {
			dividerRow = line
			break
		}
	}
	if dividerRow == "" {
		t.Fatalf("no joined divider row (├ … ┤) found; panel:\n%s", panel)
	}
	interior := strings.TrimSuffix(strings.TrimPrefix(dividerRow, panelFrameTeeLeft), panelFrameTeeRight)
	if interior == "" || strings.Trim(interior, panelRuleGlyph) != "" {
		t.Errorf("divider interior between ├ and ┤ must be all rule glyphs; got %q", interior)
	}
}

// TestHelpModalDividerConnectsToBorders asserts the divider runs the FULL inner
// panel width so its `├`/`┤` tees join the left/right frame, while the header text
// and body rows keep their own L/R inset. Practically: on the composed panel, the
// divider line is `├` + rule glyphs + `┤` with no inset gap, whereas the header/
// body rows (between their `│` side borders) ARE inset.
func TestHelpModalDividerConnectsToBorders(t *testing.T) {
	panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), 120, 36, theme.Dark, false)
	// Find the divider row: starts `├`, ends `┤`, all rule glyphs between.
	var dividerRow string
	for _, raw := range strings.Split(panel, "\n") {
		line := strings.TrimSpace(ansi.Strip(raw))
		if strings.HasPrefix(line, panelFrameTeeLeft) && strings.HasSuffix(line, panelFrameTeeRight) {
			dividerRow = line
			break
		}
	}
	if dividerRow == "" {
		t.Fatalf("no divider row joining both side borders (├ … ┤) found; panel:\n%s", panel)
	}
	// The divider interior must contain NO leading/trailing space (flush to the
	// junctions), proving it reaches both sides.
	interior := strings.TrimSuffix(strings.TrimPrefix(dividerRow, panelFrameTeeLeft), panelFrameTeeRight)
	if strings.HasPrefix(interior, " ") || strings.HasSuffix(interior, " ") {
		t.Errorf("divider must run flush junction-to-junction (no inset gap); interior = %q", interior)
	}

	// And a body row IS inset (it has leading spaces inside the `│` side borders) —
	// proving the inset lives on the rows, not the divider.
	var bodyRow string
	for _, raw := range strings.Split(panel, "\n") {
		line := ansi.Strip(raw)
		if strings.Contains(line, "Move selection") {
			bodyRow = strings.TrimRight(line, " ")
			break
		}
	}
	if bodyRow == "" {
		t.Fatalf("no 'Move selection' body row found; panel:\n%s", panel)
	}
	bl := strings.IndexRune(bodyRow, '│')
	bodyInterior := bodyRow[bl+len("│"):]
	if !strings.HasPrefix(bodyInterior, " ") {
		t.Errorf("body row must carry an L inset inside the border; interior = %q", bodyInterior)
	}
}

// TestHelpModalFlushVerticalSpacing asserts the FLUSH layout (terminal-native,
// diverging from the Paper reference's px title padding): ZERO blank rows anywhere.
// The title line sits immediately adjacent to the top border line, and the divider
// line is the immediate next line after the title — no blank between any of:
// top-border / title / divider / first-body-row.
func TestHelpModalFlushVerticalSpacing(t *testing.T) {
	panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), 120, 40, theme.Dark, false)
	lines := strings.Split(panel, "\n")

	topIdx := -1
	titleIdx := -1
	dividerIdx := -1
	for i, raw := range lines {
		// lipgloss.Place centres the panel; trim the centring pad before matching frame
		// edges by prefix/suffix.
		line := strings.TrimSpace(ansi.Strip(raw))
		if topIdx < 0 && strings.HasPrefix(line, panelFrameTopLeft) && strings.HasSuffix(line, panelFrameTopRight) {
			topIdx = i
		}
		if titleIdx < 0 && strings.Contains(line, "Keybindings") {
			titleIdx = i
		}
		if dividerIdx < 0 && strings.HasPrefix(line, panelFrameTeeLeft) && strings.HasSuffix(line, panelFrameTeeRight) {
			dividerIdx = i
		}
	}
	if topIdx < 0 || titleIdx < 0 || dividerIdx < 0 {
		t.Fatalf("could not locate top (%d), title (%d), divider (%d) rows; panel:\n%s", topIdx, titleIdx, dividerIdx, panel)
	}

	// ZERO blank rows above the title: it sits immediately under the top border.
	above := titleIdx - topIdx - 1
	if above != 0 {
		t.Errorf("title must sit FLUSH under the top border (0 blanks above); got %d (rows:\n%s)", above, strings.Join(neighbourhood(lines, titleIdx), "\n"))
	}
	// ZERO blank rows between the title and the divider.
	below := dividerIdx - titleIdx - 1
	if below != 0 {
		t.Errorf("divider must sit FLUSH under the title (0 blanks between); got %d (rows:\n%s)", below, strings.Join(neighbourhood(lines, titleIdx), "\n"))
	}

	// And ZERO blanks between the divider and the first body row.
	firstBodyIdx := -1
	for i := dividerIdx + 1; i < len(lines); i++ {
		if strings.Contains(ansi.Strip(lines[i]), "Move selection") {
			firstBodyIdx = i
			break
		}
	}
	if firstBodyIdx < 0 {
		t.Fatalf("could not locate the first body row; panel:\n%s", panel)
	}
	if gap := firstBodyIdx - dividerIdx - 1; gap != 0 {
		t.Errorf("first body row must sit FLUSH under the divider (0 blanks between); got %d", gap)
	}
}

// neighbourhood returns the stripped panel lines bracketing idx for diagnostics.
func neighbourhood(lines []string, idx int) []string {
	lo := idx - 3
	if lo < 0 {
		lo = 0
	}
	hi := idx + 3
	if hi >= len(lines) {
		hi = len(lines) - 1
	}
	out := make([]string, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		out = append(out, ansi.Strip(lines[i]))
	}
	return out
}

// TestHelpModalBodyContiguousRows asserts FIX 5's rhythm clause: the keymap rows
// are CONTIGUOUS (1-row rhythm) — no blank rows between consecutive body entries
// (the project terminal-native convention, NOT the Paper px gaps). The rows for
// "Move selection" and the next entry "Next / prev page" sit on adjacent lines.
func TestHelpModalBodyContiguousRows(t *testing.T) {
	panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), 120, 40, theme.Dark, false)
	lines := strings.Split(panel, "\n")
	moveIdx, nextIdx := -1, -1
	for i, raw := range lines {
		line := ansi.Strip(raw)
		if moveIdx < 0 && strings.Contains(line, "Move selection") {
			moveIdx = i
		}
		if nextIdx < 0 && strings.Contains(line, "Next / prev page") {
			nextIdx = i
		}
	}
	if moveIdx < 0 || nextIdx < 0 {
		t.Fatalf("could not locate body rows; move=%d next=%d", moveIdx, nextIdx)
	}
	if nextIdx-moveIdx != 1 {
		t.Errorf("body rows must be contiguous (1-row rhythm); 'Move selection' at %d, 'Next / prev page' at %d (gap %d)", moveIdx, nextIdx, nextIdx-moveIdx)
	}
}
