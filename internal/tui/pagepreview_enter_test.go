package tui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

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
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "other", Panes: []tmux.WindowPane{{Index: 0}}},
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

func TestPreviewEnter_DispatchesWithWalkedIndicesAfterPaneNav(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}, {Index: 2}}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)

	updated, _ := m.Update(nextPaneKey)
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
		t.Errorf("attacher.Run called with %#v; want %#v (post pane-nav walked pane)", got, want)
	}
	if cmd == nil {
		t.Errorf("expected non-nil tea.Cmd, got nil")
	}
}

func TestPreviewEnter_DispatchesWithWalkedIndicesAfterWindowNav(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 2, WindowName: "third", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)

	updated, _ := m.Update(nextWindowKey)
	if updated.windowIdx != 1 {
		t.Fatalf("setup: expected windowIdx=1 after →, got %d", updated.windowIdx)
	}

	_, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Fatalf("expected exactly 1 attacher.Run call, got %d", len(attacher.calls))
	}
	got := attacher.calls[0]
	want := recordedAttacherCall{session: "work", window: 1, pane: 0}
	if got != want {
		t.Errorf("attacher.Run called with %#v; want %#v (post-`→` walked window)", got, want)
	}
}

func TestPreviewEnter_DispatchesWithRawTmuxIndicesOnNonContiguousSession(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 1}}},
		{WindowIndex: 5, WindowName: "second", Panes: []tmux.WindowPane{{Index: 3}}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
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
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	reader := &recordingReader{bytes: []byte("content")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 10)
	var lines strings.Builder
	for range 50 {
		lines.WriteString("line\n")
	}
	m.viewport.SetContent(lines.String())
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected viewport.AtTop, got YOffset=%d", m.viewport.YOffset())
	}
	prevYOffset := m.viewport.YOffset()

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if updated.viewport.YOffset() != prevYOffset {
		t.Errorf("viewport.YOffset = %d; want unchanged %d (Enter must not reach viewport)", updated.viewport.YOffset(), prevYOffset)
	}
	if len(attacher.calls) != 1 {
		t.Errorf("expected attacher.Run to have fired (proof of interception), got %d calls", len(attacher.calls))
	}
}

func TestPreviewEnter_NoOpWhenAttacherIsNil(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
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
	if updated.windowIdx != m.windowIdx || updated.paneIdx != m.paneIdx {
		t.Errorf("expected windowIdx/paneIdx unchanged on nil-attacher no-op")
	}
}

func TestPreviewEnter_DispatchesWhenViewportHasRealBytes(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	reader := &recordingReader{bytes: []byte("real content bytes")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)
	m.viewport.SetContent("real content bytes")

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Errorf("expected attacher.Run to fire on real-bytes viewport, got %d calls", len(attacher.calls))
	}
}

func TestPreviewEnter_DispatchesWhenViewportRenderedPlaceholder(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	reader := &recordingReader{bytes: nil, err: nil}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)
	m.viewport.SetContent(previewPlaceholder)

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Errorf("expected attacher.Run to fire on placeholder viewport, got %d calls", len(attacher.calls))
	}
}

func TestPreviewEnter_DispatchesWhenViewportRenderedReadError(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	reader := &recordingReader{bytes: nil, err: errors.New("EACCES")}
	attacher := &fakePreviewAttacher{}
	m := newPreviewModelForEnter("work", groups, 0, 0, reader, attacher, 80, 24)
	m.viewport.SetContent(previewReadError)

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(attacher.calls) != 1 {
		t.Errorf("expected attacher.Run to fire on read-error viewport, got %d calls", len(attacher.calls))
	}
}
