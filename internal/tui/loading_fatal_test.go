package tui_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui"
)

var errFatalSentinel = errors.New("fatal cold-boot abort sentinel")

func fatalModelOnLoading(t *testing.T) tea.Model {
	t.Helper()
	lister := &mockSessionLister{sessions: []tui.Session{}}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
	m := tui.New(lister, tui.WithBootstrapping(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model
}

func TestFatalMsg_RendersErrorState(t *testing.T) {
	model := fatalModelOnLoading(t)

	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1})
	model, _ = model.Update(tui.BootstrapFatalMsg{
		FailedStep: 2,
		Message:    "Portal failed to remove the bootstrapped latch: permission denied",
		Err:        errFatalSentinel,
	})

	view := model.(tui.Model).View().Content
	visible := ansi.Strip(view)

	if !strings.Contains(visible, "✗") {
		t.Errorf("error state missing the ✗ failure glyph:\n%s", visible)
	}
	if !strings.Contains(visible, "Portal failed to remove the bootstrapped latch") {
		t.Errorf("error state missing the one-line fatal message:\n%s", visible)
	}
	if !strings.Contains(visible, tui.LabelStartingTheSaver) {
		t.Errorf("error state missing the failed step label %q:\n%s", tui.LabelStartingTheSaver, visible)
	}
	if !strings.Contains(visible, "quit") {
		t.Errorf("error state missing a quit hint:\n%s", visible)
	}

	if got := strings.Count(view, "\n") + 1; got > 24 {
		t.Errorf("error frame is %d rows tall, overflowing the 24-row terminal", got)
	}
}

func TestFatalMsg_StaysOnLoadingPage(t *testing.T) {
	model := fatalModelOnLoading(t)

	model, _ = model.Update(tui.LoadingMinElapsedMsg{})
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1})
	model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 1, Message: "boom", Err: errFatalSentinel})

	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Errorf("model transitioned off PageLoading on a fatal; got %d", model.(tui.Model).ActivePage())
	}

	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 2})
	model, _ = model.Update(tui.BootstrapCompleteMsg{})
	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Errorf("model left PageLoading after a fatal; got %d", model.(tui.Model).ActivePage())
	}
}

func TestFatalMsg_QuitsOnQ(t *testing.T) {
	model := fatalModelOnLoading(t)
	model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 1, Message: "boom", Err: errFatalSentinel})

	_, cmd := model.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	assertQuitCmd(t, cmd, "q in error state")
}

func TestFatalMsg_QuitsOnEsc(t *testing.T) {
	model := fatalModelOnLoading(t)
	model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 1, Message: "boom", Err: errFatalSentinel})

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assertQuitCmd(t, cmd, "Esc in error state")
}

func TestFatalMsg_CarriesFatalForOpenTUI(t *testing.T) {
	model := fatalModelOnLoading(t)
	model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 3, Message: "boom", Err: errFatalSentinel})

	got := model.(tui.Model).FatalError()
	if got == nil {
		t.Fatal("FatalError() returned nil after a fatal; want the carried error")
	}
	if !errors.Is(got, errFatalSentinel) {
		t.Errorf("FatalError() did not carry the original error; got %v", got)
	}
}

func TestNoFatal_FatalErrorNil(t *testing.T) {
	model := fatalModelOnLoading(t)
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1})
	model, _ = model.Update(tui.BootstrapCompleteMsg{})
	if got := model.(tui.Model).FatalError(); got != nil {
		t.Errorf("FatalError() non-nil on a non-fatal run; got %v", got)
	}
}

func assertQuitCmd(t *testing.T, cmd tea.Cmd, context string) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("%s: returned nil cmd; want tea.Quit", context)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("%s: returned cmd is not tea.Quit", context)
	}
}
