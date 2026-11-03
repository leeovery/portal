package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPreviewWindowSizeMsg_RecordsDimensionsAndSetsViewportToInnerSize(t *testing.T) {
	m := newTestPreview(t, "", 80, 24)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if updated.width != 100 {
		t.Errorf("expected m.width=100 recorded on resize, got %d", updated.width)
	}
	if updated.height != 30 {
		t.Errorf("expected m.height=30 recorded on resize, got %d", updated.height)
	}
	if updated.viewport.Width() != 100-previewFrameOverhead {
		t.Errorf("expected viewport.Width=%d (msg.Width − previewFrameOverhead), got %d", 100-previewFrameOverhead, updated.viewport.Width())
	}
	if updated.viewport.Height() != 30-previewFrameOverhead {
		t.Errorf("expected viewport.Height=%d (msg.Height − previewFrameOverhead), got %d", 30-previewFrameOverhead, updated.viewport.Height())
	}
}

func TestPreviewWindowSizeMsg_ClampsViewportDimensionsNonNegative(t *testing.T) {
	m := newTestPreview(t, "", 80, 24)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 1, Height: 0})

	if updated.viewport.Width() != 0 {
		t.Errorf("expected viewport.Width clamped to 0 for msg.Width=1, got %d", updated.viewport.Width())
	}
	if updated.viewport.Height() != 0 {
		t.Errorf("expected viewport.Height clamped to 0 for msg.Height=0, got %d", updated.viewport.Height())
	}
}
