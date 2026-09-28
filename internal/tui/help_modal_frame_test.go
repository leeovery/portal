package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func borderFgSeq(t *testing.T, tok theme.Token) string {
	t.Helper()
	return tokenFgSeq(t, tok)
}

func TestHelpModalPanelBorderColour(t *testing.T) {
	forEachBuiltinTheme(t, func(t *testing.T, th theme.Theme) {
		panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), nil, 100, 30, th, false)
		if seq := borderFgSeq(t, th.Border); !strings.Contains(panel, seq) {
			t.Errorf("help modal panel border must be the border-token SGR core %q (not white); missing in:\n%s", seq, panel)
		}
	})
}

func TestHelpModalDividerToken(t *testing.T) {
	content := renderHelpModalContent(sessionsKeymap(), testDarkTheme(t), false, nil)
	sepSeq := tokenFgSeq(t, testDarkTheme(t).Border)
	if !strings.Contains(content, sepSeq) {
		t.Errorf("help frame + divider must be drawn in the border SGR core %q; missing in:\n%s", sepSeq, content)
	}
	for line := range strings.SplitSeq(content, "\n") {
		if bare := strings.TrimSpace(ansi.Strip(line)); bare == "" || strings.Trim(bare, panelRuleGlyph+panelFrameTeeLeft+panelFrameTeeRight+"╭╮╰╯") != "" {
			continue
		}
		if !strings.Contains(line, sepSeq) {
			t.Errorf("frame-only line is not drawn in the border SGR core %q: %q", sepSeq, line)
		}
	}
}

func TestHelpModalDividerJoined(t *testing.T) {
	panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), nil, 120, 36, testDarkTheme(t), false)
	var dividerRow string
	for raw := range strings.SplitSeq(panel, "\n") {
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

func TestHelpModalDividerConnectsToBorders(t *testing.T) {
	panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), nil, 120, 36, testDarkTheme(t), false)
	var dividerRow string
	for raw := range strings.SplitSeq(panel, "\n") {
		line := strings.TrimSpace(ansi.Strip(raw))
		if strings.HasPrefix(line, panelFrameTeeLeft) && strings.HasSuffix(line, panelFrameTeeRight) {
			dividerRow = line
			break
		}
	}
	if dividerRow == "" {
		t.Fatalf("no divider row joining both side borders (├ … ┤) found; panel:\n%s", panel)
	}
	interior := strings.TrimSuffix(strings.TrimPrefix(dividerRow, panelFrameTeeLeft), panelFrameTeeRight)
	if strings.HasPrefix(interior, " ") || strings.HasSuffix(interior, " ") {
		t.Errorf("divider must run flush junction-to-junction (no inset gap); interior = %q", interior)
	}

	var bodyRow string
	for raw := range strings.SplitSeq(panel, "\n") {
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

func TestHelpModalFlushVerticalSpacing(t *testing.T) {
	panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), nil, 120, 40, testDarkTheme(t), false)
	lines := strings.Split(panel, "\n")

	topIdx := -1
	titleIdx := -1
	dividerIdx := -1
	for i, raw := range lines {
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

	above := titleIdx - topIdx - 1
	if above != 0 {
		t.Errorf("title must sit FLUSH under the top border (0 blanks above); got %d (rows:\n%s)", above, strings.Join(neighbourhood(lines, titleIdx), "\n"))
	}
	below := dividerIdx - titleIdx - 1
	if below != 0 {
		t.Errorf("divider must sit FLUSH under the title (0 blanks between); got %d (rows:\n%s)", below, strings.Join(neighbourhood(lines, titleIdx), "\n"))
	}

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

func neighbourhood(lines []string, idx int) []string {
	lo := max(idx-3, 0)
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

func TestHelpModalBodyContiguousRows(t *testing.T) {
	panel := renderHelpModalOnClearedCanvas(sessionsKeymap(), nil, 120, 40, testDarkTheme(t), false)
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
