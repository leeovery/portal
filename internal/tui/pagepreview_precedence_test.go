package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/backendtest"
)

func newPreviewModelForPrecedence(t *testing.T) (previewModel, *backendtest.Fake) {
	t.Helper()
	var b strings.Builder
	for range 50 {
		b.WriteString("line\r\n")
	}
	reader := historyFake(t, b.String(), "work")
	m := openedPreview(t, "work", reader, 80, 10)
	return m, reader
}

func TestPreviewPrecedence_UpStepsSessionWithoutReachingViewport(t *testing.T) {
	m, reader := newPreviewModelForPrecedence(t)
	before := m.viewport.YOffset()
	if before == 0 {
		t.Fatalf("setup: expected YOffset > 0 (anchored at bottom), got 0 — a leak to the viewport would be invisible")
	}
	readsBefore := len(reader.CallsTo(backendtest.OpHistory))

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	if updated.viewport.YOffset() != before {
		t.Errorf("Up reached the viewport: YOffset %d → %d", before, updated.viewport.YOffset())
	}
	if len(reader.CallsTo(backendtest.OpHistory)) != readsBefore {
		t.Errorf("Up re-read the history in the preview itself; the parent owns the session step")
	}
	if cmd == nil {
		t.Fatal("Up emitted no cmd; want a previewStepSessionMsg")
	}
	if step, ok := cmd().(previewStepSessionMsg); !ok || step.delta != -1 {
		t.Errorf("Up emitted %#v, want previewStepSessionMsg{delta: -1}", step)
	}
}

func TestPreviewPrecedence_PgDnScrollsViewportDownwardPassthroughPreserved(t *testing.T) {
	m, _ := newPreviewModelForPrecedence(t)
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected AtTop after GotoTop, got YOffset=%d", m.viewport.YOffset())
	}
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})

	if updated.viewport.YOffset() <= before {
		t.Errorf("expected YOffset to increase after PgDn (passthrough to viewport), got %d (was %d)", updated.viewport.YOffset(), before)
	}
}

func TestPreviewPrecedence_JKVimStylePassthroughPreserved(t *testing.T) {
	m, _ := newPreviewModelForPrecedence(t)
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected AtTop, got YOffset=%d", m.viewport.YOffset())
	}
	beforeJ := m.viewport.YOffset()

	afterJ, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if afterJ.viewport.YOffset() <= beforeJ {
		t.Errorf("expected YOffset to increase after j (vim-style passthrough), got %d (was %d)", afterJ.viewport.YOffset(), beforeJ)
	}

	beforeK := afterJ.viewport.YOffset()
	afterK, _ := afterJ.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if afterK.viewport.YOffset() >= beforeK {
		t.Errorf("expected YOffset to decrease after k (vim-style passthrough), got %d (was %d)", afterK.viewport.YOffset(), beforeK)
	}
}

func TestPreviewPrecedence_WindowSizeMsgStillReachesViewportForReflow(t *testing.T) {
	m, _ := newPreviewModelForPrecedence(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 42})

	wantWidth := 132 - previewFrameOverhead
	if updated.viewport.Width() != wantWidth {
		t.Errorf("expected viewport.Width=%d (msg.Width - previewFrameOverhead) after WindowSizeMsg, got %d", wantWidth, updated.viewport.Width())
	}
	wantHeight := 42 - previewFrameOverhead
	if updated.viewport.Height() != wantHeight {
		t.Errorf("expected viewport.Height=%d (msg.Height - previewFrameOverhead) after WindowSizeMsg, got %d", wantHeight, updated.viewport.Height())
	}
}

func TestPreviewPrecedence_NonKeyMsgFallsThroughToViewport(t *testing.T) {
	m, reader := newPreviewModelForPrecedence(t)
	beforeYOffset := m.viewport.YOffset()

	type customMsg struct{}
	updated, _ := m.Update(customMsg{})

	if updated.viewport.YOffset() != beforeYOffset {
		t.Errorf("expected viewport.YOffset unchanged on custom msg (no scroll keys bound), got %d (was %d)", updated.viewport.YOffset(), beforeYOffset)
	}
	if n := len(reader.CallsTo(backendtest.OpHistory)); n != 1 {
		t.Errorf("expected only the open's History read after a custom msg, got %d", n)
	}
}

// Window and pane cycling are gone: ←/→ and Tab are not the preview's own.
func TestPreviewPrecedence_ArrowsAndTabAreNotPreviewKeys(t *testing.T) {
	m, _ := newPreviewModelForPrecedence(t)
	for _, code := range []rune{tea.KeyLeft, tea.KeyRight, tea.KeyTab} {
		if handled, _, _ := m.handlePreviewKey(tea.KeyPressMsg{Code: code}); handled {
			t.Errorf("key %d is still preview-owned; window and pane cycling are gone", code)
		}
	}
}

// The body is the session's screen clipped at the panel's width, as a terminal
// that size would clip it, so nothing pans it sideways: not the viewport's own
// ←/→/h/l, and not the wheel, sideways or shifted.
func TestPreviewPrecedence_NothingPansTheBodySideways(t *testing.T) {
	wide := strings.Repeat("0123456789", 30)
	inputs := map[string]tea.Msg{
		"←":                  tea.KeyPressMsg{Code: tea.KeyLeft},
		"→":                  tea.KeyPressMsg{Code: tea.KeyRight},
		"h":                  tea.KeyPressMsg{Code: 'h', Text: "h"},
		"l":                  tea.KeyPressMsg{Code: 'l', Text: "l"},
		"wheel left":         tea.MouseWheelMsg{Button: tea.MouseWheelLeft},
		"wheel right":        tea.MouseWheelMsg{Button: tea.MouseWheelRight},
		"shifted wheel down": tea.MouseWheelMsg{Button: tea.MouseWheelDown, Mod: tea.ModShift},
		"shifted wheel up":   tea.MouseWheelMsg{Button: tea.MouseWheelUp, Mod: tea.ModShift},
	}
	for name, msg := range inputs {
		t.Run(name, func(t *testing.T) {
			m := newTestPreview(t, wide, 80, 10)
			// Off the left edge, so a pan either way would show.
			m.viewport.SetXOffset(5)

			updated, _ := m.Update(msg)

			if got := updated.viewport.XOffset(); got != 5 {
				t.Errorf("XOffset = %d after %s, want 5: the body panned sideways", got, name)
			}
		})
	}
}
