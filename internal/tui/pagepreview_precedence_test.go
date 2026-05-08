package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// This file pins the dispatch precedence inside previewModel.Update as a hard
// regression contract: preview-owned keys (Esc, Home, End, Tab, ], [) are
// matched and short-circuited BEFORE the default delegation to
// bubbles/viewport.Update. The tests here are intentionally narrow probes —
// they assert the *interception* itself, separately from the behaviour
// already covered by pagepreview_tab_test.go and pagepreview_bracket_test.go.
//
// Snapshot strategy: capture viewport.YOffset before a keypress, drive the
// keypress, and assert YOffset relative to the snapshot. Any future refactor
// that introduces double-handling (preview branch runs AND viewport.Update
// also sees the same key) would be caught either by an unexpected YOffset
// change on an owned key, or by an unexpected extra Tail call.

// newPreviewModelForPrecedence builds a previewModel sized for a 50-line
// payload (overflows a 10-row viewport) with a multi-pane / multi-window
// shape so Tab and ] / [ both have non-degenerate target panes. The viewport
// starts anchored at scroll-tail per NewPreviewModel's contract.
func newPreviewModelForPrecedence(t *testing.T) (previewModel, *recordingReader) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("line\n")
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
			{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1}},
		},
	}
	reader := &recordingReader{bytes: []byte(b.String())}
	m, ok := NewPreviewModel("work", enum, reader, 80, 10)
	if !ok {
		t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
	}
	return m, reader
}

func TestPreviewPrecedence_TabDoesNotAdvanceViewportScrollOffset(t *testing.T) {
	// If Tab leaked through to bubbles/viewport.Update it might shift YOffset
	// (current bubbles/viewport@v1.0.0 doesn't bind Tab, but a future version
	// could — that's exactly the regression this test pins). The Tab branch
	// itself calls GotoBottom() via readFocusedPaneIntoViewport, so the
	// post-press YOffset is whatever GotoBottom resolves to. We snapshot
	// AtBottom() instead of a raw YOffset comparison to keep the assertion
	// stable across content shapes — interception means the value is the
	// post-read tail position, NOT a viewport-scrolled value.
	m, _ := newPreviewModelForPrecedence(t)
	if !m.viewport.AtBottom() {
		t.Fatalf("setup: expected AtBottom after initial-open anchor, got YOffset=%d", m.viewport.YOffset)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if !updated.viewport.AtBottom() {
		t.Errorf("expected AtBottom after Tab (preview-owned: post-read tail anchor), got YOffset=%d — viewport may have seen Tab", updated.viewport.YOffset)
	}
}

func TestPreviewPrecedence_NextWindowDoesNotAdvanceViewportScrollOffset(t *testing.T) {
	// `]` is preview-owned. Same logic as Tab: the branch ends in
	// GotoBottom() via readFocusedPaneIntoViewport, so we assert AtBottom
	// rather than an unchanged YOffset (initial-open already anchored at
	// bottom; the new pane's content is the same shape, so AtBottom is the
	// stable post-condition).
	m, _ := newPreviewModelForPrecedence(t)
	if !m.viewport.AtBottom() {
		t.Fatalf("setup: expected AtBottom after initial-open anchor, got YOffset=%d", m.viewport.YOffset)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})

	if !updated.viewport.AtBottom() {
		t.Errorf("expected AtBottom after ] (preview-owned: post-read tail anchor), got YOffset=%d — viewport may have seen ]", updated.viewport.YOffset)
	}
}

func TestPreviewPrecedence_PrevWindowDoesNotAdvanceViewportScrollOffset(t *testing.T) {
	// `[` is preview-owned. Same logic as `]`.
	m, _ := newPreviewModelForPrecedence(t)
	if !m.viewport.AtBottom() {
		t.Fatalf("setup: expected AtBottom after initial-open anchor, got YOffset=%d", m.viewport.YOffset)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})

	if !updated.viewport.AtBottom() {
		t.Errorf("expected AtBottom after [ (preview-owned: post-read tail anchor), got YOffset=%d — viewport may have seen [", updated.viewport.YOffset)
	}
}

func TestPreviewPrecedence_UpScrollsViewportUpwardPassthroughPreserved(t *testing.T) {
	// Up is NOT preview-owned — it must reach bubbles/viewport.Update so
	// scroll passthrough still works. Initial-open anchors at bottom (max
	// YOffset), so Up at that position must DECREASE YOffset.
	m, _ := newPreviewModelForPrecedence(t)
	before := m.viewport.YOffset
	if before == 0 {
		t.Fatalf("setup: expected YOffset > 0 (anchored at bottom), got 0 — Up has nowhere to go")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})

	if updated.viewport.YOffset >= before {
		t.Errorf("expected YOffset to decrease after Up (passthrough to viewport), got %d (was %d)", updated.viewport.YOffset, before)
	}
}

func TestPreviewPrecedence_PgDnScrollsViewportDownwardPassthroughPreserved(t *testing.T) {
	// PgDn is NOT preview-owned — it must reach bubbles/viewport.Update.
	// Drive viewport to the top first so PgDn has somewhere to go.
	m, _ := newPreviewModelForPrecedence(t)
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected AtTop after GotoTop, got YOffset=%d", m.viewport.YOffset)
	}
	before := m.viewport.YOffset

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})

	if updated.viewport.YOffset <= before {
		t.Errorf("expected YOffset to increase after PgDn (passthrough to viewport), got %d (was %d)", updated.viewport.YOffset, before)
	}
}

func TestPreviewPrecedence_JKVimStylePassthroughPreserved(t *testing.T) {
	// j (down) and k (up) are vim-style scroll keys bound by
	// bubbles/viewport's default keymap. They must reach viewport.Update via
	// the fall-through, NOT be intercepted by the preview's KeyRunes branch
	// (which only matches `]` and `[`).
	m, _ := newPreviewModelForPrecedence(t)
	m.viewport.GotoTop()
	if !m.viewport.AtTop() {
		t.Fatalf("setup: expected AtTop, got YOffset=%d", m.viewport.YOffset)
	}
	beforeJ := m.viewport.YOffset

	afterJ, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if afterJ.viewport.YOffset <= beforeJ {
		t.Errorf("expected YOffset to increase after j (vim-style passthrough), got %d (was %d)", afterJ.viewport.YOffset, beforeJ)
	}

	beforeK := afterJ.viewport.YOffset
	afterK, _ := afterJ.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if afterK.viewport.YOffset >= beforeK {
		t.Errorf("expected YOffset to decrease after k (vim-style passthrough), got %d (was %d)", afterK.viewport.YOffset, beforeK)
	}
}

func TestPreviewPrecedence_WindowSizeMsgStillReachesViewportForReflow(t *testing.T) {
	// tea.WindowSizeMsg has its own dedicated case in Update (it does NOT
	// flow through the keypress switch), but the precedence contract for
	// it is: viewport dimensions update so reflow happens. This test pins
	// that the WindowSizeMsg arm continues to mutate viewport.Width /
	// viewport.Height — a precedence regression that ate WindowSizeMsg
	// before reaching the viewport mutation would show up as unchanged
	// dimensions.
	m, _ := newPreviewModelForPrecedence(t)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 42})

	if updated.viewport.Width != 132 {
		t.Errorf("expected viewport.Width=132 after WindowSizeMsg, got %d", updated.viewport.Width)
	}
	wantHeight := 42 - previewChromeHeight
	if updated.viewport.Height != wantHeight {
		t.Errorf("expected viewport.Height=%d (msg.Height - previewChromeHeight) after WindowSizeMsg, got %d", wantHeight, updated.viewport.Height)
	}
}

func TestPreviewPrecedence_SingleTabProducesExactlyOneTailCallNoDoubleHandling(t *testing.T) {
	// The canary for double-handling: if Tab were processed by the preview
	// branch AND ALSO leaked to viewport.Update, the most likely
	// observable corruption depends on the bubbles/viewport version, but
	// the cleanest invariant is the Tail call count. The preview Tab
	// branch calls reader.Tail exactly once via readFocusedPaneIntoViewport;
	// viewport.Update never calls reader.Tail. So observing exactly ONE
	// Tail call after a single Tab keypress proves the Tab branch fired
	// and the message did not somehow trigger a second read.
	m, reader := newPreviewModelForPrecedence(t)
	// Initial-open already made one Tail call; reset the recorder so we
	// observe ONLY the Tab-driven calls.
	reader.calls = nil

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	if len(reader.calls) != 1 {
		t.Errorf("expected exactly 1 Tail call from a single Tab keypress (no double-handling), got %d", len(reader.calls))
	}
}

func TestPreviewPrecedence_NonKeyMsgFallsThroughToViewport(t *testing.T) {
	// Edge case: messages that are neither tea.WindowSizeMsg nor tea.KeyMsg
	// must fall through the type switch and reach viewport.Update. We can't
	// easily observe viewport.Update receiving a custom message directly,
	// but we can observe its OBSERVABLE side effect — the model and
	// viewport state remain consistent (no panic, no Tail call, no spurious
	// scroll). The strongest available probe is: send a custom tea.Msg and
	// assert that the model is returned unchanged in its preview-owned
	// fields and that no Tail call was triggered.
	m, reader := newPreviewModelForPrecedence(t)
	reader.calls = nil
	beforeYOffset := m.viewport.YOffset
	beforeWindow := m.windowIdx
	beforePane := m.paneIdx

	type customMsg struct{}
	updated, _ := m.Update(customMsg{})

	if updated.windowIdx != beforeWindow {
		t.Errorf("expected windowIdx unchanged on custom msg, got %d (was %d)", updated.windowIdx, beforeWindow)
	}
	if updated.paneIdx != beforePane {
		t.Errorf("expected paneIdx unchanged on custom msg, got %d (was %d)", updated.paneIdx, beforePane)
	}
	if updated.viewport.YOffset != beforeYOffset {
		t.Errorf("expected viewport.YOffset unchanged on custom msg (no scroll keys bound), got %d (was %d)", updated.viewport.YOffset, beforeYOffset)
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected zero Tail calls on custom msg (no preview branch fires), got %d", len(reader.calls))
	}
}
