package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPreviewAttachSelected_RecordsSessionOnModel(t *testing.T) {
	sessions := []Session{{Name: "alpha", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

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
	sessions := []Session{{Name: "alpha", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

	_, cmd := m.Update(previewAttachSelectedMsg{Session: "alpha"})

	if cmd == nil {
		t.Fatalf("expected non-nil cmd carrying tea.Quit, got nil")
	}
	if msg := cmd(); msg != tea.Quit() {
		t.Errorf("expected tea.Quit() from selected handler, got %T", msg)
	}
}

func TestPreviewAttachSelected_ParityWithSessionsPageEnterShape(t *testing.T) {
	sessions := []Session{{Name: "bravo", Answered: true, Attached: false}}
	enum := newSinglePaneEnumerator()
	reader := &recordingReader{bytes: []byte("hi")}
	m := modelWithSeams(t, sessions, enum, reader)

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
