package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// newPreviewHelpModel constructs a single-window single-pane previewModel at a
// roomy 80x24 with canned scrollback, then assigns the resolved canvas mode +
// colourless flag the same way the Space handler propagates them. Used by the
// §8.5/§9.3 Preview `?` help-overlay tests.
func newPreviewHelpModel(t *testing.T, mode theme.Mode, colourless bool) previewModel {
	t.Helper()
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello scrollback line")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	m.mode = mode
	m.colourless = colourless
	return m
}

// pressPreviewKey feeds one key into the preview Update and returns the next
// previewModel plus the cmd it emitted.
func pressPreviewKey(t *testing.T, m previewModel, msg tea.KeyPressMsg) (previewModel, tea.Cmd) {
	t.Helper()
	return m.Update(msg)
}

func keyQuestionMark() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: '?', Text: "?"}
}

// TestPreviewHelpOpensOnQuestionMark asserts `?` toggles the preview help open
// and the View then lists the COMPLETE Preview keymap from the descriptor —
// scroll, ←→ window, Tab pane, Enter attach, Space/Esc back (§8.5/§12.1).
func TestPreviewHelpOpensOnQuestionMark(t *testing.T) {
	m := newPreviewHelpModel(t, theme.Dark, false)

	m, cmd := pressPreviewKey(t, m, keyQuestionMark())

	if cmd != nil {
		t.Errorf("? must not emit a cmd (no dismiss/attach); got non-nil")
	}
	if !m.helpOpen {
		t.Fatalf("? must open the preview help; helpOpen = false")
	}

	view := stripANSI(m.View())
	for _, action := range []string{
		"Scroll up / down",   // ↑/↓ scroll
		"Page up / down",     // ^↑/↓ page
		"Prev / next window", // ←→ window
		"Next pane",          // Tab pane
		"Attach this pane",   // Enter attach
		"Back to sessions",   // Space/Esc back
	} {
		if !strings.Contains(view, action) {
			t.Errorf("preview help must list %q (complete keymap from descriptor); missing in:\n%s", action, view)
		}
	}
}

// TestPreviewHelpOverlaysWithoutBlanking asserts the help OVERLAYS the preview —
// the preview chrome (the ◉ preview marker + the cyan border) is still present
// behind the help panel; it must NOT route through the blank-screen path.
func TestPreviewHelpOverlaysWithoutBlanking(t *testing.T) {
	m := newPreviewHelpModel(t, theme.Dark, false)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())

	view := stripANSI(m.View())

	// The help panel itself is present.
	if !strings.Contains(view, "Keybindings") {
		t.Errorf("preview help overlay must show the 'Keybindings' header; missing in:\n%s", view)
	}
	if !strings.Contains(view, "esc close") {
		t.Errorf("preview help overlay must show the 'esc close' dismiss hint; missing in:\n%s", view)
	}
	// The preview content/chrome is STILL visible behind the help (not blanked):
	// the ◉ preview marker and the cyan-bordered scrollback survive.
	if !strings.Contains(view, "◉ preview") {
		t.Errorf("preview help must OVERLAY (not blank) the preview; the ◉ preview marker must still be present:\n%s", view)
	}
	if !strings.Contains(view, "hello scrollback line") {
		t.Errorf("preview help must OVERLAY (not blank) the preview; the scrollback body must still be present:\n%s", view)
	}
}

// TestPreviewHelpReusesGenericRenderer asserts the overlay is the descriptor-
// driven generic renderer (renderHelpModalContent), not hand-authored Preview
// copy: the help panel content is byte-identical to renderHelpModalContent over
// the Preview descriptor.
func TestPreviewHelpReusesGenericRenderer(t *testing.T) {
	m := newPreviewHelpModel(t, theme.Dark, false)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())

	view := stripANSI(m.View())
	panel := stripANSI(renderHelpModalContent(previewKeymap(), m.mode, m.colourless))

	for _, line := range strings.Split(panel, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.Contains(view, line) {
			t.Errorf("preview help overlay must composite the generic renderer's panel verbatim; line %q missing from:\n%s", line, view)
		}
	}
}

// TestPreviewHelpTogglesClosedOnSecondQuestionMark asserts a second `?` closes
// the help.
func TestPreviewHelpTogglesClosedOnSecondQuestionMark(t *testing.T) {
	m := newPreviewHelpModel(t, theme.Dark, false)
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
	// With help closed, the overlay panel is gone.
	view := stripANSI(m.View())
	if strings.Contains(view, "Keybindings") {
		t.Errorf("the help panel must disappear after toggle-close; still present in:\n%s", view)
	}
}

// TestPreviewHelpEscDismissesWithoutBackingOut asserts Esc dismisses the help
// and does NOT trigger the preview-back path (no previewDismissedMsg).
func TestPreviewHelpEscDismissesWithoutBackingOut(t *testing.T) {
	m := newPreviewHelpModel(t, theme.Dark, false)
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

// TestPreviewHelpConsumesOtherKeysWhileOpen asserts every other preview key
// (scroll/window/pane/attach) is inert while the help is open.
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
			m := newPreviewHelpModel(t, theme.Dark, false)
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

// TestPreviewBackResumesWhenHelpClosed asserts normal Esc/Space back behaviour
// resumes when the help is closed — each emits a previewDismissedMsg.
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
			m := newPreviewHelpModel(t, theme.Dark, false)
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

// TestPreviewHelpRendersColourlessUnderNoColor asserts the help overlay renders
// colourless (no SGR foreground) under NO_COLOR, over a colourless preview.
func TestPreviewHelpRendersColourlessUnderNoColor(t *testing.T) {
	m := newPreviewHelpModel(t, theme.Dark, true)
	m, _ = pressPreviewKey(t, m, keyQuestionMark())

	view := m.View()
	stripped := stripANSI(view)

	// The help panel content survives, and the preview is still behind it.
	if !strings.Contains(stripped, "Keybindings") {
		t.Errorf("colourless preview help must still show the header; missing in:\n%s", stripped)
	}
	if !strings.Contains(stripped, "◉ preview") {
		t.Errorf("colourless preview help must still overlay the preview marker; missing in:\n%s", stripped)
	}
	// Colourless (§2.5): the composited overlay must carry NO foreground or
	// background colour SGR — the same carve-out the rest of the preview chrome
	// honours (bold/structure survives, hue does not).
	if strings.Contains(view, "\x1b[38;") {
		t.Errorf("colourless preview help carries a foreground colour SGR; must be colourless. view=%q", view)
	}
	if strings.Contains(view, "\x1b[48;") {
		t.Errorf("colourless preview help carries a background colour SGR; must be colourless. view=%q", view)
	}
}
