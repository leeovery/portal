package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// nilNilReader returns (nil, nil) on every Tail call, simulating the unified
// "no content available" outcome — collapsing ENOENT, zero-byte, and zero-line
// (only an unterminated partial) into one shape per the spec's
// § Architecture Summary > Test seams > ScrollbackReader return contract.
type nilNilReader struct {
	calls []string
}

func (r *nilNilReader) Tail(paneKey string) ([]byte, error) {
	r.calls = append(r.calls, paneKey)
	return nil, nil
}

// stripTrailingBlanks removes trailing blank lines that bubbles/viewport pads
// rendered content with up to its configured height. Placeholder assertions
// key off the trimmed payload.
func stripTrailingBlanks(s string) string {
	return strings.TrimRight(s, " \n\t")
}

func TestPreviewPlaceholder_RendersAtInitialOpenWhenTailReturnsNilNil(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
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

func TestPreviewPlaceholder_RendersAfterTabCycleWhenTailReturnsNilNil(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
	}
	reader := &nilNilReader{}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

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
		{WindowIndex: 0, WindowName: "first", PaneIndices: []int{0}},
		{WindowIndex: 1, WindowName: "second", PaneIndices: []int{0}},
	}
	reader := &nilNilReader{}
	m := newPreviewModelForTab("work", groups, 0, 0, reader, 80, 24)

	updated, _ := m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})

	if updated.windowIdx != 1 {
		t.Fatalf("setup: expected windowIdx=1 after ], got %d", updated.windowIdx)
	}
	got := stripTrailingBlanks(updated.viewport.View())
	if got != previewPlaceholder {
		t.Errorf("viewport content after ] = %q; want %q", got, previewPlaceholder)
	}
}

func TestPreviewPlaceholder_ChromeCountsRemainCorrectWhenPlaceholderShown(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
		{WindowIndex: 1, WindowName: "other", PaneIndices: []int{0}},
	}
	enum := &stubEnumerator{groups: groups}
	reader := &nilNilReader{}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}

	chrome := stripANSI(chromeLineForTest(m))

	// chromeLine() must produce identical output to the non-placeholder shape
	// because chrome is a pure function of the cached groups + windowIdx +
	// paneIdx, not of viewport content.
	expected := stripANSI(chromeLineForTest(newPreviewModelForHelpers("work", groups, 0, 0)))
	if chrome != expected {
		t.Errorf("chromeLine() under placeholder = %q; want %q (identical to non-placeholder shape)", chrome, expected)
	}
	// Sanity: the placeholder shape still surfaces correct counters.
	if !strings.Contains(chrome, "Window 1 of 2") {
		t.Errorf("chromeLine() = %q; want substring %q", chrome, "Window 1 of 2")
	}
	if !strings.Contains(chrome, "Pane 1 of 2") {
		t.Errorf("chromeLine() = %q; want substring %q", chrome, "Pane 1 of 2")
	}
	if !strings.Contains(chrome, "main") {
		t.Errorf("chromeLine() = %q; want window name %q", chrome, "main")
	}
}

func TestPreviewPlaceholder_IsCanonicalWordingNoSavedContent(t *testing.T) {
	if previewPlaceholder != "(no saved content)" {
		t.Errorf("previewPlaceholder = %q; want %q", previewPlaceholder, "(no saved content)")
	}
}

// enoentReader, zeroByteReader, and zeroLineReader simulate the three failure
// modes the spec collapses into the single (nil, nil) shape: ENOENT, zero-byte
// .bin, and a file containing only an unterminated partial line. At the
// ScrollbackReader seam they are observably identical — all three return
// (nil, nil) — and the call site must produce identical viewport content.
type enoentReader struct{}

func (enoentReader) Tail(string) ([]byte, error) { return nil, nil }

type zeroByteReader struct{}

func (zeroByteReader) Tail(string) ([]byte, error) { return nil, nil }

type zeroLineReader struct{}

func (zeroLineReader) Tail(string) ([]byte, error) { return nil, nil }

func TestPreviewPlaceholder_ENOENTZeroByteAndZeroLineProduceIdenticalViewportContent(t *testing.T) {
	groups := []tmux.WindowGroup{
		{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
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
