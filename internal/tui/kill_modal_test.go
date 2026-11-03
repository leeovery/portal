package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func killModalContains(content, s string) bool {
	return strings.Contains(ansi.Strip(content), s)
}

func TestKillModal_Header(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderKillModalContent("aviva-proxy-qNyfEO", th, false)
		if !killModalContains(content, "▲ Kill session?") {
			t.Errorf("[%v] header must read '▲ Kill session?'; got:\n%s", themeLabel(th), content)
		}
		if seq := tokenFgSeq(t, th.StateDestructive); !strings.Contains(content, seq) {
			t.Errorf("[%v] header ▲ + title must render in state.destructive SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestKillModal_BodyNameStandsAlone(t *testing.T) {
	const name = "folio-Jiz4el"
	content := renderKillModalContent(name, testDarkTheme(t), false)
	for raw := range strings.SplitSeq(content, "\n") {
		line := ansi.Strip(raw)
		if !strings.Contains(line, name) {
			continue
		}
		interior := strings.TrimSpace(strings.Trim(line, "│"))
		if interior != name {
			t.Errorf("the name row must carry the session name alone; got %q", interior)
		}
		return
	}
	t.Fatalf("body must contain the session name %q; got:\n%s", name, content)
}

func TestKillModal_BodyColourRoles(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderKillModalContent("aviva-proxy-qNyfEO", th, false)
		if seq := tokenFgSeq(t, th.StateDestructive); !strings.Contains(content, seq) {
			t.Errorf("[%v] session name must render in state.destructive SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(content, seq) {
			t.Errorf("[%v] consequence must render in text.muted SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestKillModal_ConsequenceLine(t *testing.T) {
	content := renderKillModalContent("aviva-proxy-qNyfEO", testDarkTheme(t), false)
	for _, fragment := range []string{"Ends the tmux session", "undone."} {
		if !killModalContains(content, fragment) {
			t.Errorf("consequence line must contain %q; got:\n%s", fragment, content)
		}
	}
}

func TestKillModal_Footer(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderKillModalContent("aviva-proxy-qNyfEO", th, false)
		for _, frag := range []string{"y kill", "esc cancel"} {
			if !killModalContains(content, frag) {
				t.Errorf("[%v] footer must contain %q; got:\n%s", themeLabel(th), frag, content)
			}
		}
		if seq := tokenFgSeq(t, th.AccentKey); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer key glyphs must render in accent.key SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestKillModal_SingleToneJoinedPanel(t *testing.T) {
	content := renderKillModalContent("aviva-proxy-qNyfEO", testDarkTheme(t), false)

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
		t.Errorf("kill modal must carry exactly 2 joined dividers (3 compartments); got %d", dividerCount)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).Border); !strings.Contains(content, seq) {
		t.Errorf("kill modal frame must be drawn in the border SGR core %q; missing", seq)
	}
}

func TestKillModal_BodyRowLayout(t *testing.T) {
	content := renderKillModalContent("aviva-proxy-qNyfEO", testDarkTheme(t), false)
	lines := strings.Split(content, "\n")

	nameIdx, consequenceIdx := -1, -1
	for i, raw := range lines {
		line := ansi.Strip(raw)
		if nameIdx < 0 && strings.Contains(line, "aviva-proxy-qNyfEO") {
			nameIdx = i
		}
		if consequenceIdx < 0 && strings.Contains(line, "Ends the tmux session") {
			consequenceIdx = i
		}
	}
	if nameIdx < 0 || consequenceIdx < 0 {
		t.Fatalf("could not locate name (%d) / consequence (%d) rows; content:\n%s", nameIdx, consequenceIdx, content)
	}
	if gap := consequenceIdx - nameIdx - 1; gap != 1 {
		t.Errorf("body must have exactly ONE blank row between name and consequence; got %d blank rows", gap)
	}
}

func TestKillModal_Colourless(t *testing.T) {
	content := renderKillModalContent("aviva-proxy-qNyfEO", testDarkTheme(t), true)
	if !killModalContains(content, "▲ Kill session?") {
		t.Errorf("colourless kill modal must keep the ▲ destructive glyph + title; got:\n%s", content)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).StateDestructive); strings.Contains(content, seq) {
		t.Errorf("colourless kill modal must NOT paint the state.destructive hue %q (state via glyph+bold, not colour)", seq)
	}
	if !strings.Contains(content, "\x1b[1m") {
		t.Errorf("colourless kill modal must carry bold (SGR 1) for destructive emphasis; got:\n%s", content)
	}
	if !strings.ContainsAny(content, "╭╮╰╯├┤") {
		t.Errorf("colourless kill modal must keep the frame glyphs; got:\n%s", content)
	}
}
