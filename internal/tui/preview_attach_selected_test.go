package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// Tests for Phase 3 task 3-1: the previewAttachSelectedMsg handler records the
// selected session on the model and returns tea.Quit so the surrounding TUI
// program exits before the connector handoff runs. This mirrors the
// Sessions-page Enter shape: handleSessionListEnter sets m.selected + Quit,
// then processTUIResult performs the connector call AFTER tea.NewProgram.Run
// returns.

func TestPreviewAttachSelected_RecordsSessionOnModel(t *testing.T) {
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	updated, _ := m.Update(previewAttachSelectedMsg{Session: "alpha"})

	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.Selected() != "alpha" {
		t.Errorf("Selected() = %q; want %q", got.Selected(), "alpha")
	}
}

func TestPreviewAttachSelected_ReturnsTeaQuit(t *testing.T) {
	sessions := []tmux.Session{{Name: "alpha", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	_, cmd := m.Update(previewAttachSelectedMsg{Session: "alpha"})

	if cmd == nil {
		t.Fatalf("expected non-nil cmd carrying tea.Quit, got nil")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("expected tea.Quit() from selected handler, got %T", msg)
	}
}

// Parity assertion: a successful preview-Enter dispatched through the pipeline
// must resolve to a Model whose Selected() matches the previewed session,
// matching the Sessions-page Enter handoff contract.
func TestPreviewAttachSelected_ParityWithSessionsPageEnterShape(t *testing.T) {
	sessions := []tmux.Session{{Name: "bravo", Windows: 1, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(sessions, enum, reader)

	updated, cmd := m.Update(previewAttachSelectedMsg{Session: "bravo"})
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.Selected() != "bravo" {
		t.Errorf("Selected() = %q; want %q", got.Selected(), "bravo")
	}
	if cmd == nil || cmd() != tea.Quit() {
		t.Errorf("expected tea.Quit-bearing cmd, got %v", cmd)
	}
}
