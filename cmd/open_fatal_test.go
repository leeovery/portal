package cmd

// Task spectrum-tui-design-5-6 — openTUI fatal-error exit parity (§10.5).
//
// On the concurrent cold/TUI path a fatal bootstrap step parks the model in the
// error frame and q/Esc quits the Bubble Tea program. processTUIResult must then
// return the model's carried *bootstrap.FatalError (not nil, no connect) so
// Execute writes the single user line and main.classify maps it to code 1 — the
// SAME classification today's synchronous warm/CLI path produces, so the exit is
// byte-for-byte unchanged.
//
// cmd rule: NO t.Parallel().

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// TestProcessTUIResult_ReturnsFatalError asserts a model carrying a fatal returns
// that fatal from processTUIResult (no connect), and the returned error is the
// original *bootstrap.FatalError instance so classification is byte-for-byte the
// same as the synchronous path.
func TestProcessTUIResult_ReturnsFatalError(t *testing.T) {
	fatal := bootstrap.NewFatal("Portal failed to set @portal-restoring marker: permission denied", errors.New("permission denied"))

	lister := &mockSessionLister{}
	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
	m := tui.New(lister, tui.WithServerStarted(true), tui.WithProgressReceiver(receiver))
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 3, Message: fatal.UserMessage, Err: fatal})

	connector := &mockSessionConnector{}
	err := processTUIResult(model.(tui.Model), connector)

	if err == nil {
		t.Fatal("processTUIResult returned nil on a fatal; want the *bootstrap.FatalError")
	}
	// Must be the SAME *bootstrap.FatalError instance (byte-for-byte classification parity).
	var got *bootstrap.FatalError
	if !errors.As(err, &got) {
		t.Fatalf("returned error is not a *bootstrap.FatalError; got %T", err)
	}
	if got != fatal {
		t.Errorf("returned a different *bootstrap.FatalError instance; want the original")
	}
	if connector.connectedTo != "" {
		t.Errorf("connector was called on a fatal (%q); must skip connect", connector.connectedTo)
	}
}

// TestProcessTUIResult_NoFatalUnchanged asserts the no-fatal path is byte-for-byte
// unchanged by the fatal-precedence addition: a model with NO fatal still connects
// on a selection and returns nil on a clean exit. This guards the warm/CLI and
// non-fatal cold paths against regression from the new FatalError() pre-check.
func TestProcessTUIResult_NoFatalUnchanged(t *testing.T) {
	t.Run("clean exit returns nil and does not connect", func(t *testing.T) {
		m := tui.New(&mockSessionLister{})
		connector := &mockSessionConnector{}
		if err := processTUIResult(m, connector); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "" {
			t.Errorf("connector called on clean exit: %q", connector.connectedTo)
		}
	})

	t.Run("selection still connects", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tmux.Session{{Name: "dev", Windows: 1}})
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		connector := &mockSessionConnector{}
		if err := processTUIResult(model.(tui.Model), connector); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "dev" {
			t.Errorf("connector called with %q, want %q", connector.connectedTo, "dev")
		}
	})
}
