package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestPreviewSpaceEmitsDismissedMsg locks the Space-dismiss contract at the
// previewModel.Update level: a tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} delivered to
// the preview must return a non-nil tea.Cmd whose execution yields a
// previewDismissedMsg{}, mirroring Esc exactly. Construction follows the
// hermetic pattern (NewPreviewModel + lightweight stub seams) used by the
// peer Esc-dismiss assertions in pagepreview_hermetic_test.go.
func TestPreviewSpaceEmitsDismissedMsg(t *testing.T) {
	enum := &hermeticEnumerator{}
	reader := &hermeticReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on construction, got false")
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if cmd == nil {
		t.Fatalf("expected non-nil tea.Cmd from Space, got nil")
	}
	if _, ok := cmd().(previewDismissedMsg); !ok {
		t.Fatalf("Space cmd produced %T; want previewDismissedMsg", cmd())
	}
}
