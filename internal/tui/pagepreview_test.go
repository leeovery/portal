package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// stubEnumerator records the session passed to ListWindowsAndPanesInSession
// and returns a configured groups/err pair.
type stubEnumerator struct {
	groups  []tmux.WindowGroup
	err     error
	calls   int
	lastArg string
}

func (s *stubEnumerator) ListWindowsAndPanesInSession(session string) ([]tmux.WindowGroup, error) {
	s.calls++
	s.lastArg = session
	return s.groups, s.err
}

// recordingReader records every paneKey passed to Tail and returns the
// configured (bytes, err).
type recordingReader struct {
	bytes []byte
	err   error
	calls []string
}

func (r *recordingReader) Tail(paneKey string) ([]byte, error) {
	r.calls = append(r.calls, paneKey)
	return r.bytes, r.err
}

func TestNewPreviewModel_ReturnsFalseWhenEnumerationErrors(t *testing.T) {
	enum := &stubEnumerator{err: errors.New("boom")}
	reader := &recordingReader{}

	_, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if ok {
		t.Errorf("expected ok=false on enumeration error, got true")
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected no Tail calls on enumeration error, got %d", len(reader.calls))
	}
}

func TestNewPreviewModel_ReturnsFalseOnEmptyEnumeration(t *testing.T) {
	enum := &stubEnumerator{groups: nil}
	reader := &recordingReader{}

	_, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if ok {
		t.Errorf("expected ok=false on empty enumeration, got true")
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected no Tail calls on empty enumeration, got %d", len(reader.calls))
	}
}

func TestNewPreviewModel_ReturnsFalseWhenFirstWindowHasZeroPanes(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: nil},
		},
	}
	reader := &recordingReader{}

	_, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if ok {
		t.Errorf("expected ok=false when first window has zero panes, got true")
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected no Tail calls when first window has zero panes, got %d", len(reader.calls))
	}
}

func TestNewPreviewModel_SetsFocusToZeroZeroOnSuccess(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0, 1}},
			{WindowIndex: 1, WindowName: "other", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hi")}

	m, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if !ok {
		t.Fatalf("expected ok=true on success, got false")
	}
	if m.windowIdx != 0 {
		t.Errorf("expected windowIdx=0, got %d", m.windowIdx)
	}
	if m.paneIdx != 0 {
		t.Errorf("expected paneIdx=0, got %d", m.paneIdx)
	}
}

func TestNewPreviewModel_ReadsTailForZeroZeroPaneSynchronously(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 2, WindowName: "main", PaneIndices: []int{5, 6}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello")}

	_, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	if len(reader.calls) != 1 {
		t.Fatalf("expected exactly 1 Tail call, got %d", len(reader.calls))
	}
	want := state.SanitizePaneKey("work", 2, 5)
	if reader.calls[0] != want {
		t.Errorf("expected Tail called with paneKey %q, got %q", want, reader.calls[0])
	}
}

func TestNewPreviewModel_PassesRawANSIBytesVerbatimToSetContent(t *testing.T) {
	raw := []byte("\x1b[31mred\x1b[0m")
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: raw}

	m, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	view := m.View()
	if !strings.Contains(view, "\x1b[31m") || !strings.Contains(view, "red") || !strings.Contains(view, "\x1b[0m") {
		t.Errorf("expected View() to contain raw ANSI bytes verbatim, got %q", view)
	}
}

func TestNewPreviewModel_PositionsViewportAtScrollTailOnInitialOpen(t *testing.T) {
	// Build content with more lines than viewport height so that GotoBottom
	// must explicitly run for AtBottom() to be true.
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("line\n")
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte(b.String())}

	m, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	if !m.viewport.AtBottom() {
		t.Errorf("expected viewport.AtBottom()=true immediately after construction, got false")
	}
}

func TestNewPreviewModel_ReturnsTrueWhenTailReturnsNilNil(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: nil, err: nil}

	_, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if !ok {
		t.Errorf("expected ok=true when Tail returns (nil, nil), got false")
	}
}

func TestNewPreviewModel_ReturnsTrueWhenTailReturnsNilError(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: nil, err: errors.New("EACCES")}

	_, ok := NewPreviewModel("work", enum, reader, 80, 24)

	if !ok {
		t.Errorf("expected ok=true when Tail returns (nil, err), got false")
	}
}

func TestNewPreviewModel_ConstructsFreshModelPerCallWithNoCarriedState(t *testing.T) {
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("first")}

	_, ok1 := NewPreviewModel("work", enum, reader, 80, 24)
	if !ok1 {
		t.Fatalf("first call: expected ok=true, got false")
	}

	// Mutate reader payload between calls — second model must observe the
	// new bytes, proving a fresh Tail occurred and no caching is in play.
	reader.bytes = []byte("second")

	_, ok2 := NewPreviewModel("work", enum, reader, 80, 24)
	if !ok2 {
		t.Fatalf("second call: expected ok=true, got false")
	}

	if enum.calls != 2 {
		t.Errorf("expected 2 enumeration calls (one per construction), got %d", enum.calls)
	}
	if len(reader.calls) != 2 {
		t.Errorf("expected 2 Tail calls (one per construction), got %d", len(reader.calls))
	}
}
