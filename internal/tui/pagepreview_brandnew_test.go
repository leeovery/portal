package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func brandNewFixtureGroups() []tmux.WindowGroup {
	return []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
	}
}

func TestPreviewBrandNew_EveryPaneRendersPlaceholder(t *testing.T) {
	groups := brandNewFixtureGroups()
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true on (nil, nil) initial open, got false")
	}

	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("initial (w0,p0) viewport = %q; want %q", got, previewPlaceholder)
	}

	m, _ = m.Update(nextPaneKey)
	if m.windowIdx != 0 || m.paneIdx != 1 {
		t.Fatalf("after Tab: expected (windowIdx=0, paneIdx=1), got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w0,p1) viewport = %q; want %q", got, previewPlaceholder)
	}

	m, _ = m.Update(nextPaneKey)
	if m.windowIdx != 0 || m.paneIdx != 0 {
		t.Fatalf("after Tab wrap: expected (0, 0), got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w0,p0) wrap viewport = %q; want %q", got, previewPlaceholder)
	}

	m, _ = m.Update(nextWindowKey)
	if m.windowIdx != 1 || m.paneIdx != 0 {
		t.Fatalf("after →: expected (1, 0), got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w1,p0) viewport = %q; want %q", got, previewPlaceholder)
	}

	m, _ = m.Update(nextPaneKey)
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

	steps := []struct {
		name       string
		key        tea.KeyPressMsg
		applyKey   bool
		wantWindow string
		wantPane   string
	}{
		{name: "initial (w0,p0)", applyKey: false, wantWindow: "Window 1/2", wantPane: "Pane 1/2"},
		{name: "Tab → (w0,p1)", applyKey: true, key: nextPaneKey, wantWindow: "Window 1/2", wantPane: "Pane 2/2"},
		{name: "Tab → wrap (w0,p0)", applyKey: true, key: nextPaneKey, wantWindow: "Window 1/2", wantPane: "Pane 1/2"},
		{name: "→ → (w1,p0)", applyKey: true, key: nextWindowKey, wantWindow: "Window 2/2", wantPane: "Pane 1/2"},
		{name: "→ → wrap (w0,p0)", applyKey: true, key: nextWindowKey, wantWindow: "Window 1/2", wantPane: "Pane 1/2"},
		{name: "← → wrap (w1,p0)", applyKey: true, key: prevWindowKey, wantWindow: "Window 2/2", wantPane: "Pane 1/2"},
	}

	for _, s := range steps {
		if s.applyKey {
			m, _ = m.Update(s.key)
		}
		chrome := stripANSI(chromeLineForTest(m))
		if !strings.Contains(chrome, s.wantWindow) {
			t.Errorf("%s: chrome = %q; want substring %q", s.name, chrome, s.wantWindow)
		}
		if !strings.Contains(chrome, s.wantPane) {
			t.Errorf("%s: chrome = %q; want substring %q", s.name, chrome, s.wantPane)
		}
		if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
			t.Errorf("%s: viewport = %q; want %q", s.name, got, previewPlaceholder)
		}
	}
}

func TestPreviewBrandNew_NextWindowAdvancesAndPaneNavCyclesWithinWindowUnderAllPlaceholders(t *testing.T) {
	groups := brandNewFixtureGroups()
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	m, _ = m.Update(nextWindowKey)
	if m.windowIdx != 1 {
		t.Errorf("→ did not advance windowIdx: got %d, want 1", m.windowIdx)
	}
	if m.paneIdx != 0 {
		t.Errorf("→ did not reset paneIdx: got %d, want 0", m.paneIdx)
	}

	m, _ = m.Update(nextPaneKey)
	if m.windowIdx != 1 {
		t.Errorf("Tab leaked windowIdx: got %d, want 1 (pane nav is intra-window)", m.windowIdx)
	}
	if m.paneIdx != 1 {
		t.Errorf("Tab did not advance paneIdx: got %d, want 1", m.paneIdx)
	}
	m, _ = m.Update(nextPaneKey)
	if m.windowIdx != 1 {
		t.Errorf("Tab wrap leaked windowIdx: got %d, want 1", m.windowIdx)
	}
	if m.paneIdx != 0 {
		t.Errorf("Tab wrap did not return to paneIdx 0: got %d, want 0", m.paneIdx)
	}
}

func TestPreviewBrandNew_CycleKeysDoNotSkipPlaceholderPanes(t *testing.T) {
	groups := brandNewFixtureGroups()
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	visited := map[[2]int]int{}
	visited[[2]int{m.windowIdx, m.paneIdx}]++

	m, _ = m.Update(nextPaneKey)
	visited[[2]int{m.windowIdx, m.paneIdx}]++
	m, _ = m.Update(nextWindowKey)
	visited[[2]int{m.windowIdx, m.paneIdx}]++
	m, _ = m.Update(nextPaneKey)
	visited[[2]int{m.windowIdx, m.paneIdx}]++

	for _, coord := range [][2]int{{0, 0}, {0, 1}, {1, 0}, {1, 1}} {
		if visited[coord] == 0 {
			t.Errorf("traversal skipped pane (windowIdx=%d, paneIdx=%d) — cycle keys must not skip placeholder panes", coord[0], coord[1])
		}
	}

	if len(reader.calls) != 4 {
		t.Errorf("expected 4 Tail calls (one per focus event across 4 panes), got %d (calls=%v)", len(reader.calls), reader.calls)
	}
}

func TestPreviewMixed_BytesPaneAndPlaceholderPanesCoexist(t *testing.T) {
	groups := brandNewFixtureGroups()
	w0p0Key := state.SanitizePaneKey("work", 0, 0)

	reader := &keyedReader{
		outcomes: map[string]struct {
			bytes []byte
			err   error
		}{
			w0p0Key: {bytes: []byte("first pane bytes"), err: nil},
		},
	}
	enum := &stubEnumerator{groups: groups}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	view := m.viewport.View()
	if !strings.Contains(view, "first pane bytes") {
		t.Errorf("(w0,p0) viewport = %q; want substring %q", view, "first pane bytes")
	}
	chrome := stripANSI(chromeLineForTest(m))
	if !strings.Contains(chrome, "Window 1/2") {
		t.Errorf("(w0,p0) chrome = %q; want substring %q", chrome, "Window 1/2")
	}
	if !strings.Contains(chrome, "Pane 1/2") {
		t.Errorf("(w0,p0) chrome = %q; want substring %q", chrome, "Pane 1/2")
	}

	m, _ = m.Update(nextPaneKey)
	if m.paneIdx != 1 {
		t.Fatalf("expected paneIdx=1 after Tab, got %d", m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w0,p1) viewport = %q; want %q", got, previewPlaceholder)
	}

	m, _ = m.Update(nextWindowKey)
	if m.windowIdx != 1 || m.paneIdx != 0 {
		t.Fatalf("expected (1, 0) after ], got (%d, %d)", m.windowIdx, m.paneIdx)
	}
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Errorf("(w1,p0) viewport = %q; want %q", got, previewPlaceholder)
	}
}

func TestPreviewMixed_FocusFromBytesPaneToPlaceholderAndBackIssuesFreshTailCalls(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
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

	m, _ = m.Update(nextPaneKey)
	if got := stripTrailingBlanks(m.viewport.View()); got != previewPlaceholder {
		t.Fatalf("(w0,p1) viewport = %q; want %q", got, previewPlaceholder)
	}

	m, _ = m.Update(nextPaneKey)
	if m.paneIdx != 0 {
		t.Fatalf("expected paneIdx=0 after Tab back, got %d", m.paneIdx)
	}
	if !strings.Contains(m.viewport.View(), "first pane bytes") {
		t.Errorf("(w0,p0) refocus viewport = %q; want substring %q", m.viewport.View(), "first pane bytes")
	}

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
