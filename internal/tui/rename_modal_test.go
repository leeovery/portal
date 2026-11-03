package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func renameModalContains(content, s string) bool {
	return strings.Contains(ansi.Strip(content), s)
}

func newRenameInput(value string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(value)
	ti.Focus()
	return ti
}

func TestRenameModal_ByteExact(t *testing.T) {
	got := ansi.Strip(renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", testDarkTheme(t), false))
	want := "╭──────────────────────────────────────────────────╮\n" +
		"│  Rename session                     ◉ EDIT MODE  │\n" +
		"├──────────────────────────────────────────────────┤\n" +
		"│  NEW NAME                                        │\n" +
		"│  ╭──────────────────────────────────────────╮    │\n" +
		"│  │ aviva-proxy                              │    │\n" +
		"│  ╰──────────────────────────────────────────╯    │\n" +
		"│  was: aviva-proxy-qNyfEO                         │\n" +
		"├──────────────────────────────────────────────────┤\n" +
		"│  ⏎ rename   esc cancel                           │\n" +
		"╰──────────────────────────────────────────────────╯"
	if got != want {
		t.Errorf("render mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenameModal_Header(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		if !renameModalContains(content, "Rename session") {
			t.Errorf("[%v] header must read 'Rename session'; got:\n%s", themeLabel(th), content)
		}
		if seq := tokenFgSeq(t, th.TextPrimary); !strings.Contains(content, seq) {
			t.Errorf("[%v] header 'Rename session' must render in text.primary SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestRenameModal_EditModeBadge(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		if !renameModalContains(content, "◉ EDIT MODE") {
			t.Errorf("[%v] header must show the `◉ EDIT MODE` badge; got:\n%s", themeLabel(th), content)
		}
		seg := labelSegment(t, content, "EDIT MODE")
		if seq := tokenFgSeq(t, th.AccentAttention); !strings.Contains(seg, seq) {
			t.Errorf("[%v] `◉ EDIT MODE` badge must render in accent.attention SGR core %q; seg=%q", themeLabel(th), seq, seg)
		}
	}
}

func TestRenameModal_EditModeBadgeRightAligned(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		var headerLine string
		for line := range strings.SplitSeq(ansi.Strip(content), "\n") {
			if strings.Contains(line, "Rename session") {
				headerLine = line
				break
			}
		}
		if headerLine == "" {
			t.Fatalf("[%v] header line not found; content:\n%s", themeLabel(th), ansi.Strip(content))
		}
		trimmed := strings.TrimRight(headerLine, " │")
		if !strings.HasSuffix(trimmed, "◉ EDIT MODE") {
			t.Errorf("[%v] `◉ EDIT MODE` must be right-aligned (trailing) in the header; got line:\n%q", themeLabel(th), headerLine)
		}
		titleIdx := strings.Index(headerLine, "Rename session")
		badgeIdx := strings.Index(headerLine, "◉ EDIT MODE")
		if titleIdx < 0 || badgeIdx < 0 || badgeIdx <= titleIdx {
			t.Fatalf("[%v] header must read title then far-right badge; got:\n%q", themeLabel(th), headerLine)
		}
		gap := badgeIdx - (titleIdx + len("Rename session"))
		if gap < 10 {
			t.Errorf("[%v] badge must be far-right with a wide flexible gap after the title (gap=%d); got:\n%q", themeLabel(th), gap, headerLine)
		}
		if badgeIdx < len(headerLine)/2 {
			t.Errorf("[%v] badge must sit in the right half of the header (corner), idx=%d lineLen=%d; got:\n%q", themeLabel(th), badgeIdx, len(headerLine), headerLine)
		}
	}
}

func TestRenameModal_NewNameLabel(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		if !renameModalContains(content, "NEW NAME") {
			t.Errorf("[%v] body must contain the 'NEW NAME' label; got:\n%s", themeLabel(th), content)
		}
		if seq := tokenFgSeq(t, th.AccentPrimary); !strings.Contains(content, seq) {
			t.Errorf("[%v] 'NEW NAME' label must render in accent.primary SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestRenameModal_InputValue(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		if !renameModalContains(content, "aviva-proxy") {
			t.Errorf("[%v] input must contain the typed value 'aviva-proxy'; got:\n%s", themeLabel(th), content)
		}
		if seq := tokenFgSeq(t, th.TextPrimary); !strings.Contains(content, seq) {
			t.Errorf("[%v] input value must render in text.primary SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestRenameModal_OrangeBlockCursor(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		if !strings.Contains(content, "\x1b[7m") && !strings.Contains(content, ";7m") && !strings.Contains(content, "[7;") {
			t.Errorf("[%v] input cursor must be a reverse block (SGR 7); got:\n%s", themeLabel(th), content)
		}
		if seq := tokenFgSeq(t, th.AccentAttention); !strings.Contains(content, seq) {
			t.Errorf("[%v] block cursor must carry accent.attention SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestRenameModal_OrangeInputBoxOutline(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		lines := strings.Split(content, "\n")
		valueIdx := -1
		for i, raw := range lines {
			line := ansi.Strip(raw)
			if strings.Contains(line, "aviva-proxy") && !strings.Contains(line, "was:") {
				valueIdx = i
				break
			}
		}
		if valueIdx <= 0 || valueIdx >= len(lines)-1 {
			t.Fatalf("[%v] could not locate the input value row; content:\n%s", themeLabel(th), content)
		}
		top := ansi.Strip(lines[valueIdx-1])
		bottom := ansi.Strip(lines[valueIdx+1])
		if !strings.ContainsAny(top, "╭─╮") {
			t.Errorf("[%v] row above the input value must be the box top edge (rounded outline); got %q", themeLabel(th), top)
		}
		if !strings.ContainsAny(bottom, "╰─╯") {
			t.Errorf("[%v] row below the input value must be the box bottom edge (rounded outline); got %q", themeLabel(th), bottom)
		}
		if seq := tokenFgSeq(t, th.AccentAttention); !strings.Contains(content, seq) {
			t.Errorf("[%v] input box outline must render in accent.attention SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestRenameModal_WasLine(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		if !renameModalContains(content, "was: aviva-proxy-qNyfEO") {
			t.Errorf("[%v] body must contain 'was: aviva-proxy-qNyfEO'; got:\n%s", themeLabel(th), content)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(content, seq) {
			t.Errorf("[%v] 'was:' line must render in text.muted SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestRenameModal_Footer(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", th, false)
		for _, frag := range []string{"⏎ rename", "esc cancel"} {
			if !renameModalContains(content, frag) {
				t.Errorf("[%v] footer must contain %q; got:\n%s", themeLabel(th), frag, content)
			}
		}
		if seq := tokenFgSeq(t, th.AccentKey); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer key glyphs must render in accent.key SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
		if seq := tokenFgSeq(t, th.TextMuted); !strings.Contains(content, seq) {
			t.Errorf("[%v] footer labels must render in text.muted SGR core %q; missing in:\n%s", themeLabel(th), seq, content)
		}
	}
}

func TestRenameModal_NoLitralEnterArrow(t *testing.T) {
	content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", testDarkTheme(t), false)
	if renameModalContains(content, "↵") {
		t.Errorf("footer must use ⏎ not the legacy ↵; got:\n%s", content)
	}
}

func TestRenameModal_SingleToneJoinedPanel(t *testing.T) {
	content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", testDarkTheme(t), false)

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
		t.Errorf("rename modal must carry exactly 2 joined dividers (3 compartments); got %d", dividerCount)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).Border); !strings.Contains(content, seq) {
		t.Errorf("rename modal frame must be drawn in the border SGR core %q; missing", seq)
	}
}

func TestRenameModal_BodyLayout(t *testing.T) {
	content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", testDarkTheme(t), false)
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
	if labelIdx >= valueIdx || valueIdx >= wasIdx {
		t.Errorf("body order must be NEW NAME → input box → was:; got label=%d value=%d was=%d", labelIdx, valueIdx, wasIdx)
	}
	if valueIdx-labelIdx != 2 {
		t.Errorf("input box top edge must sit directly under the NEW NAME label (value 2 rows below label); got %d rows", valueIdx-labelIdx)
	}
	if wasIdx-valueIdx != 2 {
		t.Errorf("was: line must sit directly under the input box bottom edge (was 2 rows below value); got %d rows", wasIdx-valueIdx)
	}
}

func TestRenameModal_Colourless(t *testing.T) {
	content := renderRenameModalContent(newRenameInput("aviva-proxy"), "aviva-proxy-qNyfEO", testDarkTheme(t), true)
	for _, frag := range []string{"Rename session", "◉ EDIT MODE", "NEW NAME", "aviva-proxy", "was: aviva-proxy-qNyfEO", "⏎ rename", "esc cancel"} {
		if !renameModalContains(content, frag) {
			t.Errorf("colourless rename modal must keep %q; got:\n%s", frag, content)
		}
	}
	if !strings.ContainsAny(content, "╭╮╰╯├┤") {
		t.Errorf("colourless rename modal must keep the frame/box glyphs; got:\n%s", content)
	}
	for _, tok := range []theme.Token{testDarkTheme(t).AccentAttention, testDarkTheme(t).AccentPrimary, testDarkTheme(t).AccentKey, testDarkTheme(t).TextMuted, testDarkTheme(t).TextPrimary, testDarkTheme(t).Border} {
		if seq := tokenFgSeq(t, tok); strings.Contains(content, seq) {
			t.Errorf("colourless rename modal must NOT paint the %s hue %q", tok.Name, seq)
		}
	}
}

func TestRenameModal_LongOldNameTruncates(t *testing.T) {
	longName := strings.Repeat("really-long-session-name-segment-", 6) + "end"
	content := renderRenameModalContent(newRenameInput("short"), longName, testDarkTheme(t), false)
	lines := strings.Split(content, "\n")

	var wasLine string
	var frameWidth int
	for _, raw := range lines {
		line := ansi.Strip(raw)
		if frameWidth == 0 && strings.HasPrefix(strings.TrimSpace(line), panelFrameTopLeft) {
			frameWidth = len([]rune(strings.TrimSpace(line)))
		}
		if strings.Contains(line, "was:") {
			wasLine = line
		}
	}
	if wasLine == "" {
		t.Fatalf("could not locate the was: line; content:\n%s", content)
	}
	if !strings.Contains(wasLine, "…") {
		t.Errorf("an over-long old name must be truncated with an ellipsis; got was line %q", wasLine)
	}
	if renameModalContains(content, longName) {
		t.Errorf("the full over-long old name must not render verbatim (it must truncate); got:\n%s", content)
	}
	for _, raw := range lines {
		w := len([]rune(ansi.Strip(raw)))
		if frameWidth > 0 && w > frameWidth {
			t.Errorf("no row may exceed the frame width %d; got width %d for %q", frameWidth, w, ansi.Strip(raw))
		}
	}
}

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
	cmd()
	if rec.oldName != "aviva-proxy-qNyfEO" || rec.newName != "aviva-proxy" {
		t.Errorf("Enter must rename old=%q→new=%q (trimmed); got old=%q new=%q", "aviva-proxy-qNyfEO", "aviva-proxy", rec.oldName, rec.newName)
	}
}

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

type stubLister struct {
	sessions []Session
}

func (l stubLister) ListSessions() ([]Session, error) { return l.sessions, nil }

func newRenameTestModel(renamer SessionRenamer, target, value string) Model {
	sessions := []Session{{Name: target}}
	m := NewModelWithSessions(sessions)
	m.sessionRenamer = renamer
	m.sessionLister = stubLister{sessions: sessions}
	m.modal = modalRename
	m.renameTarget = target
	m.renameInput = newRenameInput(value)
	return m
}
