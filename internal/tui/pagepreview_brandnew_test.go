package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// Brand-new-session edge case (Phase 4 task 4-4):
//
// A session whose every pane has no `.bin` content yet — every paneKey under
// the ScrollbackReader returns the unified (nil, nil) "no content available"
// shape — must remain fully traversable in preview. Cycle keys (], [, Tab)
// land on every structural entry; chrome counts (Window M of N, Pane X of Y)
// stay accurate at every step; every focused pane renders the placeholder.
//
// The mixed variant additionally pins that one pane returning bytes while the
// others return (nil, nil) is dispatched per-pane: bytes pane renders bytes,
// placeholder panes render placeholder, and refocusing back onto the bytes
// pane re-issues a fresh Tail call (no per-pane content cache, just like the
// no per-pane error cache invariant pinned by 4-2).
//
// Spec: § Brand-new-session Edge Case; § Acceptance Criteria > Edge cases.
//
// Production code is unchanged by this file — these tests pin existing
// dispatcher behaviour as a regression boundary.

// brandNewFixtureGroups is the 2 windows × 2 panes structural shape used by
// both the all-placeholder and mixed-content fixtures. Captured once so the
// chrome-counter assertions reference the same enumeration in both fixtures.
func brandNewFixtureGroups() []tmux.WindowGroup {
	return []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1}},
	}
}

func TestPreviewBrandNew_EveryPaneRendersPlaceholder(t *testing.T) {
	// All-placeholder fixture: 2 windows × 2 panes; ScrollbackReader.Tail
	// returns (nil, nil) for every paneKey. Every cycle-key step lands on
	// the placeholder and the structural traversal visits all four panes
	// without skipping.
	groups := brandNewFixtureGroups()
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on (nil, nil) initial open, got false")
	}

	// (w0, p0) — initial focus.
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("initial (w0,p0) viewport = %q; want %q", got, previewPlaceholder)
	}

	// Tab → (w0, p1).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.windowIdx != 0 || m.paneIdx != 1 {
		t.Fatalf("after Tab: expected (windowIdx=0, paneIdx=1), got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w0,p1) viewport = %q; want %q", got, previewPlaceholder)
	}

	// Tab → wraps within window back to (w0, p0).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.windowIdx != 0 || m.paneIdx != 0 {
		t.Fatalf("after Tab wrap: expected (0, 0), got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w0,p0) wrap viewport = %q; want %q", got, previewPlaceholder)
	}

	// ] → (w1, p0). Window cycle resets paneIdx to 0.
	m, _ = m.Update(nextWindowKey)
	if m.windowIdx != 1 || m.paneIdx != 0 {
		t.Fatalf("after ]: expected (1, 0), got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w1,p0) viewport = %q; want %q", got, previewPlaceholder)
	}

	// Tab → (w1, p1).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.windowIdx != 1 || m.paneIdx != 1 {
		t.Fatalf("after Tab in w1: expected (1, 1), got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w1,p1) viewport = %q; want %q", got, previewPlaceholder)
	}
}

func TestPreviewBrandNew_ChromeCountsAccurateAcrossAllPlaceholderCycles(t *testing.T) {
	groups := brandNewFixtureGroups()
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	// Step driver: each row is a sequence of key inputs to apply to the model
	// before asserting the chrome substrings present at that focus position.
	// Spec sequence pinned in the task body.
	steps := []struct {
		name        string
		key         tea.KeyMsg
		applyKey    bool // false on the initial step (no key to apply)
		wantWindow  string
		wantPane    string
		wantWinName string
	}{
		{name: "initial (w0,p0)", applyKey: false, wantWindow: "Window 1 of 2", wantPane: "Pane 1 of 2", wantWinName: "first"},
		{name: "Tab → (w0,p1)", applyKey: true, key: tea.KeyMsg{Type: tea.KeyTab}, wantWindow: "Window 1 of 2", wantPane: "Pane 2 of 2", wantWinName: "first"},
		{name: "Tab → wrap (w0,p0)", applyKey: true, key: tea.KeyMsg{Type: tea.KeyTab}, wantWindow: "Window 1 of 2", wantPane: "Pane 1 of 2", wantWinName: "first"},
		{name: "] → (w1,p0)", applyKey: true, key: nextWindowKey, wantWindow: "Window 2 of 2", wantPane: "Pane 1 of 2", wantWinName: "second"},
		{name: "] → wrap (w0,p0)", applyKey: true, key: nextWindowKey, wantWindow: "Window 1 of 2", wantPane: "Pane 1 of 2", wantWinName: "first"},
		{name: "[ → wrap (w1,p0)", applyKey: true, key: prevWindowKey, wantWindow: "Window 2 of 2", wantPane: "Pane 1 of 2", wantWinName: "second"},
	}

	for _, s := range steps {
		if s.applyKey {
			m, _ = m.Update(s.key)
		}
		chrome := stripANSI(m.chromeLine())
		if !strings.Contains(chrome, s.wantWindow) {
			t.Errorf("%s: chrome = %q; want substring %q", s.name, chrome, s.wantWindow)
		}
		if !strings.Contains(chrome, s.wantPane) {
			t.Errorf("%s: chrome = %q; want substring %q", s.name, chrome, s.wantPane)
		}
		if !strings.Contains(chrome, s.wantWinName) {
			t.Errorf("%s: chrome = %q; want window name substring %q", s.name, chrome, s.wantWinName)
		}
		// Defensive: every step is a placeholder render.
		if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
			t.Errorf("%s: viewport = %q; want %q", s.name, got, previewPlaceholder)
		}
	}
}

func TestPreviewBrandNew_NextWindowAdvancesAndTabCyclesWithinWindowUnderAllPlaceholders(t *testing.T) {
	// Mirrors the spec acceptance criteria: ] advances to the next window,
	// Tab cycles forward within the focused window. Both must work uniformly
	// when every pane is a placeholder.
	groups := brandNewFixtureGroups()
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	// ] from (w0, p0) → (w1, p0).
	m, _ = m.Update(nextWindowKey)
	if m.windowIdx != 1 {
		t.Errorf("] did not advance windowIdx: got %d, want 1", m.windowIdx)
	}
	if m.paneIdx != 0 {
		t.Errorf("] did not reset paneIdx: got %d, want 0", m.paneIdx)
	}

	// Tab inside w1 from p0 → p1.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.windowIdx != 1 {
		t.Errorf("Tab leaked windowIdx: got %d, want 1 (Tab is intra-window)", m.windowIdx)
	}
	if m.paneIdx != 1 {
		t.Errorf("Tab did not advance paneIdx: got %d, want 1", m.paneIdx)
	}
	// Tab again wraps within w1 to p0.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.windowIdx != 1 {
		t.Errorf("Tab wrap leaked windowIdx: got %d, want 1", m.windowIdx)
	}
	if m.paneIdx != 0 {
		t.Errorf("Tab wrap did not return to paneIdx 0: got %d, want 0", m.paneIdx)
	}
}

func TestPreviewBrandNew_CycleKeysDoNotSkipPlaceholderPanes(t *testing.T) {
	// 2 windows × 2 panes — exhaustive traversal must visit all 4 distinct
	// (windowIdx, paneIdx) coordinates. Sequence: initial (0,0), Tab (0,1),
	// ] (1,0), Tab (1,1).
	groups := brandNewFixtureGroups()
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	// Coordinates visited (after each step), in order. Map values: count of
	// visits — must be at least 1 for every structural entry by the end.
	visited := map[[2]int]int{}
	visited[[2]int{m.windowIdx, m.paneIdx}]++

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	visited[[2]int{m.windowIdx, m.paneIdx}]++
	m, _ = m.Update(nextWindowKey)
	visited[[2]int{m.windowIdx, m.paneIdx}]++
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	visited[[2]int{m.windowIdx, m.paneIdx}]++

	for _, coord := range [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}} {
		if visited[coord] == 0 {
			t.Errorf("traversal skipped pane (windowIdx=%d, paneIdx=%d) — cycle keys must not skip placeholder panes", coord[0], coord[1])
		}
	}

	// Reader must have been called once per focus event: initial (1) + Tab
	// + ] + Tab = 4. (No skip = no missed Tail call either.)
	if len(reader.calls) != 4 {
		t.Errorf("expected 4 Tail calls (one per focus event across 4 panes), got %d (calls=%v)", len(reader.calls), reader.calls)
	}
}

func TestPreviewMixed_BytesPaneAndPlaceholderPanesCoexist(t *testing.T) {
	// Mixed fixture: w0p0 has bytes; the other three panes return (nil, nil).
	// Initial focus on (w0, p0) renders bytes. Tab to (w0, p1) renders
	// placeholder. ] to (w1, p0) renders placeholder.
	groups := brandNewFixtureGroups()
	w0p0Key := state.SanitizePaneKey("work", 0, 0)

	reader := &keyedReader{
		outcomes: map[string]struct {
			bytes []byte
			err   error
		}{
			w0p0Key: {bytes: []byte("first pane bytes"), err: nil},
			// Other paneKeys default to (nil, nil) via map zero-value lookup
			// in keyedReader.Tail.
		},
	}
	enum := &stubEnumerator{groups: groups}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	// Initial (w0, p0) renders bytes.
	view := m.viewport.View()
	if !strings.Contains(view, "first pane bytes") {
		t.Errorf("(w0,p0) viewport = %q; want substring %q", view, "first pane bytes")
	}
	chrome := stripANSI(m.chromeLine())
	if !strings.Contains(chrome, "Window 1 of 2") {
		t.Errorf("(w0,p0) chrome = %q; want substring %q", chrome, "Window 1 of 2")
	}
	if !strings.Contains(chrome, "Pane 1 of 2") {
		t.Errorf("(w0,p0) chrome = %q; want substring %q", chrome, "Pane 1 of 2")
	}

	// Tab → (w0, p1) renders placeholder.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.paneIdx != 1 {
		t.Fatalf("expected paneIdx=1 after Tab, got %d", m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w0,p1) viewport = %q; want %q", got, previewPlaceholder)
	}

	// ] → (w1, p0) renders placeholder.
	m, _ = m.Update(nextWindowKey)
	if m.windowIdx != 1 || m.paneIdx != 0 {
		t.Fatalf("expected (1, 0) after ], got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w1,p0) viewport = %q; want %q", got, previewPlaceholder)
	}
}

func TestPreviewMixed_FocusFromBytesPaneToPlaceholderAndBackIssuesFreshTailCalls(t *testing.T) {
	// w0p0 returns bytes; w0p1 returns (nil, nil). Tab away to w0p1 (placeholder),
	// Tab back to w0p0 (bytes). The Tail call count for w0p0 must be 2 — the
	// constructor's initial read plus the refocus read — pinning the no-cache
	// invariant for the bytes path (parallel to the no per-pane error cache
	// invariant from 4-2).
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0, 1}},
	}
	w0p0Key := state.SanitizePaneKey("work", 0, 0)
	w0p1Key := state.SanitizePaneKey("work", 0, 1)

	reader := &keyedReader{
		outcomes: map[string]struct {
			bytes []byte
			err   error
		}{
			w0p0Key: {bytes: []byte("first pane bytes"), err: nil},
			w0p1Key: {bytes: nil, err: nil},
		},
	}
	enum := &stubEnumerator{groups: groups}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	// Tab to w0p1 (placeholder).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Fatalf("(w0,p1) viewport = %q; want %q", got, previewPlaceholder)
	}

	// Tab back to w0p0 (bytes again — re-issued Tail).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.paneIdx != 0 {
		t.Fatalf("expected paneIdx=0 after Tab back, got %d", m.paneIdx)
	}
	if !strings.Contains(m.viewport.View(), "first pane bytes") {
		t.Errorf("(w0,p0) refocus viewport = %q; want substring %q", m.viewport.View(), "first pane bytes")
	}

	// w0p0 was Tail'd twice (initial + refocus). w0p1 once.
	w0p0Calls, w0p1Calls := 0, 0
	for _, c := range reader.calls {
		switch c {
		case w0p0Key:
			w0p0Calls++
		case w0p1Key:
			w0p1Calls++
		}
	}
	if w0p0Calls != 2 {
		t.Errorf("expected 2 Tail calls for w0p0 (initial + refocus), got %d (all calls=%v)", w0p0Calls, reader.calls)
	}
	if w0p1Calls != 1 {
		t.Errorf("expected 1 Tail call for w0p1, got %d", w0p1Calls)
	}
}
