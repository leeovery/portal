package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// nextPaneKey is the §9.3 pane-nav key shape: `Tab` cycles forward through panes
// (wrapping). This REPLACES the former `Ctrl+←`/`Ctrl+→` pair — `Ctrl+←/→` is
// hijacked by macOS Mission Control Spaces switching, so pane reverts to the
// pre-rebuild `Tab` forward-cycle.
var nextPaneKey = tea.KeyPressMsg{Code: tea.KeyTab}

// newPreviewModelForTab constructs a previewModel directly (bypassing the
// initial-open enumeration / read in NewPreviewModel) wired with a reader and
// a sized viewport so nav handling can be exercised against curated groups.
// The constructor's initial Tail call is intentionally not made here — call
// counts on the reader thus reflect *only* the operations under test.
func newPreviewModelForTab(session string, groups []tmux.WindowGroup, windowIdx, paneIdx int, reader ScrollbackReader, width, height int) previewModel {
	return previewModel{
		session:   session,
		reader:    reader,
		groups:    groups,
		windowIdx: windowIdx,
		paneIdx:   paneIdx,
		viewport:  viewport.New(viewport.WithWidth(width), viewport.WithHeight(height)),
		width:     width,
		height:    height,
	}
}

func TestPreviewPaneNav_NextAdvancesPaneIdxByOneWithinMultiPaneWindow(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1, 2}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, cmd := m.Update(nextPaneKey)

	if updated.paneIdx != 1 {
		t.Errorf("expected paneIdx=1 after Tab, got %d", updated.paneIdx)
	}
	if cmd != nil {
		t.Errorf("expected nil cmd after Tab (synchronous read), got non-nil")
	}
}

func TestPreviewPaneNav_NextAdvancesAcrossSuccessivePanes(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1, 2}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	m, _ = m.Update(nextPaneKey)
	m, _ = m.Update(nextPaneKey)

	if m.paneIdx != 2 {
		t.Errorf("expected paneIdx=2 after two Tab presses, got %d", m.paneIdx)
	}
}

func TestPreviewPaneNav_NextWrapsFromLastPaneBackToZeroWithinSameWindow(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1, 2}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 2, reader, 80, 24)

	updated, _ := m.Update(nextPaneKey)

	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 after Tab from last pane, got %d", updated.paneIdx)
	}
	if updated.windowIdx != 0 {
		t.Errorf("expected windowIdx unchanged at 0 after pane wrap, got %d", updated.windowIdx)
	}
}

func TestPreviewPaneNav_SinglePaneWindowIsSilentNoOpZeroTail(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, cmd := m.Update(nextPaneKey)

	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 unchanged on single-pane window, got %d", updated.paneIdx)
	}
	if updated.windowIdx != 0 {
		t.Errorf("expected windowIdx=0 unchanged on single-pane window, got %d", updated.windowIdx)
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected zero Tail calls on single-pane window, got %d", len(reader.calls))
	}
	if cmd != nil {
		t.Errorf("expected nil cmd on single-pane no-op, got non-nil")
	}
}

func TestPreviewPaneNav_SingleWindowSinglePaneSessionIsSilentNoOp(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, cmd := m.Update(nextPaneKey)

	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 unchanged in degenerate session, got %d", updated.paneIdx)
	}
	if updated.windowIdx != 0 {
		t.Errorf("expected windowIdx=0 unchanged in degenerate session, got %d", updated.windowIdx)
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected zero Tail calls in degenerate session, got %d", len(reader.calls))
	}
	if cmd != nil {
		t.Errorf("expected nil cmd in degenerate session, got non-nil")
	}
}

func TestPreviewPaneNav_TriggersExactlyOneTailCallWithNewlyFocusedPaneKey(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 2, WindowName: "main", PaneIndices: []int{4, 7, 9}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	_, _ = m.Update(nextPaneKey)

	if len(reader.calls) != 1 {
		t.Fatalf("expected exactly 1 Tail call after Tab, got %d", len(reader.calls))
	}
	want := state.SanitizePaneKey("work", 2, 7)
	if reader.calls[0] != want {
		t.Errorf("expected Tail called with paneKey %q (raw window=2, raw pane=7), got %q", want, reader.calls[0])
	}
}

func TestPreviewPaneNav_ResetsViewportScrollPositionToTail(t *testing.T) {
	// Build content larger than the viewport so AtBottom is non-trivial.
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("line\n")
	}
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
	}
	reader := &recordingReader{bytes: []byte(b.String())}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 10)
	// Pre-load some content and scroll to top so a successful pane nav must
	// explicitly call GotoBottom to satisfy AtBottom().
	m.viewport.SetContent("stale\nstale\nstale\n")
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected AtTop before Tab, got YOffset=%d", m.viewport.YOffset())
	}

	updated, _ := m.Update(nextPaneKey)

	if !updated.viewport.AtBottom() {
		t.Errorf("expected viewport.AtBottom()=true after Tab, got YOffset=%d", updated.viewport.YOffset())
	}
}

func TestPreviewPaneNav_DoesNotModifyWindowIdx(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1, 2}},
		{WindowIndex: 2, WindowName: "third", PaneIndices: []int{0, 1}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	// Start on the middle window, last pane — Tab should wrap pane within
	// this window, *not* advance to the next window.
	m := newPreviewModelForTab("work", groups, 1, 2, reader, 80, 24)

	updated, _ := m.Update(nextPaneKey)

	if updated.windowIdx != 1 {
		t.Errorf("expected windowIdx=1 unchanged after Tab, got %d", updated.windowIdx)
	}
	if updated.paneIdx != 0 {
		t.Errorf("expected paneIdx=0 (wrapped) after Tab, got %d", updated.paneIdx)
	}
}

func TestPreviewPaneNav_InterceptedBeforeViewportSeesIt(t *testing.T) {
	// Pin the contract that the pane-nav branch lands BEFORE the default
	// viewport delegation: paneIdx advanced (proof of interception) and the
	// viewport state matches the post-read GotoBottom contract. Tab must never
	// be swallowed by the embedded viewport.
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("line\n")
	}
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
	}
	reader := &recordingReader{bytes: []byte(b.String())}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 10)

	updated, cmd := m.Update(nextPaneKey)

	if updated.paneIdx != 1 {
		t.Errorf("expected paneIdx=1 (pane-nav branch ran), got %d", updated.paneIdx)
	}
	if !updated.viewport.AtBottom() {
		t.Errorf("expected AtBottom=true after pane-nav interception+read, got YOffset=%d", updated.viewport.YOffset())
	}
	if cmd != nil {
		t.Errorf("expected nil cmd from pane-nav branch (synchronous read, intercepted before viewport), got non-nil")
	}
	if len(reader.calls) != 1 {
		t.Errorf("expected exactly 1 Tail call from pane nav, got %d", len(reader.calls))
	}
}
