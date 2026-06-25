package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// nextWindowKey and prevWindowKey are the §9.3 window-nav key shapes: `→` next
// window, `←` prev window (REPLACING the former `]`/`[`). They are the plain
// (un-modified) arrows — `Tab` drives pane nav instead.
var (
	nextWindowKey = tea.KeyPressMsg{Code: tea.KeyRight}
	prevWindowKey = tea.KeyPressMsg{Code: tea.KeyLeft}
)

func TestPreviewWindowNav_NextAdvancesWindowIdxByOneAndResetsPaneIdx(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1, 2}},
		{WindowIndex: 2, WindowName: "third", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 1, reader, 80, 24)

	updated, cmd := m.Update(nextWindowKey)

	if updated.windowIdx != 1 {
		t.Errorf("expected windowIdx=1 after →, got %d", updated.windowIdx)
	}
	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 after →, got %d", updated.paneIdx)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd after → (synchronous read), got non-nil")
	}
	if len(reader.calls) != 1 {
		t.Errorf("expected exactly 1 Tail call after →, got %d", len(reader.calls))
	}
}

func TestPreviewWindowNav_NextWrapsFromLastWindowToZero(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1}},
		{WindowIndex: 2, WindowName: "third", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 2, 0, reader, 80, 24)

	updated, _ := m.Update(nextWindowKey)

	if updated.windowIdx != 0 {
		t.Errorf("expected windowIdx=0 after → wrap, got %d", updated.windowIdx)
	}
	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 after → wrap, got %d", updated.paneIdx)
	}
}

func TestPreviewWindowNav_PrevRewindsWindowIdxByOneAndResetsPaneIdx(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1, 2}},
		{WindowIndex: 2, WindowName: "third", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 2, 0, reader, 80, 24)

	updated, cmd := m.Update(prevWindowKey)

	if updated.windowIdx != 1 {
		t.Errorf("expected windowIdx=1 after ←, got %d", updated.windowIdx)
	}
	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 after ←, got %d", updated.paneIdx)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd after ← (synchronous read), got non-nil")
	}
	if len(reader.calls) != 1 {
		t.Errorf("expected exactly 1 Tail call after ←, got %d", len(reader.calls))
	}
}

func TestPreviewWindowNav_PrevFromWindowZeroWrapsToLastWindow(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1}},
		{WindowIndex: 2, WindowName: "third", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, _ := m.Update(prevWindowKey)

	if updated.windowIdx != 2 {
		t.Errorf("expected windowIdx=2 after ← wrap from 0, got %d", updated.windowIdx)
	}
	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 after ← wrap, got %d", updated.paneIdx)
	}
}

func TestPreviewWindowNav_NextSingleWindowMultiPaneSessionIsSilentNoOp(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1, 2}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 1, reader, 80, 24)

	updated, cmd := m.Update(nextWindowKey)

	if updated.windowIdx != 0 {
		t.Errorf("expected windowIdx=0 unchanged in single-window session, got %d", updated.windowIdx)
	}
	if updated.paneIdx != 1 {
		t.Errorf("expected paneIdx=1 unchanged in single-window session no-op, got %d", updated.paneIdx)
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected zero Tail calls in single-window session, got %d", len(reader.calls))
	}
	if cmd != nil {
		t.Errorf("expected nil cmd in single-window session no-op, got non-nil")
	}
}

func TestPreviewWindowNav_PrevSingleWindowMultiPaneSessionIsSilentNoOp(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1, 2}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 1, reader, 80, 24)

	updated, cmd := m.Update(prevWindowKey)

	if updated.windowIdx != 0 {
		t.Errorf("expected windowIdx=0 unchanged in single-window session, got %d", updated.windowIdx)
	}
	if updated.paneIdx != 1 {
		t.Errorf("expected paneIdx=1 unchanged in single-window session no-op, got %d", updated.paneIdx)
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected zero Tail calls in single-window session, got %d", len(reader.calls))
	}
	if cmd != nil {
		t.Errorf("expected nil cmd in single-window session no-op, got non-nil")
	}
}

func TestPreviewWindowNav_WindowCycleResetsPaneIdxToZeroEvenWhenNonZero(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1, 2}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1, 2, 3}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	// Start mid-window, mid-pane (paneIdx=2).
	m := newPreviewModelForTab("work", groups, 0, 2, reader, 80, 24)

	updated, _ := m.Update(nextWindowKey)
	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 after → from non-zero paneIdx, got %d", updated.paneIdx)
	}

	// And in the reverse direction.
	m2 := newPreviewModelForTab("work", groups, 1, 3, reader, 80, 24)
	updated2, _ := m2.Update(prevWindowKey)
	if updated2.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 after ← from non-zero paneIdx, got %d", updated2.paneIdx)
	}
}

func TestPreviewWindowNav_WindowCycleTriggersExactlyOneTailCallWithPaneZeroOfNewWindow(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 2, WindowName: "second", PaneIndices: []int{4, 7, 9}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 1, reader, 80, 24)

	_, _ = m.Update(nextWindowKey)

	if len(reader.calls) != 1 {
		t.Fatalf("expected exactly 1 Tail call after →, got %d", len(reader.calls))
	}
	want := state.SanitizePaneKey("work", 2, 4)
	if reader.calls[0] != want {
		t.Errorf("expected Tail called with paneKey %q (raw window=2, raw pane=4 — pane 0 of new window), got %q", want, reader.calls[0])
	}
}

func TestPreviewWindowNav_WindowCycleResetsViewportScrollPositionToTail(t *testing.T) {
	// Build content larger than the viewport so AtBottom is non-trivial.
	var b strings.Builder
	for range 50 {
		b.WriteString("line\n")
	}
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte(b.String())}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 10)
	// Pre-load some content and scroll to top so a successful → must
	// explicitly call GotoBottom to satisfy AtBottom().
	m.viewport.SetContent("stale\nstale\nstale\n")
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected AtTop before →, got YOffset=%d", m.viewport.YOffset())
	}

	updated, _ := m.Update(nextWindowKey)

	if !updated.viewport.AtBottom() {
		t.Errorf("expected viewport.AtBottom()=true after → cycle, got YOffset=%d", updated.viewport.YOffset())
	}
}

// TestPreviewWindowNav_InterceptedBeforeViewportHorizontalScroll pins the §9.3
// validation caveat: bubbles/viewport binds plain `←`/`→` for horizontal scroll,
// so the preview MUST intercept them for window nav BEFORE delegating. A
// single-window session would otherwise let the arrow leak through to the
// viewport's horizontal scroll; here the multi-window fixture proves window nav
// won (windowIdx advanced, exactly one Tail).
func TestPreviewWindowNav_InterceptedBeforeViewportHorizontalScroll(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, cmd := m.Update(nextWindowKey)

	if updated.windowIdx != 1 {
		t.Errorf("expected windowIdx=1 (→ intercepted for window nav), got %d", updated.windowIdx)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd (synchronous read, intercepted before viewport), got non-nil")
	}
	if len(reader.calls) != 1 {
		t.Errorf("expected exactly 1 Tail call from window nav, got %d", len(reader.calls))
	}
}
