package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestPreviewWindowSizeMsg_RecordsDimensionsAndSetsViewportToInnerSize pins
// the resize contract: tea.WindowSizeMsg records msg.Width/msg.Height on
// m.width/m.height and calls m.viewport.SetSize with both dimensions reduced by
// previewFrameOverhead (= 6), clamped non-negative. The §9 joined panel's frame
// spans 6 rows (top + header + 2 dividers + footer + bottom) and 6 columns
// (2 side borders + 2·panelRowInset each side), so the inner viewport surface is
// (msg.Width − 6) × (msg.Height − 6) per § Resize behaviour.
func TestPreviewWindowSizeMsg_RecordsDimensionsAndSetsViewportToInnerSize(t *testing.T) {
	m := newFramePreviewModelAt(t, "main", nil, 80, 24)

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

// TestPreviewWindowSizeMsg_ClampsViewportDimensionsNonNegative pins the
// degenerate-terminal contract: viewport.SetSize with negative arguments is
// unspecified, so the WindowSizeMsg handler must clamp both dimensions to
// zero at the lower bound. msg.Width=1, msg.Height=0 produces viewport
// dimensions (0, 0) — the clamp boundary per § Resize behaviour.
func TestPreviewWindowSizeMsg_ClampsViewportDimensionsNonNegative(t *testing.T) {
	m := newFramePreviewModelAt(t, "main", nil, 80, 24)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 1, Height: 0})

	if updated.viewport.Width() != 0 {
		t.Errorf("expected viewport.Width clamped to 0 for msg.Width=1, got %d", updated.viewport.Width())
	}
	if updated.viewport.Height() != 0 {
		t.Errorf("expected viewport.Height clamped to 0 for msg.Height=0, got %d", updated.viewport.Height())
	}
}
