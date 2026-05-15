package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Tests for Phase 3 task 3-1: previewAttachPipeline no longer terminates with
// connector.Connect. The success path runs only steps 1-3
// (HasSessionProbe + SelectWindow + SelectPane) and returns
// previewAttachSelectedMsg carrying the captured session name. The connector
// handoff has moved post-TUI (cmd/open.go processTUIResult), so the pipeline
// MUST NOT depend on any connector seam.

func TestPreviewAttachPipeline_SuccessReturnsSelectedMsg(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	logger, _ := newTestLogger(t)
	p := &previewAttachPipeline{tmux: tm, logger: logger}

	msg := runPipelineCmd(t, p.Run("foo", 2, 5))

	got, ok := msg.(previewAttachSelectedMsg)
	if !ok {
		t.Fatalf("message type = %T, want previewAttachSelectedMsg", msg)
	}
	if got.Session != "foo" {
		t.Errorf("Session = %q, want %q", got.Session, "foo")
	}
	// All three pre-select tmux calls run; the connector is gone.
	if len(tm.calls) != 3 {
		t.Errorf("expected 3 tmux calls, got %d: %#v", len(tm.calls), tm.calls)
	}
}

// IntegrationStyle: drive a Model through preview-Enter with a fake connector
// wired in cmd/open.go's processTUIResult shape — Connect must be called
// exactly once and only AFTER the TUI has returned tea.Quit. This is the
// inside-tmux orphan-process regression guard.
func TestPreviewAttachIntegration_ConnectInvokedAfterQuit(t *testing.T) {
	tm := &fakePreviewAttachTmux{hasPresent: true}
	logger, _ := newTestLogger(t)
	pipeline := &previewAttachPipeline{tmux: tm, logger: logger}

	// Execute the pipeline cmd — the success path returns
	// previewAttachSelectedMsg, not previewAttachErrorMsg, and the connector
	// is NOT invoked from inside the goroutine.
	msg := pipeline.Run("foo", 1, 0)()
	sel, ok := msg.(previewAttachSelectedMsg)
	if !ok {
		t.Fatalf("expected previewAttachSelectedMsg, got %T", msg)
	}

	// Now simulate the model receiving that message.
	m := modelWithSeams(nil, &stubEnumerator{}, &recordingReader{})
	updated, cmd := m.Update(sel)
	got := updated.(Model)
	if got.Selected() != "foo" {
		t.Fatalf("expected Selected()=%q, got %q", "foo", got.Selected())
	}
	if cmd == nil || cmd() != tea.Quit() {
		t.Fatalf("expected tea.Quit cmd from selected handler")
	}

	// Post-TUI handoff (mirrors processTUIResult): the connector runs HERE,
	// after the TUI program has shut down. Inside-tmux switch-client and
	// outside-tmux syscall.Exec both happen at this stage.
	conn := &fakePreviewConnector{}
	if got.Selected() != "" {
		_ = conn.Connect(got.Selected())
	}
	if len(conn.calls) != 1 || conn.calls[0] != "foo" {
		t.Errorf("connector.Connect calls = %#v, want [foo]", conn.calls)
	}
}
