package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

var nextPaneKey = tea.KeyPressMsg{Code: tea.KeyTab}

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
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}, {Index: 2}}},
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
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}, {Index: 2}}},
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
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}, {Index: 2}}},
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
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
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
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
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
		{WindowIndex: 2, WindowName: "main", Panes: []tmux.WindowPane{{Index: 4}, {Index: 7}, {Index: 9}}},
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
	var b strings.Builder
	for range 50 {
		b.WriteString("line\n")
	}
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
	}
	reader := &recordingReader{bytes: []byte(b.String())}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 10)
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
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}, {Index: 2}}},
		{WindowIndex: 2, WindowName: "third", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
	}
	reader := &recordingReader{bytes: []byte("content")}
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
	var b strings.Builder
	for range 50 {
		b.WriteString("line\n")
	}
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
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
