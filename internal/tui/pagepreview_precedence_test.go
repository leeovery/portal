package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

func newPreviewModelForPrecedence(t *testing.T) (previewModel, *recordingReader) {
	t.Helper()
	var b strings.Builder
	for range 50 {
		b.WriteString("line\n")
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
			{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		},
	}
	reader := &recordingReader{bytes: []byte(b.String())}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 10)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	return m, reader
}

func TestPreviewPrecedence_PaneNavDoesNotAdvanceViewportScrollOffset(t *testing.T) {
	m, _ := newPreviewModelForPrecedence(t)
	if !m.viewport.AtBottom() {
		t.Fatalf("setup: expected AtBottom after initial-open anchor, got YOffset=%d", m.viewport.YOffset())
	}

	updated, _ := m.Update(nextPaneKey)

	if !updated.viewport.AtBottom() {
		t.Errorf("expected AtBottom after Tab (preview-owned: post-read tail anchor), got YOffset=%d — viewport may have seen Tab", updated.viewport.YOffset())
	}
}

func TestPreviewPrecedence_NextWindowDoesNotAdvanceViewportScrollOffset(t *testing.T) {
	m, _ := newPreviewModelForPrecedence(t)
	if !m.viewport.AtBottom() {
		t.Fatalf("setup: expected AtBottom after initial-open anchor, got YOffset=%d", m.viewport.YOffset())
	}

	updated, _ := m.Update(nextWindowKey)

	if !updated.viewport.AtBottom() {
		t.Errorf("expected AtBottom after → (preview-owned: post-read tail anchor), got YOffset=%d — viewport may have seen →", updated.viewport.YOffset())
	}
}

func TestPreviewPrecedence_PrevWindowDoesNotAdvanceViewportScrollOffset(t *testing.T) {
	m, _ := newPreviewModelForPrecedence(t)
	if !m.viewport.AtBottom() {
		t.Fatalf("setup: expected AtBottom after initial-open anchor, got YOffset=%d", m.viewport.YOffset())
	}

	updated, _ := m.Update(prevWindowKey)

	if !updated.viewport.AtBottom() {
		t.Errorf("expected AtBottom after ← (preview-owned: post-read tail anchor), got YOffset=%d — viewport may have seen ←", updated.viewport.YOffset())
	}
}

func TestPreviewPrecedence_UpScrollsViewportUpwardPassthroughPreserved(t *testing.T) {
	m, _ := newPreviewModelForPrecedence(t)
	before := m.viewport.YOffset()
	if before == 0 {
		t.Fatalf("setup: expected YOffset > 0 (anchored at bottom), got 0 — Up has nowhere to go")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	if updated.viewport.YOffset() >= before {
		t.Errorf("expected YOffset to decrease after Up (passthrough to viewport), got %d (was %d)", updated.viewport.YOffset(), before)
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

func TestPreviewPrecedence_SinglePaneNavProducesExactlyOneTailCallNoDoubleHandling(t *testing.T) {
	m, reader := newPreviewModelForPrecedence(t)
	reader.calls = nil

	_, _ = m.Update(nextPaneKey)

	if len(reader.calls) != 1 {
		t.Errorf("expected exactly 1 Tail call from a single Tab keypress (no double-handling), got %d", len(reader.calls))
	}
}

func TestPreviewPrecedence_NonKeyMsgFallsThroughToViewport(t *testing.T) {
	m, reader := newPreviewModelForPrecedence(t)
	reader.calls = nil
	beforeYOffset := m.viewport.YOffset()
	beforeWindow := m.windowIdx
	beforePane := m.paneIdx

	type customMsg struct{}
	updated, _ := m.Update(customMsg{})

	if updated.windowIdx != beforeWindow {
		t.Errorf("expected windowIdx unchanged on custom msg, got %d (was %d)", updated.windowIdx, beforeWindow)
	}
	if updated.paneIdx != beforePane {
		t.Errorf("expected paneIdx unchanged on custom msg, got %d (was %d)", updated.paneIdx, beforePane)
	}
	if updated.viewport.YOffset() != beforeYOffset {
		t.Errorf("expected viewport.YOffset unchanged on custom msg (no scroll keys bound), got %d (was %d)", updated.viewport.YOffset(), beforeYOffset)
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected zero Tail calls on custom msg (no preview branch fires), got %d", len(reader.calls))
	}
}
