package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// newPreviewModelWithLines builds a previewModel whose viewport content is
// the given line count, sized to a viewport of width=80 and height=10 so
// content overflows and YOffset has room to move.
func newPreviewModelWithLines(t *testing.T, lineCount int) (previewModel, *recordingReader) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < lineCount; i++ {
		b.WriteString("line\n")
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte(b.String())}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 10)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	return m, reader
}

func TestPreviewScrollsDownOnDownKey(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)
	// GotoBottom in NewPreviewModel anchors at bottom; scroll up first so
	// Down has somewhere to go.
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected AtTop after GotoTop, got YOffset=%d", m.viewport.YOffset())
	}
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	if updated.viewport.YOffset() <= before {
		t.Errorf("expected YOffset to increase from %d after Down, got %d", before, updated.viewport.YOffset())
	}
}

func TestPreviewSilentNoOpScrollUpAtTop(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)
	m.viewport.GotoTop()
	if m.viewport.YOffset() != 0 {
		t.Fatalf("setup: expected YOffset=0 at top, got %d", m.viewport.YOffset())
	}

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})

	if updated.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset still 0 after Up at top, got %d", updated.viewport.YOffset())
	}
	if cmd != nil {
		t.Errorf("expected nil cmd on silent no-op Up at top, got non-nil")
	}
}

func TestPreviewSilentNoOpScrollDownAtBottom(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)

	// Drive End first to ensure we are at the bottom regardless of any
	// initial-anchor changes upstream.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !m.viewport.AtBottom() {
		t.Fatalf("setup: expected AtBottom after End, got YOffset=%d", m.viewport.YOffset())
	}
	before := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	if updated.viewport.YOffset() != before {
		t.Errorf("expected YOffset unchanged at bottom after Down, got %d (was %d)", updated.viewport.YOffset(), before)
	}
}

func TestPreviewHomeJumpsToTopViaPreviewOwnedBinding(t *testing.T) {
	m, _ := newPreviewModelWithLines(t, 50)
	// Page down a couple of times to push YOffset above 0.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	// If still at top after PgDn (e.g. content shorter than expected),
	// fail loudly so we don't inadvertently get a vacuously-passing test.
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

func TestPreviewWindowSizeMsgDoesNotCallTail(t *testing.T) {
	m, reader := newPreviewModelWithLines(t, 50)
	if len(reader.calls) != 1 {
		t.Fatalf("setup: expected 1 Tail call from initial-open, got %d", len(reader.calls))
	}

	_, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if len(reader.calls) != 1 {
		t.Errorf("expected Tail calls unchanged at 1 after WindowSizeMsg, got %d", len(reader.calls))
	}
}

func TestPreviewResizeDoesNotCallTailAcross100Events(t *testing.T) {
	m, reader := newPreviewModelWithLines(t, 50)
	if len(reader.calls) != 1 {
		t.Fatalf("setup: expected 1 Tail call from initial-open, got %d", len(reader.calls))
	}

	for i := 0; i < 100; i++ {
		// Vary dimensions slightly so each resize is a distinct value.
		m, _ = m.Update(tea.WindowSizeMsg{Width: 80 + i, Height: 24 + (i % 5)})
	}

	if len(reader.calls) != 1 {
		t.Errorf("expected Tail calls still 1 after 100 WindowSizeMsg events, got %d", len(reader.calls))
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

	// Resize to a height that comfortably accommodates a YOffset of 5
	// (200 lines minus a 20-line viewport leaves max YOffset of 180).
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	if updated.viewport.YOffset() != 5 {
		t.Errorf("expected YOffset preserved at 5 across accommodating resize, got %d", updated.viewport.YOffset())
	}
}

func TestPreviewEmptyContentDownIsNoOp(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: nil, err: nil}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("setup: expected ok=true on (nil, nil) Tail, got false")
	}
	if m.viewport.YOffset() != 0 {
		t.Fatalf("setup: expected YOffset=0 on empty content, got %d", m.viewport.YOffset())
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	if updated.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset=0 after Down on empty content, got %d", updated.viewport.YOffset())
	}

	updated2, _ := updated.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if updated2.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset=0 after Up on empty content, got %d", updated2.viewport.YOffset())
	}
}

func TestPreviewSingleLineContentDownIsNoOp(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("only one line\n")}
	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("setup: expected ok=true, got false")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	if updated.viewport.YOffset() != 0 {
		t.Errorf("expected YOffset=0 after Down on single-line content, got %d", updated.viewport.YOffset())
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
	// Already at bottom from initial-open anchor.
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
