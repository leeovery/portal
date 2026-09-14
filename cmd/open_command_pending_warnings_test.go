package cmd

import (
	"bytes"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/warning"
)

// coldCommandPendingModel is the model `portal open -- <command>` builds on a
// cold or version-unlatched server: a picker with a command to run alongside a
// bootstrap still running behind it.
func coldCommandPendingModel(t *testing.T, terminal tea.Msg) tui.Model {
	t.Helper()

	receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
	var model tea.Model = tui.Build(tui.Deps{
		Lister:           &mockSessionLister{sessions: []tmux.Session{{Name: "portal-a1b2"}}},
		Command:          []string{"npm", "run", "dev"},
		ServerStarted:    true,
		ProgressReceiver: receiver,
	})
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(terminal)

	m, ok := model.(tui.Model)
	if !ok {
		t.Fatalf("model type = %T, want tui.Model", model)
	}
	return m
}

// refusingConnector fails the test if the teardown ever reaches the attach.
type refusingConnector struct{ t *testing.T }

func (c *refusingConnector) Connect(name string) error {
	c.t.Helper()
	c.t.Errorf("connected to %q; a bootstrap fatal must reach no session", name)
	return nil
}

func TestFinishTUI_ColdCommandPendingRoute(t *testing.T) {
	t.Run("it writes a down-daemon warning once at teardown on the cold command-pending route", func(t *testing.T) {
		warnings := soakedWarnings()
		model := coldCommandPendingModel(t, tui.BootstrapCompleteMsg{Warnings: warnings})

		var canvas, stderr bytes.Buffer
		if err := finishTUI(model, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if want := wantWarningOutput(warnings); stderr.String() != want {
			t.Errorf("stderr = %q, want %q exactly once", stderr.String(), want)
		}
	})

	t.Run("it writes the same lines as the CLI path", func(t *testing.T) {
		warnings := soakedWarnings()
		accumulateWarnings(t, warnings)
		var cli bytes.Buffer
		bootstrapWarnings.EmitTo(&cli)

		model := coldCommandPendingModel(t, tui.BootstrapCompleteMsg{Warnings: warnings})

		var canvas, stderr bytes.Buffer
		if err := finishTUI(model, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if stderr.String() != cli.String() {
			t.Errorf("teardown stderr = %q, want the CLI path's %q", stderr.String(), cli.String())
		}
	})

	t.Run("it writes nothing at teardown when the bootstrap warned about nothing", func(t *testing.T) {
		model := coldCommandPendingModel(t, tui.BootstrapCompleteMsg{Warnings: []warning.Warning{}})

		var canvas, stderr bytes.Buffer
		if err := finishTUI(model, &observingConnector{onConnect: func() {}}, &canvas, &stderr); err != nil {
			t.Fatalf("finishTUI: %v", err)
		}

		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want nothing", stderr.String())
		}
	})

	t.Run("it reports a non-nil FatalError at teardown for a command-pending fatal", func(t *testing.T) {
		fatal := errors.New("clear @portal-restoring: no server")
		model := coldCommandPendingModel(t, tui.BootstrapFatalMsg{FailedStep: 8, Message: "bootstrap failed", Err: fatal})

		if model.FatalError() == nil {
			t.Fatal("FatalError() is nil; the process would exit 0 on a failed bootstrap")
		}

		var canvas, stderr bytes.Buffer
		err := finishTUI(model, &refusingConnector{t: t}, &canvas, &stderr)
		if !errors.Is(err, fatal) {
			t.Errorf("finishTUI err = %v, want the orchestrator's %v", err, fatal)
		}
	})
}
