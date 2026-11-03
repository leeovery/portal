package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/backendtest"
)

func newPreviewModelWithLines(t *testing.T, lineCount int) (previewModel, *backendtest.Fake) {
	t.Helper()
	var b strings.Builder
	for range lineCount {
		b.WriteString("line\r\n")
	}
	reader := historyFake(t, b.String(), "work")
	m := openedPreview(t, "work", reader, 80, 10)
	return m, reader
}

func TestPreviewArrowsLeaveTheScrollbackAlone(t *testing.T) {
	for _, code := range []rune{tea.KeyUp, tea.KeyDown} {
		m, _ := newPreviewModelWithLines(t, 50)
		m.viewport.GotoTop()
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})
		before := m.viewport.YOffset()

		updated, _ := m.Update(tea.KeyPressMsg{Code: code})

		if updated.viewport.YOffset() != before {
			t.Errorf("arrow %d scrolled the scrollback from %d to %d; ↑/↓ step sessions in the preview", code, before, updated.viewport.YOffset())
		}
	}
}

func TestPreviewCtrlDownPagesDown(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 200)
	m.viewport.GotoTop()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})

	if got, want := updated.viewport.YOffset(), m.viewport.Height(); got != want {
		t.Errorf("YOffset after ^↓ = %d, want one page (%d)", got, want)
	}
}

func TestPreviewCtrlUpPagesUp(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 200)
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})

	if got, want := updated.viewport.YOffset(), before-m.viewport.Height(); got != want {
		t.Errorf("YOffset after ^↑ = %d, want one page up (%d)", got, want)
	}
}

func TestPreviewSilentNoOpScrollUpAtTop(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)
	m.viewport.GotoTop()
	if m.viewport.YOffset() != 0 {
		t.Fatalf("setup: expected YOffset=0 at top, got %d", m.viewport.YOffset())
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})

	if updated.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset still 0 after k at top, got %d", updated.viewport.YOffset())
	}
	if cmd != nil {
		t.Errorf("expected nil cmd on silent no-op k at top, got non-nil")
	}
}

func TestPreviewSilentNoOpScrollDownAtBottom(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !m.viewport.AtBottom() {
		t.Fatalf("setup: expected AtBottom after End, got YOffset=%d", m.viewport.YOffset())
	}
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	if updated.viewport.YOffset() != before {
		t.Errorf("expected YOffset unchanged at bottom after j, got %d (was %d)", updated.viewport.YOffset(), before)
	}
}

func TestPreviewHomeJumpsToTopViaPreviewOwnedBinding(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.viewport.YOffset() == 0 {
		t.Fatalf("setup: expected YOffset > 0 after PgDn, got 0")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyHome})

	if updated.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset=0 after Home, got %d", updated.viewport.YOffset())
	}
}

func TestPreviewEndJumpsToBottomViaPreviewOwnedBinding(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected AtTop, got YOffset=%d", m.viewport.YOffset())
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})

	if !updated.viewport.AtBottom() {
		t.Errorf("expected AtBottom after End, got YOffset=%d", updated.viewport.YOffset())
	}
}

func TestPreviewWindowSizeMsgDoesNotReadHistory(t *testing.T) {
	m, reader := newPreviewModelWithLines(t, 50)
	if len(reader.CallsTo(backendtest.OpHistory)) != 1 {
		t.Fatalf("setup: expected 1 History read from initial-open, got %d", len(reader.CallsTo(backendtest.OpHistory)))
	}

	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if len(reader.CallsTo(backendtest.OpHistory)) != 1 {
		t.Errorf("expected History reads unchanged at 1 after WindowSizeMsg, got %d", len(reader.CallsTo(backendtest.OpHistory)))
	}
}

func TestPreviewResizeDoesNotReadHistoryAcross100Events(t *testing.T) {
	m, reader := newPreviewModelWithLines(t, 50)
	if len(reader.CallsTo(backendtest.OpHistory)) != 1 {
		t.Fatalf("setup: expected 1 History read from initial-open, got %d", len(reader.CallsTo(backendtest.OpHistory)))
	}

	for i := range 100 {
		m, _ = m.Update(tea.WindowSizeMsg{Width: 80 + i, Height: 24 + (i % 5)})
	}

	if len(reader.CallsTo(backendtest.OpHistory)) != 1 {
		t.Errorf("expected History reads still 1 after 100 WindowSizeMsg events, got %d", len(reader.CallsTo(backendtest.OpHistory)))
	}
}

func TestPreviewWindowSizeMsgUpdatesViewportDimensions(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	wantWidth := 120 - previewFrameOverhead
	if updated.viewport.Width() != wantWidth {
		t.Errorf("expected viewport.Width=%d (msg.Width - previewFrameOverhead), got %d", wantWidth, updated.viewport.Width())
	}
	wantHeight := 40 - previewFrameOverhead
	if updated.viewport.Height() != wantHeight {
		t.Errorf("expected viewport.Height=%d (msg.Height - previewFrameOverhead), got %d", wantHeight, updated.viewport.Height())
	}
	if updated.width != 120 {
		t.Errorf("expected previewModel.width=120, got %d", updated.width)
	}
	if updated.height != 40 {
		t.Errorf("expected previewModel.height=40, got %d", updated.height)
	}
}

func TestPreviewPreservesScrollOffsetAcrossResizeWhenAccommodating(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 200)
	m.viewport.SetYOffset(5)
	if m.viewport.YOffset() != 5 {
		t.Fatalf("setup: expected YOffset=5, got %d", m.viewport.YOffset())
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	if updated.viewport.YOffset() != 5 {
		t.Errorf("expected YOffset preserved at 5 across accommodating resize, got %d", updated.viewport.YOffset())
	}
}

func TestPreviewEmptyContentDownIsNoOp(t *testing.T) {
	// A blank screen: nothing drawn but the cursor's position.
	m := newTestPreview(t, "\x1b[1;1H\x1b[0m", 80, 24)
	if m.viewport.YOffset() != 0 {
		t.Fatalf("setup: expected YOffset=0 on empty content, got %d", m.viewport.YOffset())
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	if updated.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset=0 after j on empty content, got %d", updated.viewport.YOffset())
	}

	updated2, _ := updated.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if updated2.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset=0 after k on empty content, got %d", updated2.viewport.YOffset())
	}
}

func TestPreviewSingleLineContentDownIsNoOp(t *testing.T) {
	m := newTestPreview(t, "only one line", 80, 24)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	if updated.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset=0 after j on single-line content, got %d", updated.viewport.YOffset())
	}
}

func TestPreviewPgDnDelegatesToViewport(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 200)
	m.viewport.GotoTop()
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})

	if updated.viewport.YOffset() <= before {
		t.Errorf("expected YOffset to increase after PgDn, got %d (was %d)", updated.viewport.YOffset(), before)
	}
}

func TestPreviewPgUpDelegatesToViewport(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 200)
	before := m.viewport.YOffset()
	if before == 0 {
		t.Fatalf("setup: expected YOffset > 0 (anchored at bottom), got 0")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})

	if updated.viewport.YOffset() >= before {
		t.Errorf("expected YOffset to decrease after PgUp, got %d (was %d)", updated.viewport.YOffset(), before)
	}
}

func TestPreviewCtrlDDelegatesToViewport(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 200)
	m.viewport.GotoTop()
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	if updated.viewport.YOffset() <= before {
		t.Errorf("expected YOffset to increase after ctrl-d, got %d (was %d)", updated.viewport.YOffset(), before)
	}
}

func TestPreviewCtrlUDelegatesToViewport(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 200)
	before := m.viewport.YOffset()
	if before == 0 {
		t.Fatalf("setup: expected YOffset > 0, got 0")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})

	if updated.viewport.YOffset() >= before {
		t.Errorf("expected YOffset to decrease after ctrl-u, got %d (was %d)", updated.viewport.YOffset(), before)
	}
}

func TestPreviewJDelegatesToViewport(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)
	m.viewport.GotoTop()
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})

	if updated.viewport.YOffset() <= before {
		t.Errorf("expected YOffset to increase after j, got %d (was %d)", updated.viewport.YOffset(), before)
	}
}

func TestPreviewKDelegatesToViewport(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)
	before := m.viewport.YOffset()
	if before == 0 {
		t.Fatalf("setup: expected YOffset > 0, got 0")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})

	if updated.viewport.YOffset() >= before {
		t.Errorf("expected YOffset to decrease after k, got %d (was %d)", updated.viewport.YOffset(), before)
	}
}
