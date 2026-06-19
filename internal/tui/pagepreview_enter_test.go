package tui

import (
	"errors"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// newPreviewModelForEnter constructs a previewModel directly so the Enter
// branch in Update can be exercised against curated groups with a recorded
// PreviewAttacher injected, without going through NewPreviewModel's initial
// enumeration / read dance.
func newPreviewModelForEnter(session string, groups []tmux.WindowGroup, windowIdx, paneIdx int, reader ScrollbackReader, attacher PreviewAttacher, width, height int) previewModel {
	return previewModel{
		session:   session,
		reader:    reader,
		attacher:  attacher,
		groups:    groups,
		windowIdx: windowIdx,
		paneIdx:   paneIdx,
		viewport:  viewport.New(viewport.WithWidth(width), viewport.WithHeight(height)),
		width:     width,
		height:    height,
	}
}

func TestPreviewEnter_DispatchesWithCapturedRawIndicesWhenNoNavigation(t *testing.T) {
	// User opened preview on a session; never pressed ] / [ / Tab.
	// Enter must dispatch with the captured-at-open coordinates — the first
	// WindowGroup's WindowIndex and that group's first PaneIndex.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "other", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Fatalf("expected exactly 1 attacher.Run call, got %d", len(attacher.calls))
	}
	got := attacher.calls[0]
	want := recordedAttacherCall{session: "work", window: 0, pane: 0}
	if got != want {
		t.Errorf("attacher.Run called with %#v; want %#v", got, want)
	}
	if cmd == nil {
		t.Errorf("expected non-nil tea.Cmd returned from Enter, got nil")
	}
}

func TestPreviewEnter_DispatchesWithWalkedIndicesAfterTab(t *testing.T) {
	// User Tabs to next pane within the same window, then presses Enter.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1, 2}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)

	// Walk forward via Tab.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if updated.paneIdx != 1 {
		t.Fatalf("setup: expected paneIdx=1 after Tab, got %d", updated.paneIdx)
	}

	_, cmd := updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Fatalf("expected exactly 1 attacher.Run call, got %d", len(attacher.calls))
	}
	got := attacher.calls[0]
	want := recordedAttacherCall{session: "work", window: 0, pane: 1}
	if got != want {
		t.Errorf("attacher.Run called with %#v; want %#v (post-Tab walked pane)", got, want)
	}
	if cmd == nil {
		t.Errorf("expected non-nil tea.Cmd, got nil")
	}
}

func TestPreviewEnter_DispatchesWithWalkedIndicesAfterBracket(t *testing.T) {
	// User cycles forward to next window via `]`, then presses Enter.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0}},
		{WindowIndex: 2, WindowName: "third", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)

	// `]` advances windowIdx by 1 and resets paneIdx to 0.
	updated, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	if updated.windowIdx != 1 {
		t.Fatalf("setup: expected windowIdx=1 after `]`, got %d", updated.windowIdx)
	}

	_, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Fatalf("expected exactly 1 attacher.Run call, got %d", len(attacher.calls))
	}
	got := attacher.calls[0]
	want := recordedAttacherCall{session: "work", window: 1, pane: 0}
	if got != want {
		t.Errorf("attacher.Run called with %#v; want %#v (post-`]` walked window)", got, want)
	}
}

func TestPreviewEnter_DispatchesWithRawTmuxIndicesOnNonContiguousSession(t *testing.T) {
	// Session with non-contiguous WindowIndex and pane-base-index 1.
	// Spec § Captured coordinate values pins that raw tmux indices — not
	// 0-based slice positions — are passed to the pipeline.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{1}},
		{WindowIndex: 5, WindowName: "second", PaneIndices: []int{3}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
	// Cursor on second window (slice 1 → raw 5), only pane (slice 0 → raw 3).
	m := newPreviewModelForEnter("work", groups, 1, 0, reader, attacher, 80, 24)

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Fatalf("expected exactly 1 attacher.Run call, got %d", len(attacher.calls))
	}
	got := attacher.calls[0]
	want := recordedAttacherCall{session: "work", window: 5, pane: 3}
	if got != want {
		t.Errorf("attacher.Run called with %#v; want %#v (raw tmux indices, not slice positions)", got, want)
	}
}

func TestPreviewEnter_NotForwardedToViewport(t *testing.T) {
	// Pin that Enter does NOT reach the embedded viewport — the case must
	// return before viewport.Update. bubbles/viewport@v1.0.0 treats Enter as
	// a no-op for scrolling today, but the intercept must hold so any future
	// viewport binding on Enter cannot leak through preview.
	//
	// We exercise this by pre-scrolling the viewport to the top and asserting
	// the YOffset is untouched after Enter: if Enter were forwarded, a future
	// viewport binding to Enter that calls GotoBottom (or similar) would
	// mutate YOffset, while the intercept keeps it pinned.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 10)
	// Fill viewport with content larger than its height so scroll position
	// is meaningfully observable, then park at top.
	var lines string
	for i := 0; i < 50; i++ {
		lines += "line\n"
	}
	m.viewport.SetContent(lines)
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected viewport.AtTop, got YOffset=%d", m.viewport.YOffset())
	}
	prevYOffset := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if updated.viewport.YOffset() != prevYOffset {
		t.Errorf("viewport.YOffset = %d; want unchanged %d (Enter must not reach viewport)", updated.viewport.YOffset(), prevYOffset)
	}
	// Sanity: Enter was intercepted, not silently ignored — the attacher fired.
	if len(attacher.calls) != 1 {
		t.Errorf("expected attacher.Run to have fired (proof of interception), got %d calls", len(attacher.calls))
	}
}

func TestPreviewEnter_NoOpWhenAttacherIsNil(t *testing.T) {
	// Defensive guard: tests that construct preview without an attacher
	// (older callsites and any future ones that never wire the seam) must
	// receive a silent no-op for Enter, not a nil-deref panic.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, nil, 80, 24)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Enter with nil attacher panicked: %v", r)
		}
	}()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if cmd != nil {
		t.Errorf("expected nil cmd on nil-attacher no-op, got non-nil")
	}
	// Model is otherwise unchanged.
	if updated.windowIdx != m.windowIdx || updated.paneIdx != m.paneIdx {
		t.Errorf("expected windowIdx/paneIdx unchanged on nil-attacher no-op")
	}
}

func TestPreviewEnter_DispatchesWhenViewportHasRealBytes(t *testing.T) {
	// Spec § Mid-load: Enter attaches unconditionally regardless of viewport
	// content state. Real-bytes branch.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("real content bytes")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)
	// Simulate a real-bytes viewport by pre-loading content.
	m.viewport.SetContent("real content bytes")

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Errorf("expected attacher.Run to fire on real-bytes viewport, got %d calls", len(attacher.calls))
	}
}

func TestPreviewEnter_DispatchesWhenViewportRenderedPlaceholder(t *testing.T) {
	// Spec § Mid-load: (nil, nil) placeholder branch must NOT gate Enter.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	// reader returns (nil, nil) — the no-saved-content shape.
	reader := &recordingReader{bytes: nil, err: nil}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)
	// Simulate the placeholder render so the test honestly reflects the
	// observable viewport content state.
	m.viewport.SetContent(previewPlaceholder)

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Errorf("expected attacher.Run to fire on placeholder viewport, got %d calls", len(attacher.calls))
	}
}

func TestPreviewEnter_DispatchesWhenViewportRenderedReadError(t *testing.T) {
	// Spec § Mid-load: OS read-error branch must NOT gate Enter.
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: nil, err: errors.New("EACCES")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)
	// Simulate the error string render so the test honestly reflects the
	// observable viewport content state.
	m.viewport.SetContent(previewReadError)

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Errorf("expected attacher.Run to fire on read-error viewport, got %d calls", len(attacher.calls))
	}
}
