package tui

import (
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/tmux"
)

func newSinglePaneEnumerator() *stubEnumerator {
	return &stubEnumerator{groups: []tmux.WindowGroup{{WindowIndex: 0, WindowName: "main", Panes: []tmux.WindowPane{{Index: 0}}}}}
}

type recordedCall struct {
	verb    string
	session string
	window  int
	pane    int
}

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

type fakePreviewConnector struct {
	calls []string
	err   error
}

func (f *fakePreviewConnector) Connect(name string) error {
	f.calls = append(f.calls, name)
	return f.err
}

func newTestLogger(t *testing.T) (*slog.Logger, *logtest.Sink) {
	t.Helper()
	logger, sink := logtest.NewCaptureLogger(t)
	return logger.With("component", "preview"), sink
}

func readLog(t *testing.T, sink *logtest.Sink) string {
	t.Helper()
	return sink.Body()
}

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
	tm := &fakePreviewAttachTmux{
		hasPresent:      true,
		hasErr:          errors.New("os-layer probe failure"),
		selectWindowErr: errors.New("no window"),
		selectPaneErr:   errors.New("no pane"),
	}
	silent := log.Discard()
	p := &previewAttachPipeline{tmux: tm, logger: silent}

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

func makeExitError(t *testing.T) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit 1")
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit from sh -c 'exit 1'")
	}
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("expected *exec.ExitError, got %T", err)
	}
	return err
}
