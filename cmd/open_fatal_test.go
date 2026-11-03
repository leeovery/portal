package cmd

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tui"
)

func TestProcessTUIResult_ReturnsFatalError(t *testing.T) {
	fatal := bootstrap.NewFatal("Portal failed to set @portal-restoring marker: permission denied", errors.New("permission denied"))

	lister := asPickerLister(&mockSessionLister{})
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
	// Must be the same instance, so classification stays identical.
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

func TestProcessTUIResult_NoFatalUnchanged(t *testing.T) {
	t.Run("clean exit returns nil and does not connect", func(t *testing.T) {
		m := tui.New(asPickerLister(&mockSessionLister{}))
		connector := &mockSessionConnector{}
		if err := processTUIResult(m, connector); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if connector.connectedTo != "" {
			t.Errorf("connector called on clean exit: %q", connector.connectedTo)
		}
	})

	t.Run("selection still connects", func(t *testing.T) {
		m := tui.NewModelWithSessions([]tui.Session{{Name: "dev"}})
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
