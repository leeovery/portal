package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// killModalContains reports whether the rendered kill modal contains substring s
// after stripping ANSI (a content presence check).
func killModalContains(content, s string) bool {
	return strings.Contains(ansi.Strip(content), s)
}

// TestKillModal_Header asserts the §8.3 header `▲ Kill session?` — the ▲ triangle
// glyph AND the title text both render in state.red (glyph + colour, §2.2).
func TestKillModal_Header(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderKillModalContent("aviva-proxy-qNyfEO", 1, mode, false)
		if !killModalContains(content, "▲ Kill session?") {
			t.Errorf("[%v] header must read '▲ Kill session?'; got:\n%s", mode, content)
		}
		// The ▲ and title both carry state.red.
		if seq := tokenFgSeq(t, theme.MV.StateRed, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] header ▲ + title must render in state.red SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestKillModal_BodyNameAndWindows asserts the body line 1: the session name in
// state.red, then `· N window(s)` in text.detail on the same line, with correct
// pluralisation.
func TestKillModal_BodyNameAndWindows(t *testing.T) {
	for _, tc := range []struct {
		name    string
		windows int
		want    string
	}{
		{"aviva-proxy-qNyfEO", 1, "· 1 window"},
		{"folio-Jiz4el", 4, "· 4 windows"},
		{"empty-defensive", 0, "· 0 windows"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			content := renderKillModalContent(tc.name, tc.windows, theme.Dark, false)
			if !killModalContains(content, tc.name) {
				t.Errorf("body must contain the session name %q; got:\n%s", tc.name, content)
			}
			if !killModalContains(content, tc.want) {
				t.Errorf("body must contain the window count %q; got:\n%s", tc.want, content)
			}
			// Name + count on the SAME line.
			var nameLine string
			for _, raw := range strings.Split(content, "\n") {
				line := ansi.Strip(raw)
				if strings.Contains(line, tc.name) {
					nameLine = line
					break
				}
			}
			if !strings.Contains(nameLine, tc.want) {
				t.Errorf("name and window count must share one line; name line = %q, want count %q", nameLine, tc.want)
			}
		})
	}
}

// TestKillModal_BodyColourRoles asserts the body colour wiring: the name in
// state.red, the `· N window(s)` count in text.detail, and the consequence line in
// text.detail.
func TestKillModal_BodyColourRoles(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderKillModalContent("aviva-proxy-qNyfEO", 1, mode, false)
		if seq := tokenFgSeq(t, theme.MV.StateRed, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] session name must render in state.red SGR core %q; missing in:\n%s", mode, seq, content)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] count + consequence must render in text.detail SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestKillModal_ConsequenceLine asserts the consequence copy renders (text.detail),
// the §8.3 "Ends the tmux session…" warning.
func TestKillModal_ConsequenceLine(t *testing.T) {
	content := renderKillModalContent("aviva-proxy-qNyfEO", 1, theme.Dark, false)
	// The full copy WRAPS across lines within the panel; assert the start and end
	// fragments (each landing within one wrapped line) are both present so the body
	// carries the whole sentence.
	for _, fragment := range []string{"Ends the tmux session", "undone."} {
		if !killModalContains(content, fragment) {
			t.Errorf("consequence line must contain %q; got:\n%s", fragment, content)
		}
	}
}

// TestKillModal_Footer asserts the §8.3 footer `y kill   esc cancel`: the y and esc
// key glyphs in accent.blue, the kill/cancel labels in text.detail. The dismiss key
// lives in the footer as `esc cancel` (§8.1).
func TestKillModal_Footer(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderKillModalContent("aviva-proxy-qNyfEO", 1, mode, false)
		for _, frag := range []string{"y kill", "esc cancel"} {
			if !killModalContains(content, frag) {
				t.Errorf("[%v] footer must contain %q; got:\n%s", mode, frag, content)
			}
		}
		// Key glyphs (y, esc) in accent.blue.
		if seq := tokenFgSeq(t, theme.MV.AccentBlue, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer key glyphs must render in accent.blue SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestKillModal_SingleToneJoinedPanel asserts the kill modal composes through the
// shared single-tone joined panel: the dividers start ├ and end ┤, every frame
// glyph in border.separator (the same frame the help modal uses). Three
// compartments → two dividers.
func TestKillModal_SingleToneJoinedPanel(t *testing.T) {
	content := renderKillModalContent("aviva-proxy-qNyfEO", 1, theme.Dark, false)

	dividerCount := 0
	for _, raw := range strings.Split(content, "\n") {
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
	// Single-tone: border.separator present, border.footer absent (dark-only check).
	if seq := tokenFgSeq(t, theme.MV.BorderSeparator, theme.Dark); !strings.Contains(content, seq) {
		t.Errorf("kill modal frame must be drawn in border.separator SGR core %q; missing", seq)
	}
	if seq := tokenFgSeq(t, theme.MV.BorderFooter, theme.Dark); strings.Contains(content, seq) {
		t.Errorf("single-tone kill modal must NOT use border.footer SGR core %q", seq)
	}
}

// TestKillModal_BodyRowLayout asserts the terminal-native body spacing: the
// name·count row, then ONE blank row, then the consequence line — the single blank
// separates the "what" from the "warning".
func TestKillModal_BodyRowLayout(t *testing.T) {
	content := renderKillModalContent("aviva-proxy-qNyfEO", 1, theme.Dark, false)
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
	// Exactly ONE blank row between them (consequence is two rows below name).
	if gap := consequenceIdx - nameIdx - 1; gap != 1 {
		t.Errorf("body must have exactly ONE blank row between name and consequence; got %d blank rows", gap)
	}
}

// TestKillModal_Colourless asserts the §2.5 NO_COLOR carve-out: destructive state
// is carried by the ▲ glyph + bold (NOT colour) — the frame keeps its glyphs but
// paints no state.red hue, and the title run is bold.
func TestKillModal_Colourless(t *testing.T) {
	content := renderKillModalContent("aviva-proxy-qNyfEO", 1, theme.Dark, true)
	// The ▲ destructive glyph survives.
	if !killModalContains(content, "▲ Kill session?") {
		t.Errorf("colourless kill modal must keep the ▲ destructive glyph + title; got:\n%s", content)
	}
	// No state.red hue painted.
	if seq := tokenFgSeq(t, theme.MV.StateRed, theme.Dark); strings.Contains(content, seq) {
		t.Errorf("colourless kill modal must NOT paint the state.red hue %q (state via glyph+bold, not colour)", seq)
	}
	// Bold carries the destructive emphasis (SGR 1 present).
	if !strings.Contains(content, "\x1b[1m") {
		t.Errorf("colourless kill modal must carry bold (SGR 1) for destructive emphasis; got:\n%s", content)
	}
	// And no background/foreground hue at all (native fg/bg) — the frame glyphs survive.
	if !strings.ContainsAny(content, "╭╮╰╯├┤") {
		t.Errorf("colourless kill modal must keep the frame glyphs; got:\n%s", content)
	}
}
