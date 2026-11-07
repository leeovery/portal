package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func deleteModalContains(content, s string) bool {
	return strings.Contains(ansi.Strip(content), s)
}

func TestDeleteModal_Header(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", th, false)
		if !deleteModalContains(content, "▲ Delete project?") {
			t.Errorf("[%v] header must read '▲ Delete project?'; got:\n%s", themeLabel(th), content)
		}
		if seq := tokenFgSeq(t, th.StateDestructive); !strings.Contains(content, seq) {
			t.Errorf("[%v] header ▲ + title must render in state.destructive SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestDeleteModal_BodyNameAndPath(t *testing.T) {
	const name = "flow-v1-api"
	const path = "/Users/leeovery/Code/fabric/flowv1/flow-v1-api"
	content := renderDeleteModalContent(name, path, testDarkTheme(t), false)

	if !deleteModalContains(content, name) {
		t.Errorf("body must contain the project name %q; got:\n%s", name, content)
	}
	if !deleteModalContains(content, path) {
		t.Errorf("body must contain the project path %q; got:\n%s", path, content)
	}
	nameLine, pathLine := -1, -1
	for i, raw := range strings.Split(content, "\n") {
		line := ansi.Strip(raw)
		if nameLine < 0 && strings.Contains(line, name) && !strings.Contains(line, path) {
			nameLine = i
		}
		if pathLine < 0 && strings.Contains(line, path) {
			pathLine = i
		}
	}
	if nameLine < 0 || pathLine < 0 {
		t.Fatalf("could not locate name (%d) / path (%d) rows; content:\n%s", nameLine, pathLine, content)
	}
	if pathLine <= nameLine {
		t.Errorf("path line (%d) must sit below the name line (%d)", pathLine, nameLine)
	}
}

func TestDeleteModal_BodyColourRoles(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", th, false)
		if seq := tokenFgSeq(t, th.StateDestructive); !strings.Contains(content, seq) {
			t.Errorf("[%v] project name must render in state.destructive SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(content, seq) {
			t.Errorf("[%v] path + consequence must render in text.muted SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestDeleteModal_ConsequenceLine(t *testing.T) {
	content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", testDarkTheme(t), false)

	for _, fragment := range []string{"Removes this project from Portal", "untouched."} {
		if !deleteModalContains(content, fragment) {
			t.Errorf("consequence line must contain %q; got:\n%s", fragment, content)
		}
	}
	if deleteModalContains(content, killConsequence) {
		t.Errorf("delete consequence must NOT read as kill's, ending the session (record-only); got:\n%s", content)
	}
	for _, fragment := range []string{"sessions", "files", "untouched"} {
		if !deleteModalContains(content, fragment) {
			t.Errorf("consequence must state %q are untouched; got:\n%s", fragment, content)
		}
	}
}

func TestDeleteModal_Footer(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", th, false)
		for _, frag := range []string{"y delete", "esc cancel"} {
			if !deleteModalContains(content, frag) {
				t.Errorf("[%v] footer must contain %q; got:\n%s", themeLabel(th), frag, content)
			}
		}
		if seq := tokenFgSeq(t, th.AccentKey); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer key glyphs must render in accent.key SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestDeleteModal_SingleToneJoinedPanel(t *testing.T) {
	content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", testDarkTheme(t), false)

	dividerCount := 0
	for raw := range strings.SplitSeq(content, "\n") {
		line := strings.TrimSpace(ansi.Strip(raw))
		if strings.HasPrefix(line, panelFrameTeeLeft) && strings.HasSuffix(line, panelFrameTeeRight) {
			dividerCount++
			interior := strings.TrimSuffix(strings.TrimPrefix(line, panelFrameTeeLeft), panelFrameTeeRight)
			if interior == "" || strings.Trim(interior, panelRuleGlyph) != "" {
				t.Errorf("divider interior must be all rule glyphs; got %q", interior)
			}
		}
	}
	if dividerCount != 2 {
		t.Errorf("delete modal must carry exactly 2 joined dividers (3 compartments); got %d", dividerCount)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).Border); !strings.Contains(content, seq) {
		t.Errorf("delete modal frame must be drawn in the border SGR core %q; missing", seq)
	}
}

func TestDeleteModal_Colourless(t *testing.T) {
	content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", testDarkTheme(t), true)
	if !deleteModalContains(content, "▲ Delete project?") {
		t.Errorf("colourless delete modal must keep the ▲ destructive glyph + title; got:\n%s", content)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).StateDestructive); strings.Contains(content, seq) {
		t.Errorf("colourless delete modal must NOT paint the state.destructive hue %q (state via glyph+bold, not colour)", seq)
	}
	if !strings.Contains(content, "\x1b[1m") {
		t.Errorf("colourless delete modal must carry bold (SGR 1) for destructive emphasis; got:\n%s", content)
	}
	if !strings.ContainsAny(content, "╭╮╰╯├┤") {
		t.Errorf("colourless delete modal must keep the frame glyphs; got:\n%s", content)
	}
}

func TestDeleteModal_LongPathTruncates(t *testing.T) {
	longPath := "/Users/leeovery/" + strings.Repeat("really-long-directory-segment/", 8) + "end"
	content := renderDeleteModalContent("flow-v1-api", longPath, testDarkTheme(t), false)
	lines := strings.Split(content, "\n")

	var pathLine string
	var frameWidth int
	for _, raw := range lines {
		line := ansi.Strip(raw)
		if frameWidth == 0 && strings.HasPrefix(strings.TrimSpace(line), panelFrameTopLeft) {
			frameWidth = len([]rune(strings.TrimSpace(line)))
		}
		if strings.Contains(line, "…") && strings.Contains(line, "Users") {
			pathLine = line
		}
	}
	if pathLine == "" {
		t.Fatalf("could not locate a truncated path line; content:\n%s", content)
	}
	if deleteModalContains(content, longPath) {
		t.Errorf("the full over-long path must not render verbatim (it must truncate); got:\n%s", content)
	}
	for _, raw := range lines {
		w := len([]rune(ansi.Strip(raw)))
		if frameWidth > 0 && w > frameWidth {
			t.Errorf("no row may exceed the frame width %d; got width %d for %q", frameWidth, w, ansi.Strip(raw))
		}
	}
}
