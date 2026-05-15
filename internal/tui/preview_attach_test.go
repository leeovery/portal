package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/state"
)

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

// fakePreviewConnector records its single Connect invocation and returns the
// canned error (nil for the in-tmux switch success path, non-nil for the
// connector-error envelope path).
type fakePreviewConnector struct {
	calls []string
	err   error
}

func (f *fakePreviewConnector) Connect(name string) error {
	f.calls = append(f.calls, name)
	return f.err
}

// newTestLogger opens a *state.Logger at a temp portal.log path so tests can
// read back what was logged. rotate=false to keep the file in place across
// the test's lifetime. Closed via t.Cleanup.
func newTestLogger(t *testing.T) (*state.Logger, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "portal.log")
	logger, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger, logPath
}

// readLog returns the log file's contents as a string, or "" if missing.
func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}
		t.Fatalf("read log: %v", err)
	}
	return string(b)
}

// runPipelineCmd invokes the tea.Cmd returned by Run and returns its message.
// All four-call paths are synchronous inside the goroutine the Cmd represents,
// so executing the Cmd inline yields the terminal message directly.
func runPipelineCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatalf("Run returned nil tea.Cmd; expected non-nil")
	}
	return cmd()
}

func TestPreviewAttachPipelineRunReturnsNonNilCmd(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	conn := &fakePreviewConnector{}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

	if cmd := p.Run("foo", 1, 0); cmd == nil {
		t.Fatalf("Run returned nil tea.Cmd")
	}
}

func TestPreviewAttachPipelineSuccessPathOrderAndArgs(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	conn := &fakePreviewConnector{}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

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
	if len(conn.calls) != 1 || conn.calls[0] != "foo" {
		t.Errorf("connector calls = %#v, want exactly [foo]", conn.calls)
	}
	got, ok := msg.(previewAttachErrorMsg)
	if !ok {
		t.Fatalf("message type = %T, want previewAttachErrorMsg", msg)
	}
	if got.Err != nil {
		t.Errorf("Err = %v, want nil", got.Err)
	}
}

func TestPreviewAttachPipelineBailsOnExitError(t *testing.T) {
	// Build a real *exec.ExitError so the discriminator routes to bail.
	exitErr := makeExitError(t)
	tm := &fakePreviewAttachTmux{hasPresent: false, hasErr: exitErr}
	conn := &fakePreviewConnector{}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

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
	if len(conn.calls) != 0 {
		t.Errorf("connector called %d times, want 0", len(conn.calls))
	}
}

func TestPreviewAttachPipelineOSLayerHasSessionErrorProceedsAndLogsWarn(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true, hasErr: errors.New("exec: no tmux binary")}
	conn := &fakePreviewConnector{}
	logger, logPath := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 1, 0))

	if len(tm.calls) != 3 {
		t.Fatalf("expected 3 tmux calls after OS-layer probe error, got %d: %#v", len(tm.calls), tm.calls)
	}
	if _, ok := msg.(previewAttachErrorMsg); !ok {
		t.Fatalf("message type = %T, want previewAttachErrorMsg", msg)
	}
	content := readLog(t, logPath)
	if !strings.Contains(content, "WARN") || !strings.Contains(content, state.ComponentPreview) {
		t.Errorf("log %q missing WARN + ComponentPreview", content)
	}
}

func TestPreviewAttachPipelineSelectWindowErrorLogsAndProceeds(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true, selectWindowErr: errors.New("no such window")}
	conn := &fakePreviewConnector{}
	logger, logPath := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 9, 0))

	if len(tm.calls) != 3 {
		t.Fatalf("expected pipeline to proceed past select-window, got %d calls", len(tm.calls))
	}
	if len(conn.calls) != 1 {
		t.Errorf("connector calls = %d, want 1", len(conn.calls))
	}
	if _, ok := msg.(previewAttachErrorMsg); !ok {
		t.Fatalf("message type = %T, want previewAttachErrorMsg", msg)
	}
	content := readLog(t, logPath)
	if !strings.Contains(content, "WARN") || !strings.Contains(content, state.ComponentPreview) {
		t.Errorf("log %q missing WARN + ComponentPreview for select-window failure", content)
	}
}

func TestPreviewAttachPipelineSelectPaneErrorLogsAndProceeds(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true, selectPaneErr: errors.New("no such pane")}
	conn := &fakePreviewConnector{}
	logger, logPath := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 1, 9))

	if len(tm.calls) != 3 {
		t.Fatalf("expected 3 tmux calls, got %d", len(tm.calls))
	}
	if len(conn.calls) != 1 {
		t.Errorf("connector calls = %d, want 1", len(conn.calls))
	}
	if _, ok := msg.(previewAttachErrorMsg); !ok {
		t.Fatalf("message type = %T, want previewAttachErrorMsg", msg)
	}
	content := readLog(t, logPath)
	if !strings.Contains(content, "WARN") || !strings.Contains(content, state.ComponentPreview) {
		t.Errorf("log %q missing WARN + ComponentPreview for select-pane failure", content)
	}
}

func TestPreviewAttachPipelineBothSelectsErrorBothLoggedConnectorFires(t *testing.T) {
	tm := &fakePreviewAttachTmux{
		hasPresent:      true,
		selectWindowErr: errors.New("no window"),
		selectPaneErr:   errors.New("no pane"),
	}
	conn := &fakePreviewConnector{}
	logger, logPath := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

	_ = runPipelineCmd(t, p.Run("foo", 1, 0))

	if len(conn.calls) != 1 {
		t.Errorf("connector calls = %d, want 1 (must fire even after both selects fail)", len(conn.calls))
	}
	content := readLog(t, logPath)
	warnCount := strings.Count(content, "WARN")
	if warnCount < 2 {
		t.Errorf("expected at least 2 WARN entries (one per select failure), got %d in %q", warnCount, content)
	}
	if !strings.Contains(content, state.ComponentPreview) {
		t.Errorf("expected ComponentPreview in log, got %q", content)
	}
}

func TestPreviewAttachPipelineConnectorErrorIsWrappedInMsg(t *testing.T) {
	connectorErr := errors.New("switch-client failed")
	tm := &fakePreviewAttachTmux{hasPresent: true}
	conn := &fakePreviewConnector{err: connectorErr}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 1, 0))

	got, ok := msg.(previewAttachErrorMsg)
	if !ok {
		t.Fatalf("message type = %T, want previewAttachErrorMsg", msg)
	}
	if !errors.Is(got.Err, connectorErr) {
		t.Errorf("Err = %v, want connectorErr (%v)", got.Err, connectorErr)
	}
}

func TestPreviewAttachPipelineConnectorSuccessReturnsNilErrMsg(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	conn := &fakePreviewConnector{err: nil}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 1, 0))

	got, ok := msg.(previewAttachErrorMsg)
	if !ok {
		t.Fatalf("message type = %T, want previewAttachErrorMsg", msg)
	}
	if got.Err != nil {
		t.Errorf("Err = %v, want nil on connector success", got.Err)
	}
}

func TestPreviewAttachPipelineNilLoggerDoesNotPanic(t *testing.T) {
	// Combine every WARN-trigger so the nil-logger path is exercised across
	// every Warn call inside Run. *state.Logger has a documented nil-receiver
	// no-op contract — see internal/state/logger.go.
	tm := &fakePreviewAttachTmux{
		hasPresent:      true,
		hasErr:          errors.New("os-layer probe failure"),
		selectWindowErr: errors.New("no window"),
		selectPaneErr:   errors.New("no pane"),
	}
	conn := &fakePreviewConnector{}
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: nil}

	// Run-and-execute must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("pipeline panicked with nil logger: %v", r)
		}
	}()
	_ = runPipelineCmd(t, p.Run("foo", 1, 0))
}

func TestPreviewAttachPipelineEmptySessionBailsBeforeTmuxCalls(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	conn := &fakePreviewConnector{}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, connector: conn, logger: logger}

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
	if len(conn.calls) != 0 {
		t.Errorf("connector calls = %d, want 0", len(conn.calls))
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
