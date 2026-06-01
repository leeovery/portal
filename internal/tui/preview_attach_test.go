package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// newSinglePaneEnumerator returns a *stubEnumerator pre-populated with the
// minimal single-window / single-pane topology used by preview-attach tests
// that only need a degenerate session shape. Centralised so test files do not
// repeat the literal across many call sites.
func newSinglePaneEnumerator() *stubEnumerator {
	return &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", PaneIndices: []int{0}}}}
}

// recordedCall captures a single invocation against fakePreviewAttachTmux so
// tests can assert both the relative ordering of calls and the per-call
// arguments. The verb field doubles as a hook for "called once each" asserts.
type recordedCall struct {
	verb    string
	session string
	window  int
	pane    int
}

// fakePreviewAttachTmux is the test-only stand-in for the production
// previewAttachTmux interface. Each method records its call into calls and
// returns the per-method canned outcome the test set up.
type fakePreviewAttachTmux struct {
	calls []recordedCall

	hasPresent bool
	hasErr     error

	selectWindowErr error
	selectPaneErr   error
}

func (f *fakePreviewAttachTmux) HasSessionProbe(name string) (bool, error) {
	f.calls = append(f.calls, recordedCall{verb: "has", session: name})
	return f.hasPresent, f.hasErr
}

func (f *fakePreviewAttachTmux) SelectWindow(session string, window int) error {
	f.calls = append(f.calls, recordedCall{verb: "selWin", session: session, window: window})
	return f.selectWindowErr
}

func (f *fakePreviewAttachTmux) SelectPane(session string, window, pane int) error {
	f.calls = append(f.calls, recordedCall{verb: "selPane", session: session, window: window, pane: pane})
	return f.selectPaneErr
}

// fakePreviewConnector records its single Connect invocation. The connector
// is no longer wired into the pipeline (Phase 3 task 3-1 moved the handoff
// post-TUI); this fake remains useful for tests that simulate the
// processTUIResult-side handoff.
type fakePreviewConnector struct {
	calls []string
	err   error
}

func (f *fakePreviewConnector) Connect(name string) error {
	f.calls = append(f.calls, name)
	return f.err
}

// previewCaptureSink is a slog.Handler that records every record into a text
// body in the shape "<LEVEL> component=<c> <msg> key=value..." so the
// preview-attach tests can assert on the level label, the bound component, and
// per-call attrs after the observability migration retyped the pipeline's
// logger to *slog.Logger.
type previewCaptureSink struct {
	mu    sync.Mutex
	lines []string
	// shared points at the lines-owning sink so handlers derived via
	// WithAttrs/WithGroup (e.g. the .With("component", ...) binding) still
	// record into the same buffer.
	shared *previewCaptureSink
	// bound holds attrs accumulated via WithAttrs (notably the component
	// binding) so they are rendered on every record the derived handler emits.
	bound []slog.Attr
}

func (s *previewCaptureSink) owner() *previewCaptureSink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

func (s *previewCaptureSink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (s *previewCaptureSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &previewCaptureSink{shared: s.owner(), bound: next}
}

func (s *previewCaptureSink) WithGroup(_ string) slog.Handler { return s }

func (s *previewCaptureSink) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Level.String())
	b.WriteString(" ")
	b.WriteString(r.Message)
	for _, a := range s.bound {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
		return true
	})
	owner := s.owner()
	owner.mu.Lock()
	owner.lines = append(owner.lines, b.String())
	owner.mu.Unlock()
	return nil
}

func (s *previewCaptureSink) body() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

// newTestLogger returns a capturing *slog.Logger bound to the preview
// component plus the sink so tests can read back what was logged.
func newTestLogger(t *testing.T) (*slog.Logger, *previewCaptureSink) {
	t.Helper()
	sink := &previewCaptureSink{}
	return slog.New(sink).With("component", "preview"), sink
}

// readLog renders the captured body of a previewCaptureSink.
func readLog(t *testing.T, sink *previewCaptureSink) string {
	t.Helper()
	return sink.body()
}

// runPipelineCmd invokes the tea.Cmd returned by Run and returns its message.
// The three-call pre-select path runs synchronously inside the goroutine the
// Cmd represents, so executing the Cmd inline yields the terminal message
// directly (previewAttachBailMsg on bail, previewAttachSelectedMsg on
// success).
func runPipelineCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("Run returned nil tea.Cmd; expected non-nil")
	}
	return cmd()
}

func TestPreviewAttachPipelineRunReturnsNonNilCmd(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	if cmd := p.Run("foo", 1, 0); cmd == nil {
		t.Fatalf("Run returned nil tea.Cmd")
	}
}

func TestPreviewAttachPipelineSuccessPathOrderAndArgs(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 2, 5))

	if len(tm.calls) != 3 {
		t.Fatalf("expected 3 tmux calls, got %d: %#v", len(tm.calls), tm.calls)
	}
	if tm.calls[0] != (recordedCall{verb: "has", session: "foo"}) {
		t.Errorf("call[0] = %#v, want has(foo)", tm.calls[0])
	}
	if tm.calls[1] != (recordedCall{verb: "selWin", session: "foo", window: 2}) {
		t.Errorf("call[1] = %#v, want selWin(foo,2)", tm.calls[1])
	}
	if tm.calls[2] != (recordedCall{verb: "selPane", session: "foo", window: 2, pane: 5}) {
		t.Errorf("call[2] = %#v, want selPane(foo,2,5)", tm.calls[2])
	}
	got, ok := msg.(previewAttachSelectedMsg)
	if !ok {
		t.Fatalf("message type = %T, want previewAttachSelectedMsg", msg)
	}
	if got.Session != "foo" {
		t.Errorf("Session = %q, want %q", got.Session, "foo")
	}
}

func TestPreviewAttachPipelineBailsOnExitError(t *testing.T) {
	// Build a real *exec.ExitError so the discriminator routes to bail.
	exitErr := makeExitError(t)
	tm := &fakePreviewAttachTmux{hasPresent: false, hasErr: exitErr}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 1, 0))

	bail, ok := msg.(previewAttachBailMsg)
	if !ok {
		t.Fatalf("message type = %T, want previewAttachBailMsg", msg)
	}
	if bail.Session != "foo" {
		t.Errorf("bail.Session = %q, want %q", bail.Session, "foo")
	}
	if len(tm.calls) != 1 || tm.calls[0].verb != "has" {
		t.Errorf("expected single has-session call, got %#v", tm.calls)
	}
}

func TestPreviewAttachPipelineOSLayerHasSessionErrorProceedsAndLogsWarn(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true, hasErr: errors.New("exec: no tmux binary")}
	logger, sink := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 1, 0))

	if len(tm.calls) != 3 {
		t.Fatalf("expected 3 tmux calls after OS-layer probe error, got %d: %#v", len(tm.calls), tm.calls)
	}
	if _, ok := msg.(previewAttachSelectedMsg); !ok {
		t.Fatalf("message type = %T, want previewAttachSelectedMsg", msg)
	}
	content := readLog(t, sink)
	if !strings.Contains(content, "WARN") || !strings.Contains(content, "preview") {
		t.Errorf("log %q missing WARN + ComponentPreview", content)
	}
}

func TestPreviewAttachPipelineSelectWindowErrorLogsAndProceeds(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true, selectWindowErr: errors.New("no such window")}
	logger, sink := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 9, 0))

	if len(tm.calls) != 3 {
		t.Fatalf("expected pipeline to proceed past select-window, got %d calls", len(tm.calls))
	}
	if _, ok := msg.(previewAttachSelectedMsg); !ok {
		t.Fatalf("message type = %T, want previewAttachSelectedMsg", msg)
	}
	content := readLog(t, sink)
	if !strings.Contains(content, "WARN") || !strings.Contains(content, "preview") {
		t.Errorf("log %q missing WARN + ComponentPreview for select-window failure", content)
	}
}

func TestPreviewAttachPipelineSelectPaneErrorLogsAndProceeds(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true, selectPaneErr: errors.New("no such pane")}
	logger, sink := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 1, 9))

	if len(tm.calls) != 3 {
		t.Fatalf("expected 3 tmux calls, got %d", len(tm.calls))
	}
	if _, ok := msg.(previewAttachSelectedMsg); !ok {
		t.Fatalf("message type = %T, want previewAttachSelectedMsg", msg)
	}
	content := readLog(t, sink)
	if !strings.Contains(content, "WARN") || !strings.Contains(content, "preview") {
		t.Errorf("log %q missing WARN + ComponentPreview for select-pane failure", content)
	}
}

func TestPreviewAttachPipelineBothSelectsErrorBothLoggedAndSelectedEmitted(t *testing.T) {
	tm := &fakePreviewAttachTmux{
		hasPresent:      true,
		selectWindowErr: errors.New("no window"),
		selectPaneErr:   errors.New("no pane"),
	}
	logger, sink := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 1, 0))

	if _, ok := msg.(previewAttachSelectedMsg); !ok {
		t.Fatalf("message type = %T, want previewAttachSelectedMsg even after both select failures", msg)
	}
	content := readLog(t, sink)
	warnCount := strings.Count(content, "WARN")
	if warnCount < 2 {
		t.Errorf("expected at least 2 WARN entries (one per select failure), got %d in %q", warnCount, content)
	}
	if !strings.Contains(content, "preview") {
		t.Errorf("expected ComponentPreview in log, got %q", content)
	}
}

func TestPreviewAttachPipelineSilentLoggerDoesNotPanic(t *testing.T) {
	// Combine every WARN-trigger so every Warn call inside Run is exercised.
	// Post-migration the pipeline always holds a real *slog.Logger (production
	// passes log.For("preview")); a silent io.Discard-backed logger must drive
	// every WARN path without panicking.
	tm := &fakePreviewAttachTmux{
		hasPresent:      true,
		hasErr:          errors.New("os-layer probe failure"),
		selectWindowErr: errors.New("no window"),
		selectPaneErr:   errors.New("no pane"),
	}
	silent := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := &previewAttachPipeline{tmux: tm, logger: silent}

	// Run-and-execute must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pipeline panicked with silent logger: %v", r)
		}
	}()
	_ = runPipelineCmd(t, p.Run("foo", 1, 0))
}

func TestPreviewAttachPipelineEmptySessionBailsBeforeTmuxCalls(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	msg := runPipelineCmd(t, p.Run("", 1, 0))

	bail, ok := msg.(previewAttachBailMsg)
	if !ok {
		t.Fatalf("message type = %T, want previewAttachBailMsg", msg)
	}
	if bail.Session != "" {
		t.Errorf("bail.Session = %q, want empty string", bail.Session)
	}
	if len(tm.calls) != 0 {
		t.Errorf("tmux calls = %#v, want none on empty-session guard", tm.calls)
	}
}

// makeExitError synthesises a real *exec.ExitError by running a command that
// is guaranteed to exit non-zero. The discriminator inside the pipeline uses
// errors.As(err, &*exec.ExitError); fabricating a real one here keeps the
// test honest about what the production wiring will see from tmux.
func makeExitError(t *testing.T) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit 1")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit from sh -c 'exit 1'")
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *exec.ExitError, got %T", err)
	}
	return err
}
