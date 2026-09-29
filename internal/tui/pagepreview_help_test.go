package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func newPreviewHelpModel(t *testing.T, th theme.Theme, colourless bool) previewModel {
	t.Helper()
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello scrollback line")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	m.th = th
	m.colourless = colourless
	return m
}

func pressPreviewKey(t *testing.T, m previewModel, msg tea.KeyPressMsg) (previewModel, tea.Cmd) {
	t.Helper()
	return m.Update(msg)
}

func keyQuestionMark() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: '?', Text: "?"}
}

func TestPreviewHelpOpensOnQuestionMark(t *testing.T) {
	m := newPreviewHelpModel(t, testDarkTheme(t), false)

	m, cmd := pressPreviewKey(t, m, keyQuestionMark())

	if cmd != nil {
		t.Errorf("? must not emit a cmd (no dismiss/attach); got non-nil")
	}
	if !m.helpOpen {
		t.Fatalf("? must open the preview help; helpOpen = false")
	}

	view := stripANSI(m.View())
	for _, action := range []string{
		"Scroll up / down",
		"Page up / down",
		"Prev / next window",
		"Next pane",
		"Attach this pane",
		"Back to sessions",
	} {
		if !strings.Contains(view, action) {
			t.Errorf("preview help must list %q (complete keymap from descriptor); missing in:\n%s", action, view)
		}
	}
}

func TestPreviewHelpOverlaysWithoutBlanking(t *testing.T) {
	m := newPreviewHelpModel(t, testDarkTheme(t), false)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())

	view := stripANSI(m.View())

	if !strings.Contains(view, "Keybindings") {
		t.Errorf("preview help overlay must show the 'Keybindings' header; missing in:\n%s", view)
	}
	if !strings.Contains(view, "esc close") {
		t.Errorf("preview help overlay must show the 'esc close' dismiss hint; missing in:\n%s", view)
	}
	if !strings.Contains(view, "◉ preview") {
		t.Errorf("preview help must OVERLAY (not blank) the preview; the ◉ preview marker must still be present:\n%s", view)
	}
	if !strings.Contains(view, "hello scrollback line") {
		t.Errorf("preview help must OVERLAY (not blank) the preview; the scrollback body must still be present:\n%s", view)
	}
}

func TestPreviewHelpReusesGenericRenderer(t *testing.T) {
	m := newPreviewHelpModel(t, testDarkTheme(t), false)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())

	view := stripANSI(m.View())
	panel := stripANSI(renderHelpModalContent(previewKeymap(), m.th, m.colourless, nil))

	for line := range strings.SplitSeq(panel, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(view, line) {
			t.Errorf("preview help overlay must composite the generic renderer's panel verbatim; line %q missing from:\n%s", line, view)
		}
	}
}

func TestPreviewHelpTogglesClosedOnSecondQuestionMark(t *testing.T) {
	m := newPreviewHelpModel(t, testDarkTheme(t), false)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())
	if !m.helpOpen {
		t.Fatalf("setup invariant: first ? must open help")
	}

	m, cmd := pressPreviewKey(t, m, keyQuestionMark())

	if m.helpOpen {
		t.Errorf("second ? must toggle the preview help closed; helpOpen = true")
	}
	if cmd != nil {
		t.Errorf("second ? must consume the key (no dismiss cmd); got non-nil")
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "Keybindings") {
		t.Errorf("the help panel must disappear after toggle-close; still present in:\n%s", view)
	}
}

func TestPreviewHelpEscDismissesWithoutBackingOut(t *testing.T) {
	m := newPreviewHelpModel(t, testDarkTheme(t), false)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())
	if !m.helpOpen {
		t.Fatalf("setup invariant: ? must open help")
	}

	m, cmd := pressPreviewKey(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})

	if m.helpOpen {
		t.Errorf("Esc must dismiss the help; helpOpen = true")
	}
	if cmd != nil {
		t.Fatalf("Esc on the help overlay must NOT emit a cmd (no previewDismissedMsg back-out); got non-nil")
	}
}

func TestPreviewHelpConsumesOtherKeysWhileOpen(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"left window", tea.KeyPressMsg{Code: tea.KeyLeft}},
		{"right window", tea.KeyPressMsg{Code: tea.KeyRight}},
		{"tab pane", tea.KeyPressMsg{Code: tea.KeyTab}},
		{"enter attach", tea.KeyPressMsg{Code: tea.KeyEnter}},
		{"up scroll", tea.KeyPressMsg{Code: tea.KeyUp}},
		{"down scroll", tea.KeyPressMsg{Code: tea.KeyDown}},
		{"home top", tea.KeyPressMsg{Code: tea.KeyHome}},
		{"end bottom", tea.KeyPressMsg{Code: tea.KeyEnd}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newPreviewHelpModel(t, testDarkTheme(t), false)
			m, _ = pressPreviewKey(t, m, keyQuestionMark())
			windowBefore, paneBefore := m.windowIdx, m.paneIdx

			m, cmd := pressPreviewKey(t, m, tc.msg)

			if !m.helpOpen {
				t.Errorf("a non-dismiss preview key must keep the help open; helpOpen = false")
			}
			if cmd != nil {
				t.Errorf("%s while help is open must be inert (no cmd); got non-nil", tc.name)
			}
			if m.windowIdx != windowBefore || m.paneIdx != paneBefore {
				t.Errorf("%s while help is open must not move focus; window %d→%d pane %d→%d",
					tc.name, windowBefore, m.windowIdx, paneBefore, m.paneIdx)
			}
		})
	}
}

func TestPreviewBackResumesWhenHelpClosed(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"esc back", tea.KeyPressMsg{Code: tea.KeyEscape}},
		{"space back", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newPreviewHelpModel(t, testDarkTheme(t), false)
			if m.helpOpen {
				t.Fatalf("setup invariant: help must start closed")
			}

			_, cmd := pressPreviewKey(t, m, tc.msg)

			if cmd == nil {
				t.Fatalf("%s with help closed must emit the preview-back cmd; got nil", tc.name)
			}
			if _, ok := cmd().(previewDismissedMsg); !ok {
				t.Errorf("%s with help closed must emit previewDismissedMsg", tc.name)
			}
		})
	}
}

func TestPreviewHelpRendersColourlessUnderNoColor(t *testing.T) {
	m := newPreviewHelpModel(t, testDarkTheme(t), true)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())

	view := m.View()
	stripped := stripANSI(view)

	if !strings.Contains(stripped, "Keybindings") {
		t.Errorf("colourless preview help must still show the header; missing in:\n%s", stripped)
	}
	if !strings.Contains(stripped, "◉ preview") {
		t.Errorf("colourless preview help must still overlay the preview marker; missing in:\n%s", stripped)
	}
	if strings.Contains(view, "\x1b[38;") {
		t.Errorf("colourless preview help carries a foreground colour SGR; must be colourless. view=%q", view)
	}
	if strings.Contains(view, "\x1b[48;") {
		t.Errorf("colourless preview help carries a background colour SGR; must be colourless. view=%q", view)
	}
}
