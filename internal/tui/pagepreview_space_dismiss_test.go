package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPreviewSpaceEmitsDismissedMsg(t *testing.T) {
	m := newTestPreview(t, "content", 80, 24)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if cmd == nil {
		t.Fatalf("expected non-nil tea.Cmd from Space, got nil")
	}
	if _, ok := cmd().(previewDismissedMsg); !ok {
		t.Fatalf("Space cmd produced %T; want previewDismissedMsg", cmd())
	}
}
