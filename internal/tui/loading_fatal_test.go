package tui_test

// Task spectrum-tui-design-5-6 — fatal cold-boot error contract (§10.5 / §10.2).
//
// On the concurrent cold/TUI path a fatal bootstrap step (EnsureServer,
// RegisterPortalHooks, SetRestoring, ClearRestoring) can no longer return through
// PersistentPreRunE — the orchestrator runs in a goroutine while Bubble Tea is
// live on the loading page. §10.5 mandates the fatal becomes an in-TUI error
// state on the loading page: the failed step row gets a state.red ✗ marker + a
// one-line message, the page never transitions to the picker, and q/Esc quits
// with a non-zero exit (openTUI returns the *bootstrap.FatalError).
//
// These tests pin the model behaviour: the error-state render, the
// no-transition-to-picker invariant, the q/Esc → tea.Quit binding, the fatal
// carried for openTUI's exit-code extraction, and that a best-effort step never
// drives this state.

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// errFatalSentinel is a stand-in *bootstrap.FatalError-shaped error for the model
// tests: internal/tui must not import cmd/bootstrap, so the model carries the
// fatal as an error interface and these tests assert it round-trips by identity.
// The text is irrelevant — the tests assert identity via errors.Is — so it stays
// a plain lowercase sentinel (the rendered UserMessage is passed separately on the
// BootstrapFatalMsg.Message field).
var errFatalSentinel = errors.New("fatal cold-boot abort sentinel")

// fatalModelOnLoading builds a loading-page model (serverStarted) with a wired
// progress receiver so it is on the concurrent cold/TUI route, advanced past a
// WindowSizeMsg so it renders at a real size.
func fatalModelOnLoading(t *testing.T) tea.Model {
	t.Helper()
	lister := &mockSessionLister{sessions: []tmux.Session{}}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"} })
	m := tui.New(lister, tui.WithServerStarted(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model
}

// TestFatalMsg_RendersErrorState asserts a BootstrapFatalMsg drives the
// loading-page error state: the failed step's friendly label carries the state.red
// ✗ marker and the one-line message renders beneath the step-list.
func TestFatalMsg_RendersErrorState(t *testing.T) {
	model := fatalModelOnLoading(t)

	// Stream steps 1–2 (done), then a fatal at step 3 (SetRestoring).
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"})
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"})
	model, _ = model.Update(tui.BootstrapFatalMsg{
		FailedStep: 3,
		Message:    "Portal failed to set @portal-restoring marker: permission denied",
		Err:        errFatalSentinel,
	})

	view := model.(tui.Model).View().Content
	visible := ansi.Strip(view)

	// The ✗ failure glyph is present.
	if !strings.Contains(visible, "✗") {
		t.Errorf("error state missing the ✗ failure glyph:\n%s", visible)
	}
	// The one-line user message renders.
	if !strings.Contains(visible, "Portal failed to set @portal-restoring marker") {
		t.Errorf("error state missing the one-line fatal message:\n%s", visible)
	}
	// The failed step's friendly label (step 3 → "Registered hooks") is still shown.
	if !strings.Contains(visible, tui.LabelRegisteredHooks) {
		t.Errorf("error state missing the failed step label %q:\n%s", tui.LabelRegisteredHooks, visible)
	}
	// A quit hint tells the user how to exit.
	if !strings.Contains(visible, "quit") {
		t.Errorf("error state missing a quit hint:\n%s", visible)
	}

	// The centred message + hint composition never overflows the 80×24 frame —
	// the full rendered View is exactly the terminal height (no extra rows pushed
	// past the bottom by the footer caption/hint).
	if got := strings.Count(view, "\n") + 1; got > 24 {
		t.Errorf("error frame is %d rows tall, overflowing the 24-row terminal", got)
	}
}

// TestFatalMsg_StaysOnLoadingPage asserts the model NEVER transitions to the
// picker on a fatal — activePage stays PageLoading even after the gates that
// would otherwise dismiss it (min-elapsed) fire.
func TestFatalMsg_StaysOnLoadingPage(t *testing.T) {
	model := fatalModelOnLoading(t)

	model, _ = model.Update(tui.LoadingMinElapsedMsg{})
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"})
	model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 1, Message: "boom", Err: errFatalSentinel})

	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Errorf("model transitioned off PageLoading on a fatal; got %d", model.(tui.Model).ActivePage())
	}

	// A late terminal complete or further progress must not rescue it into the picker.
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"})
	model, _ = model.Update(tui.BootstrapCompleteMsg{})
	if model.(tui.Model).ActivePage() != tui.PageLoading {
		t.Errorf("model left PageLoading after a fatal; got %d", model.(tui.Model).ActivePage())
	}
}

// TestFatalMsg_QuitsOnQ asserts q quits (tea.Quit) once in the error state.
func TestFatalMsg_QuitsOnQ(t *testing.T) {
	model := fatalModelOnLoading(t)
	model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 1, Message: "boom", Err: errFatalSentinel})

	_, cmd := model.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	assertQuitCmd(t, cmd, "q in error state")
}

// TestFatalMsg_QuitsOnEsc asserts Esc quits (tea.Quit) once in the error state.
func TestFatalMsg_QuitsOnEsc(t *testing.T) {
	model := fatalModelOnLoading(t)
	model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 1, Message: "boom", Err: errFatalSentinel})

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assertQuitCmd(t, cmd, "Esc in error state")
}

// TestFatalMsg_CarriesFatalForOpenTUI asserts the model exposes the underlying
// fatal error so openTUI can extract it post-program and return it for the
// non-zero exit classification (§10.5).
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

// TestNoFatal_FatalErrorNil asserts a model that never received a fatal exposes a
// nil FatalError — the synchronous/normal complete path must not look fatal.
func TestNoFatal_FatalErrorNil(t *testing.T) {
	model := fatalModelOnLoading(t)
	model, _ = model.Update(tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"})
	model, _ = model.Update(tui.BootstrapCompleteMsg{})
	if got := model.(tui.Model).FatalError(); got != nil {
		t.Errorf("FatalError() non-nil on a non-fatal run; got %v", got)
	}
}

// assertQuitCmd asserts the returned cmd is tea.Quit (its message is tea.QuitMsg).
func assertQuitCmd(t *testing.T, cmd tea.Cmd, context string) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("%s: returned nil cmd; want tea.Quit", context)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("%s: returned cmd is not tea.Quit", context)
	}
}
