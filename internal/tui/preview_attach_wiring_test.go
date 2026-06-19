package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// fakePreviewAttacher is a test-only PreviewAttacher that records Run
// invocations. Returns a no-op tea.Cmd so callers can dispatch without
// triggering any tmux side effects.
type fakePreviewAttacher struct {
	calls []recordedAttacherCall
}

type recordedAttacherCall struct {
	session string
	window  int
	pane    int
}

func (f *fakePreviewAttacher) Run(session string, window, pane int) tea.Cmd {
	f.calls = append(f.calls, recordedAttacherCall{session: session, window: window, pane: pane})
	return func() tea.Msg { return nil }
}

func TestWithPreviewAttachPipeline_WiresAttacherOntoModel(t *testing.T) {
	attacher := &fakePreviewAttacher{}
	m := New(nil, WithPreviewAttachPipeline(attacher))

	if m.previewAttacher == nil {
		t.Fatalf("previewAttacher = nil; want fakePreviewAttacher")
	}
	if m.previewAttacher != PreviewAttacher(attacher) {
		t.Errorf("previewAttacher = %#v; want %#v", m.previewAttacher, attacher)
	}
}

func TestNewPreviewModel_PropagatesAttacherOntoPreviewModel(t *testing.T) {
	attacher := &fakePreviewAttacher{}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("x")}

	m, ok := NewPreviewModel("work", enum, reader, attacher, 80, 24)
	if !ok {
		t.Fatalf("NewPreviewModel: ok=false; want true")
	}
	if m.attacher == nil {
		t.Fatalf("previewModel.attacher = nil; want fakePreviewAttacher")
	}
	if m.attacher != PreviewAttacher(attacher) {
		t.Errorf("previewModel.attacher = %#v; want %#v", m.attacher, attacher)
	}
}

func TestNewPreviewModel_AcceptsNilAttacher(t *testing.T) {
	// Tests that do not exercise Enter pass nil; the constructor must not
	// require an attacher to be non-nil.
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("x")}

	m, ok := NewPreviewModel("work", enum, reader, nil, 80, 24)
	if !ok {
		t.Fatalf("NewPreviewModel: ok=false; want true")
	}
	if m.attacher != nil {
		t.Errorf("previewModel.attacher = %#v; want nil", m.attacher)
	}
}

func TestSpaceOnSessionsPage_PassesModelAttacherIntoPreviewModel(t *testing.T) {
	// Behavioural assertion: when Space opens preview, the Model's previewAttacher
	// is propagated onto the constructed previewModel via NewPreviewModel.
	attacher := &fakePreviewAttacher{}
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
	}
	enum := &stubEnumerator{
		groups: []tmux.WindowGroup{
			{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}},
		},
	}
	reader := &recordingReader{bytes: []byte("hello")}
	m := modelWithSeams(sessions, enum, reader)
	m.previewAttacher = attacher

	updated, _ := m.Update(keySpaceMsg())
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", updated)
	}
	if got.activePage != pagePreview {
		t.Fatalf("expected activePage=pagePreview, got %v", got.activePage)
	}
	if got.preview.attacher == nil {
		t.Fatalf("preview.attacher = nil; want propagated fakePreviewAttacher")
	}
	if got.preview.attacher != PreviewAttacher(attacher) {
		t.Errorf("preview.attacher = %#v; want %#v", got.preview.attacher, attacher)
	}
}

func TestNewPreviewAttachPipeline_ReturnsNonNilAttacher(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	logger, _ := newTestLogger(t)

	p := NewPreviewAttachPipeline(tm, logger)
	if p == nil {
		t.Fatalf("NewPreviewAttachPipeline returned nil")
	}
	// Smoke test that the returned attacher executes the pipeline:
	// invoking Run should drive the fake tmux through at least HasSessionProbe.
	cmd := p.Run("foo", 1, 0)
	if cmd == nil {
		t.Fatalf("Run returned nil tea.Cmd")
	}
	_ = cmd()
	if len(tm.calls) == 0 {
		t.Errorf("expected at least one tmux call after Run, got 0")
	}
}

func TestNewPreviewAttachPipeline_NilLoggerTolerated(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}

	p := NewPreviewAttachPipeline(tm, nil)
	if p == nil {
		t.Fatalf("NewPreviewAttachPipeline returned nil with nil logger")
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Run panicked with nil logger: %v", r)
		}
	}()
	_ = p.Run("foo", 1, 0)()
}
