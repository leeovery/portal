package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// renameModalContains reports whether the rendered rename modal contains substring
// s after stripping ANSI (a content presence check).
func renameModalContains(content, s string) bool {
	return strings.Contains(ansi.Strip(content), s)
}

// newRenameInput builds a focused, blink-disabled textinput pre-filled with value
// — the same input the rename modal styles + wraps. Blink off keeps the rendered
// cursor deterministic (a solid block, never a blinked-off gap).
func newRenameInput(value string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(value)
	ti.Focus()
	return ti
}

// TestRenameModal_Header asserts the §8.4 header `Rename session` rendered in
// text.primary (the non-destructive modal title colour).
func TestRenameModal_Header(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", mode, false)
		if !renameModalContains(content, "Rename session") {
			t.Errorf("[%v] header must read 'Rename session'; got:\n%s", mode, content)
		}
		if seq := tokenFgSeq(t, theme.MV.TextPrimary, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] header 'Rename session' must render in text.primary SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestRenameModal_NewNameLabel asserts the §8.4/§13.1 `NEW NAME` field label in
// accent.violet (the focused-field label colour — the input is the live editing
// element).
func TestRenameModal_NewNameLabel(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", mode, false)
		if !renameModalContains(content, "NEW NAME") {
			t.Errorf("[%v] body must contain the 'NEW NAME' label; got:\n%s", mode, content)
		}
		if seq := tokenFgSeq(t, theme.MV.AccentViolet, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] 'NEW NAME' label must render in accent.violet SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestRenameModal_InputValue asserts the typed value renders in text.primary inside
// the input box.
func TestRenameModal_InputValue(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", mode, false)
		if !renameModalContains(content, "aviva-proxy") {
			t.Errorf("[%v] input must contain the typed value 'aviva-proxy'; got:\n%s", mode, content)
		}
		if seq := tokenFgSeq(t, theme.MV.TextPrimary, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] input value must render in text.primary SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestRenameModal_VioletBlockCursor asserts the §13.1 input cursor is a violet
// block: the cursor renders Reverse over an accent.violet foreground (so the block
// fills violet), making the input the live editing element.
func TestRenameModal_VioletBlockCursor(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", mode, false)
		// The bubbles cursor renders its block via SGR 7 (reverse) over the cursor
		// colour foreground — assert both the reverse attr and the violet hue are
		// present so the block cursor is violet, not the default.
		if !strings.Contains(content, "\x1b[7m") && !strings.Contains(content, ";7m") && !strings.Contains(content, "[7;") {
			t.Errorf("[%v] input cursor must be a reverse block (SGR 7); got:\n%s", mode, content)
		}
		if seq := tokenFgSeq(t, theme.MV.AccentViolet, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] block cursor must carry accent.violet SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestRenameModal_VioletInputBoxOutline asserts the input value sits inside a
// border-defined box whose outline is accent.violet (§8.1: transparent fill, no
// recessed-input token — the outline is the only treatment).
func TestRenameModal_VioletInputBoxOutline(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", mode, false)
		// The input box draws its own rounded outline rows (corners + sides), distinct
		// from the panel frame. Find the row carrying the value, then assert its
		// preceding and following rows are box outline rows (top/bottom edges).
		lines := strings.Split(content, "\n")
		valueIdx := -1
		for i, raw := range lines {
			line := ansi.Strip(raw)
			// The value row carries the value AND an inner box side glyph (not the
			// panel side alone). Match on the value.
			if strings.Contains(line, "aviva-proxy") && !strings.Contains(line, "was:") {
				valueIdx = i
				break
			}
		}
		if valueIdx <= 0 || valueIdx >= len(lines)-1 {
			t.Fatalf("[%v] could not locate the input value row; content:\n%s", mode, content)
		}
		top := ansi.Strip(lines[valueIdx-1])
		bottom := ansi.Strip(lines[valueIdx+1])
		if !strings.ContainsAny(top, "╭─╮") {
			t.Errorf("[%v] row above the input value must be the box top edge (rounded outline); got %q", mode, top)
		}
		if !strings.ContainsAny(bottom, "╰─╯") {
			t.Errorf("[%v] row below the input value must be the box bottom edge (rounded outline); got %q", mode, bottom)
		}
		// The box outline is accent.violet.
		if seq := tokenFgSeq(t, theme.MV.AccentViolet, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] input box outline must render in accent.violet SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestRenameModal_WasLine asserts the `was: <old name>` context line renders in
// text.detail from the rename target.
func TestRenameModal_WasLine(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", mode, false)
		if !renameModalContains(content, "was: aviva-proxy-qNyfEO") {
			t.Errorf("[%v] body must contain 'was: aviva-proxy-qNyfEO'; got:\n%s", mode, content)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] 'was:' line must render in text.detail SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestRenameModal_Footer asserts the §8.4 footer `⏎ rename   esc cancel`: the ⏎ and
// esc key glyphs in accent.blue, the rename/cancel labels in text.detail. The
// dismiss key lives in the footer as `esc cancel` (§8.1).
func TestRenameModal_Footer(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", mode, false)
		for _, frag := range []string{"⏎ rename", "esc cancel"} {
			if !renameModalContains(content, frag) {
				t.Errorf("[%v] footer must contain %q; got:\n%s", mode, frag, content)
			}
		}
		if seq := tokenFgSeq(t, theme.MV.AccentBlue, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer key glyphs must render in accent.blue SGR core %q; missing in:\n%s", mode, seq, content)
		}
		if seq := tokenFgSeq(t, theme.MV.TextDetail, mode); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer labels must render in text.detail SGR core %q; missing in:\n%s", mode, seq, content)
		}
	}
}

// TestRenameModal_NoLitralEnterArrow asserts the footer uses the ⏎ glyph (matching
// the help modal + Projects footer), NOT the legacy ↵.
func TestRenameModal_NoLitralEnterArrow(t *testing.T) {
	content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", theme.Dark, false)
	if renameModalContains(content, "↵") {
		t.Errorf("footer must use ⏎ not the legacy ↵; got:\n%s", content)
	}
}

// TestRenameModal_SingleToneJoinedPanel asserts the rename modal composes through
// the shared single-tone joined panel: three compartments (header / body / footer)
// → two joined ├───┤ dividers, every frame glyph in border.separator.
func TestRenameModal_SingleToneJoinedPanel(t *testing.T) {
	content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", theme.Dark, false)

	dividerCount := 0
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(ansi.Strip(raw))
		if strings.HasPrefix(line, helpFrameTeeLeft) && strings.HasSuffix(line, helpFrameTeeRight) {
			dividerCount++
			interior := strings.TrimSuffix(strings.TrimPrefix(line, helpFrameTeeLeft), helpFrameTeeRight)
			if interior == "" || strings.Trim(interior, helpRuleGlyph) != "" {
				t.Errorf("divider interior must be all rule glyphs; got %q", interior)
			}
		}
	}
	if dividerCount != 2 {
		t.Errorf("rename modal must carry exactly 2 joined dividers (3 compartments); got %d", dividerCount)
	}
	// Single-tone: the panel frame uses border.separator, never border.footer.
	if seq := tokenFgSeq(t, theme.MV.BorderSeparator, theme.Dark); !strings.Contains(content, seq) {
		t.Errorf("rename modal frame must be drawn in border.separator SGR core %q; missing", seq)
	}
	if seq := tokenFgSeq(t, theme.MV.BorderFooter, theme.Dark); strings.Contains(content, seq) {
		t.Errorf("single-tone rename modal must NOT use border.footer SGR core %q", seq)
	}
}

// TestRenameModal_BodyLayout asserts the terminal-native body order: the NEW NAME
// label, then the input box (3 rows: top edge, value, bottom edge), then the `was:`
// context line — flush, no Paper px gaps.
func TestRenameModal_BodyLayout(t *testing.T) {
	content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", theme.Dark, false)
	lines := strings.Split(content, "\n")

	labelIdx, valueIdx, wasIdx := -1, -1, -1
	for i, raw := range lines {
		line := ansi.Strip(raw)
		if labelIdx < 0 && strings.Contains(line, "NEW NAME") {
			labelIdx = i
		}
		if valueIdx < 0 && strings.Contains(line, "aviva-proxy") && !strings.Contains(line, "was:") && !strings.Contains(line, "NEW NAME") {
			valueIdx = i
		}
		if wasIdx < 0 && strings.Contains(line, "was:") {
			wasIdx = i
		}
	}
	if labelIdx < 0 || valueIdx < 0 || wasIdx < 0 {
		t.Fatalf("could not locate label (%d) / value (%d) / was (%d) rows; content:\n%s", labelIdx, valueIdx, wasIdx, content)
	}
	// Order: label above value above was.
	if labelIdx >= valueIdx || valueIdx >= wasIdx {
		t.Errorf("body order must be NEW NAME → input box → was:; got label=%d value=%d was=%d", labelIdx, valueIdx, wasIdx)
	}
	// The value sits one row below the label's box top edge (label, box-top, value).
	if valueIdx-labelIdx != 2 {
		t.Errorf("input box top edge must sit directly under the NEW NAME label (value 2 rows below label); got %d rows", valueIdx-labelIdx)
	}
	// The was: line sits one row below the box bottom edge (value, box-bottom, was).
	if wasIdx-valueIdx != 2 {
		t.Errorf("was: line must sit directly under the input box bottom edge (was 2 rows below value); got %d rows", wasIdx-valueIdx)
	}
}

// TestRenameModal_Colourless asserts the §2.5 NO_COLOR carve-out: the structure
// survives (frame glyphs, the input box outline, the labels/copy) but no role hue
// is painted — every state reads from structure, not colour.
func TestRenameModal_Colourless(t *testing.T) {
	content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", theme.Dark, true)
	// The copy + structure survive.
	for _, frag := range []string{"Rename session", "NEW NAME", "aviva-proxy", "was: aviva-proxy-qNyfEO", "⏎ rename", "esc cancel"} {
		if !renameModalContains(content, frag) {
			t.Errorf("colourless rename modal must keep %q; got:\n%s", frag, content)
		}
	}
	// Frame + input box glyphs survive.
	if !strings.ContainsAny(content, "╭╮╰╯├┤") {
		t.Errorf("colourless rename modal must keep the frame/box glyphs; got:\n%s", content)
	}
	// No role hues painted: not accent.violet, accent.blue, text.detail, or
	// text.primary.
	for _, tok := range []theme.Token{theme.MV.AccentViolet, theme.MV.AccentBlue, theme.MV.TextDetail, theme.MV.TextPrimary, theme.MV.BorderSeparator} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(content, seq) {
			t.Errorf("colourless rename modal must NOT paint the %s hue %q", tok.Name, seq)
		}
	}
}

// TestRenameModal_LongOldNameTruncates asserts the edge case: a very long old name
// in the `was:` line truncates with an ellipsis so the panel doesn't overflow — the
// `was:` row width must not exceed the panel's body width.
func TestRenameModal_LongOldNameTruncates(t *testing.T) {
	longName := strings.Repeat("really-long-session-name-segment-", 6) + "end"
	content := renderRenameModalContent(newRenameInput("short"), longName, theme.Dark, false)
	lines := strings.Split(content, "\n")

	var wasLine string
	var frameWidth int
	for _, raw := range lines {
		line := ansi.Strip(raw)
		if frameWidth == 0 && strings.HasPrefix(strings.TrimSpace(line), helpFrameTopLeft) {
			frameWidth = len([]rune(strings.TrimSpace(line)))
		}
		if strings.Contains(line, "was:") {
			wasLine = line
		}
	}
	if wasLine == "" {
		t.Fatalf("could not locate the was: line; content:\n%s", content)
	}
	// The truncated old name must carry an ellipsis (it was too long to fit).
	if !strings.Contains(wasLine, "…") {
		t.Errorf("an over-long old name must be truncated with an ellipsis; got was line %q", wasLine)
	}
	// The full long name must NOT appear in full (it was truncated).
	if renameModalContains(content, longName) {
		t.Errorf("the full over-long old name must not render verbatim (it must truncate); got:\n%s", content)
	}
	// No rendered line exceeds the frame width (no overflow).
	for _, raw := range lines {
		w := len([]rune(ansi.Strip(raw)))
		if frameWidth > 0 && w > frameWidth {
			t.Errorf("no row may exceed the frame width %d; got width %d for %q", frameWidth, w, ansi.Strip(raw))
		}
	}
}

// TestUpdateRenameModal_EnterRenamesNonEmpty asserts parity: Enter with a trimmed
// non-empty name dispatches the rename (renameAndRefresh) — the modal closes and a
// command is returned that renames via the seam.
func TestUpdateRenameModal_EnterRenamesNonEmpty(t *testing.T) {
	rec := &recordingRenamer{}
	m := newRenameTestModel(rec, "aviva-proxy-qNyfEO", "  aviva-proxy  ")

	updated, cmd := m.updateRenameModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(Model)
	if um.modal != modalNone {
		t.Errorf("Enter on a non-empty name must close the modal; modal still %v", um.modal)
	}
	if cmd == nil {
		t.Fatalf("Enter on a non-empty name must return a rename command")
	}
	cmd() // execute the rename command to drive the seam.
	if rec.oldName != "aviva-proxy-qNyfEO" || rec.newName != "aviva-proxy" {
		t.Errorf("Enter must rename old=%q→new=%q (trimmed); got old=%q new=%q", "aviva-proxy-qNyfEO", "aviva-proxy", rec.oldName, rec.newName)
	}
}

// TestUpdateRenameModal_EnterEmptyIsNoOp asserts parity: Enter with an empty /
// whitespace-only trimmed name is a no-op — the modal stays open and no rename
// fires.
func TestUpdateRenameModal_EnterEmptyIsNoOp(t *testing.T) {
	rec := &recordingRenamer{}
	m := newRenameTestModel(rec, "aviva-proxy-qNyfEO", "   ")

	updated, cmd := m.updateRenameModal(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(Model)
	if um.modal != modalRename {
		t.Errorf("Enter on a whitespace-only name must keep the modal open; modal now %v", um.modal)
	}
	if cmd != nil {
		t.Errorf("Enter on a whitespace-only name must NOT return a command")
	}
	if rec.called {
		t.Errorf("Enter on a whitespace-only name must not rename")
	}
}

// TestUpdateRenameModal_EscCancels asserts parity: Esc cancels — the modal closes
// and no rename fires.
func TestUpdateRenameModal_EscCancels(t *testing.T) {
	rec := &recordingRenamer{}
	m := newRenameTestModel(rec, "aviva-proxy-qNyfEO", "aviva-proxy")

	updated, cmd := m.updateRenameModal(tea.KeyPressMsg{Code: tea.KeyEscape})
	um := updated.(Model)
	if um.modal != modalNone {
		t.Errorf("Esc must close the modal; modal still %v", um.modal)
	}
	if cmd != nil {
		t.Errorf("Esc must not return a command")
	}
	if rec.called {
		t.Errorf("Esc must not rename")
	}
}

// recordingRenamer is a SessionRenamer seam fake that records the rename call.
type recordingRenamer struct {
	called  bool
	oldName string
	newName string
}

func (r *recordingRenamer) RenameSession(oldName, newName string) error {
	r.called = true
	r.oldName = oldName
	r.newName = newName
	return nil
}

// stubLister is a SessionLister seam fake returning a fixed session set — wired so
// renameAndRefresh's post-rename re-list does not nil-panic.
type stubLister struct {
	sessions []tmux.Session
}

func (l stubLister) ListSessions() ([]tmux.Session, error) { return l.sessions, nil }

// newRenameTestModel builds a Model in the rename modal state with the given
// renamer seam, rename target, and pre-filled input value — the minimal shape
// updateRenameModal operates on.
func newRenameTestModel(renamer SessionRenamer, target, value string) Model {
	sessions := []tmux.Session{{Name: target, Windows: 1}}
	m := NewModelWithSessions(sessions)
	m.sessionRenamer = renamer
	m.sessionLister = stubLister{sessions: sessions}
	m.modal = modalRename
	m.renameTarget = target
	m.renameInput = newRenameInput(value)
	return m
}
