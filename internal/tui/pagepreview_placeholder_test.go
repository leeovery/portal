package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

type nilNilReader struct {
	calls []string
}

func (r *nilNilReader) Tail(pane PaneScrollback) ([]byte, error) {
	paneKey := pane.PaneKey
	r.calls = append(r.calls, paneKey)
	return nil, nil
}

func stripTrailingBlanks(s string) string {
	return strings.TrimRight(s, " \n\t")
}

func TestPreviewPlaceholder_RendersAtInitialOpenWhenTailReturnsNilNil(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
		},
	}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)

	if !ok {
		t.Fatalf("expected ok=true when Tail returns (nil, nil), got false")
	}
	got := stripTrailingBlanks(m.viewport.View())
	if got != previewPlaceholder {
		t.Errorf("viewport content = %q; want %q", got, previewPlaceholder)
	}
}

func TestPreviewPlaceholder_RendersAfterPaneNavCycleWhenTailReturnsNilNil(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
	}
	reader := &nilNilReader{}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, _ := m.Update(nextPaneKey)

	if updated.paneIdx != 1 {
		t.Fatalf("setup: expected paneIdx=1 after Tab, got %d", updated.paneIdx)
	}
	got := stripTrailingBlanks(updated.viewport.View())
	if got != previewPlaceholder {
		t.Errorf("viewport content after Tab = %q; want %q", got, previewPlaceholder)
	}
}

func TestPreviewPlaceholder_RendersAfterNextWindowCycleWhenTailReturnsNilNil(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "first", Panes: []tmux.WindowPane{{Index: 0}}},
		{WindowIndex: 1, WindowName: "second", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	reader := &nilNilReader{}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, _ := m.Update(nextWindowKey)

	if updated.windowIdx != 1 {
		t.Fatalf("setup: expected windowIdx=1 after →, got %d", updated.windowIdx)
	}
	got := stripTrailingBlanks(updated.viewport.View())
	if got != previewPlaceholder {
		t.Errorf("viewport content after → = %q; want %q", got, previewPlaceholder)
	}
}

func TestPreviewPlaceholder_ChromeCountsRemainCorrectWhenPlaceholderShown(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}, {Index: 1}}},
		{WindowIndex: 1, WindowName: "other", Panes: []tmux.WindowPane{{Index: 0}}},
	}
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	chrome := stripANSI(chromeLineForTest(m))

	expected := stripANSI(chromeLineForTest(newPreviewModelForHelpers(t, "work", groups, 0, 0)))
	if chrome != expected {
		t.Errorf("chromeLine() under placeholder = %q; want %q (identical to non-placeholder shape)", chrome, expected)
	}
	if !strings.Contains(chrome, "Window 1/2") {
		t.Errorf("chromeLine() = %q; want substring %q", chrome, "Window 1/2")
	}
	if !strings.Contains(chrome, "Pane 1/2") {
		t.Errorf("chromeLine() = %q; want substring %q", chrome, "Pane 1/2")
	}
	if !strings.Contains(chrome, "◉ preview work") {
		t.Errorf("chromeLine() = %q; want marker + session %q", chrome, "◉ preview work")
	}
}

func TestPreviewPlaceholder_IsCanonicalWordingNoSavedContent(t *testing.T) {
	if previewPlaceholder != "(no saved content)" {
		t.Errorf("previewPlaceholder = %q; want %q", previewPlaceholder, "(no saved content)")
	}
}

type enoentReader struct{}

func (enoentReader) Tail(PaneScrollback) ([]byte, error) { return nil, nil }

type zeroByteReader struct{}

func (zeroByteReader) Tail(PaneScrollback) ([]byte, error) { return nil, nil }

type zeroLineReader struct{}

func (zeroLineReader) Tail(PaneScrollback) ([]byte, error) { return nil, nil }

func TestPreviewPlaceholder_ENOENTZeroByteAndZeroLineProduceIdenticalViewportContent(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}},
	}

	readers := []ScrollbackReader{
		enoentReader{},
		zeroByteReader{},
		zeroLineReader{},
	}

	views := make([]string, len(readers))
	for i, r := range readers {
		enum := &stubEnumerator{groups: groups}
		m, ok := NewPreviewModel("work", enum, r, nil, 80, 24)
		if !ok {
			t.Fatalf("reader %d: expected ok=true, got false", i)
		}
		views[i] = m.viewport.View()
	}

	for i := 1; i < len(views); i++ {
		if views[i] != views[0] {
			t.Errorf("reader %d viewport.View() differs from reader 0:\n[0]=%q\n[%d]=%q", i, views[0], i, views[i])
		}
	}
}
