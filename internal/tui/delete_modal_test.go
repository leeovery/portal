package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// deleteModalContains reports whether the rendered delete modal contains substring
// s after stripping ANSI (a content presence check).
func deleteModalContains(content, s string) bool {
	return strings.Contains(ansi.Strip(content), s)
}

// TestDeleteModal_Header asserts the §8.6 header `▲ Delete project?` — the ▲
// triangle glyph AND the title text both render in state.red (glyph + colour,
// §2.2), mirroring the kill modal's destructive header treatment.
func TestDeleteModal_Header(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", mode, false)
		if !deleteModalContains(content, "▲ Delete project?") {
			t.Errorf("[%v] header must read '▲ Delete project?'; got:\n%s", mode, content)
		}
		// The ▲ and title both carry state.red.
		if seq := tokenFgSeq(t, theme.MV.StateRed, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] header ▲ + title must render in state.red SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestDeleteModal_BodyNameAndPath asserts the body: line 1 = the project name in
// state.red, line 2 = its path in text.detail, on separate lines.
func TestDeleteModal_BodyNameAndPath(t *testing.T) {
	const name = "flow-v1-api"
	const path = "/Users/leeovery/Code/fabric/flowv1/flow-v1-api"
	content := renderDeleteModalContent(name, path, theme.Dark, false)

	if !deleteModalContains(content, name) {
		t.Errorf("body must contain the project name %q; got:\n%s", name, content)
	}
	if !deleteModalContains(content, path) {
		t.Errorf("body must contain the project path %q; got:\n%s", path, content)
	}
	// Name and path on SEPARATE lines (name above path).
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

// TestDeleteModal_BodyColourRoles asserts the body colour wiring: the name in
// state.red, the path + consequence line in text.detail.
func TestDeleteModal_BodyColourRoles(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", mode, false)
		if seq := tokenFgSeq(t, theme.MV.StateRed, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] project name must render in state.red SGR core %q; missing in:\n%s", mode, seq, content)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] path + consequence must render in text.detail SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestDeleteModal_ConsequenceLine asserts the §8.6 RECORD-ONLY consequence copy
// renders (text.detail) and is DISTINCT from the kill modal's session-ending
// consequence: it must NOT mention ending the tmux session, and it MUST state the
// sessions/files are untouched (the disambiguation requirement — deleting a project
// record ≠ killing a session).
func TestDeleteModal_ConsequenceLine(t *testing.T) {
	content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", theme.Dark, false)

	// The full record-only copy WRAPS across lines within the panel; assert start and
	// end fragments are both present so the body carries the whole sentence.
	for _, fragment := range []string{"Removes this project from Portal", "untouched."} {
		if !deleteModalContains(content, fragment) {
			t.Errorf("consequence line must contain %q; got:\n%s", fragment, content)
		}
	}
	// DISTINCT from kill: must NOT carry the kill modal's session-ending warning.
	if deleteModalContains(content, "Ends the tmux session") {
		t.Errorf("delete consequence must NOT mention ending the tmux session (record-only); got:\n%s", content)
	}
	// And it must affirm sessions/files are untouched (the disambiguation).
	for _, fragment := range []string{"sessions", "files", "untouched"} {
		if !deleteModalContains(content, fragment) {
			t.Errorf("consequence must state %q are untouched; got:\n%s", fragment, content)
		}
	}
}

// TestDeleteModal_Footer asserts the §8.6 footer `y delete   esc cancel`: the y and
// esc key glyphs in accent.blue, the delete/cancel labels in text.detail. The
// dismiss key lives in the footer as `esc cancel` (§8.1).
func TestDeleteModal_Footer(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", mode, false)
		for _, frag := range []string{"y delete", "esc cancel"} {
			if !deleteModalContains(content, frag) {
				t.Errorf("[%v] footer must contain %q; got:\n%s", mode, frag, content)
			}
		}
		// Key glyphs (y, esc) in accent.blue.
		if seq := tokenFgSeq(t, theme.MV.AccentBlue, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer key glyphs must render in accent.blue SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestDeleteModal_SingleToneJoinedPanel asserts the delete modal composes through
// the shared single-tone joined panel: dividers start ├ / end ┤, every frame glyph
// in border.separator (the same frame the kill modal uses). Three compartments →
// two dividers. No fill (border.separator-only frame).
func TestDeleteModal_SingleToneJoinedPanel(t *testing.T) {
	content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", theme.Dark, false)

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
	// Single-tone: border.separator present, border.footer absent (dark-only check).
	if seq := tokenFgSeq(t, theme.MV.BorderSeparator, theme.Dark); !strings.Contains(content, seq) {
		t.Errorf("delete modal frame must be drawn in border.separator SGR core %q; missing", seq)
	}
	if seq := tokenFgSeq(t, theme.MV.BorderFooter, theme.Dark); strings.Contains(content, seq) {
		t.Errorf("single-tone delete modal must NOT use border.footer SGR core %q", seq)
	}
}

// TestDeleteModal_Colourless asserts the §2.5 NO_COLOR carve-out: destructive state
// is carried by the ▲ glyph + bold (NOT colour) — the frame keeps its glyphs but
// paints no state.red hue, and the title run is bold.
func TestDeleteModal_Colourless(t *testing.T) {
	content := renderDeleteModalContent("flow-v1-api", "/Users/leeovery/Code/fabric", theme.Dark, true)
	// The ▲ destructive glyph survives.
	if !deleteModalContains(content, "▲ Delete project?") {
		t.Errorf("colourless delete modal must keep the ▲ destructive glyph + title; got:\n%s", content)
	}
	// No state.red hue painted.
	if seq := tokenFgSeq(t, theme.MV.StateRed, theme.Dark); strings.Contains(content, seq) {
		t.Errorf("colourless delete modal must NOT paint the state.red hue %q (state via glyph+bold, not colour)", seq)
	}
	// Bold carries the destructive emphasis (SGR 1 present).
	if !strings.Contains(content, "\x1b[1m") {
		t.Errorf("colourless delete modal must carry bold (SGR 1) for destructive emphasis; got:\n%s", content)
	}
	// And the frame glyphs survive (native fg/bg).
	if !strings.ContainsAny(content, "╭╮╰╯├┤") {
		t.Errorf("colourless delete modal must keep the frame glyphs; got:\n%s", content)
	}
}

// TestDeleteModal_LongPathTruncates asserts the edge case: a very long project path
// truncates with an ellipsis so the panel doesn't overflow — no rendered row may
// exceed the frame width.
func TestDeleteModal_LongPathTruncates(t *testing.T) {
	longPath := "/Users/leeovery/" + strings.Repeat("really-long-directory-segment/", 8) + "end"
	content := renderDeleteModalContent("flow-v1-api", longPath, theme.Dark, false)
	lines := strings.Split(content, "\n")

	var pathLine string
	var frameWidth int
	for _, raw := range lines {
		line := ansi.Strip(raw)
		if frameWidth == 0 && strings.HasPrefix(strings.TrimSpace(line), panelFrameTopLeft) {
			frameWidth = len([]rune(strings.TrimSpace(line)))
		}
		// The truncated path line carries the ellipsis (it was too long to fit).
		if strings.Contains(line, "…") && strings.Contains(line, "Users") {
			pathLine = line
		}
	}
	if pathLine == "" {
		t.Fatalf("could not locate a truncated path line; content:\n%s", content)
	}
	// The full long path must NOT appear verbatim (it must truncate).
	if deleteModalContains(content, longPath) {
		t.Errorf("the full over-long path must not render verbatim (it must truncate); got:\n%s", content)
	}
	// No rendered line exceeds the frame width (no overflow).
	for _, raw := range lines {
		w := len([]rune(ansi.Strip(raw)))
		if frameWidth > 0 && w > frameWidth {
			t.Errorf("no row may exceed the frame width %d; got width %d for %q", frameWidth, w, ansi.Strip(raw))
		}
	}
}
