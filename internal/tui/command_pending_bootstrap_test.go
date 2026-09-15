package tui_test

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// commandPendingDeps is what `portal open -- <command>` builds: a command to
// run, and on the cold route a receiver for the concurrent orchestrator.
func commandPendingDeps(receiver tea.Cmd) tui.Deps {
	return tui.Deps{
		Lister:           &mockSessionLister{sessions: []tmux.Session{}},
		Command:          []string{"npm", "run", "dev"},
		ServerStarted:    receiver != nil,
		ProgressReceiver: receiver,
	}
}

// initMessages runs every command Init batched and returns what they produced,
// skipping the ones that do not answer promptly (the appearance-gate timeout).
func initMessages(t *testing.T, m tui.Model) []tea.Msg {
	t.Helper()

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil")
	}
	// tea.Batch collapses to the single cmd when only one survives, so a
	// non-batch reply is one message rather than a failure.
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}

	var msgs []tea.Msg
	for _, c := range batch {
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func(cmd tea.Cmd) { done <- cmd() }(c)
		select {
		case got := <-done:
			msgs = append(msgs, got)
		case <-time.After(100 * time.Millisecond):
		}
	}
	return msgs
}

func TestCommandPendingBootstrapDelivery(t *testing.T) {
	t.Run("it issues the progress receiver for a command-pending model on the cold route", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 3} })
		m := tui.Build(commandPendingDeps(receiver))

		var found bool
		for _, msg := range initMessages(t, m) {
			if pm, ok := msg.(tui.BootstrapProgressMsg); ok && pm.Index == 3 {
				found = true
			}
		}
		if !found {
			t.Error("Init() did not issue the progress receiver; the orchestrator's events never reach Update")
		}
	})

	t.Run("it issues no bootstrap message for a command-pending model on the warm route", func(t *testing.T) {
		m := tui.Build(commandPendingDeps(nil))

		for _, msg := range initMessages(t, m) {
			switch msg.(type) {
			case tui.BootstrapProgressMsg, tui.BootstrapCompleteMsg, tui.BootstrapFatalMsg:
				t.Errorf("warm command-pending Init() produced %T; the teardown already owes the warnings", msg)
			}
		}
	})

	t.Run("it records a bootstrap fatal delivered to a command-pending model", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
		fatal := errors.New("clear @portal-restoring: no server")

		var model tea.Model = tui.Build(commandPendingDeps(receiver))
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, cmd := model.Update(tui.BootstrapFatalMsg{FailedStep: 8, Message: "bootstrap failed", Err: fatal})

		updated := model.(tui.Model)
		if !errors.Is(updated.FatalError(), fatal) {
			t.Errorf("FatalError() = %v, want the orchestrator's %v", updated.FatalError(), fatal)
		}
		if cmd == nil {
			t.Fatal("fatal on a command-pending model returned no cmd; want tea.Quit")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("fatal cmd produced %T, want tea.QuitMsg — the picker cannot act on a half-bootstrapped server", cmd())
		}
	})

	t.Run("it does not run the pending command when a bootstrap fatal is active", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
		creator := &mockSessionCreator{sessionName: "myapp-abc123"}
		deps := commandPendingDeps(receiver)
		deps.Creator = creator

		var model tea.Model = tui.Build(deps)
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: []project.Project{{Name: "myapp", Path: "/tmp/myapp"}}})
		if page := model.(tui.Model).ActivePage(); page != tui.PageProjects {
			t.Fatalf("ActivePage() = %d, want PageProjects", page)
		}
		model, _ = model.Update(tui.BootstrapFatalMsg{FailedStep: 8, Message: "bootstrap failed", Err: errors.New("boom")})

		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd != nil {
			cmd()
		}

		if creator.createdDir != "" {
			t.Errorf("minted a session in %q against a half-bootstrapped server", creator.createdDir)
		}
		if got := model.(tui.Model).Selected(); got != "" {
			t.Errorf("Selected() = %q, want none", got)
		}
	})

	t.Run("it stages the complete message's warnings for the teardown on the command-pending route", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
		warnings := []tui.BootstrapWarning{{Lines: []string{"Portal's session saver is not running."}}}

		var model tea.Model = tui.Build(commandPendingDeps(receiver))
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})

		updated := model.(tui.Model)
		if got := updated.PendingBootstrapWarnings(); len(got) != 1 {
			t.Errorf("PendingBootstrapWarnings() = %#v, want the one warning the teardown owes", got)
		}
		if got := updated.BufferedWarnings(); len(got) != 0 {
			t.Errorf("BufferedWarnings() = %#v, want none — there is no notice band to reach", got)
		}
	})

	t.Run("it leaves a command-pending model appending the message's warnings onto the staged set", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })

		m := tui.Build(commandPendingDeps(receiver))
		m.SetPendingBootstrapWarnings([]tui.BootstrapWarning{{Lines: []string{"staged"}}})
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.BootstrapCompleteMsg{
			Warnings: []tui.BootstrapWarning{{Lines: []string{"orchestrator"}}},
		})

		got := model.(tui.Model).PendingBootstrapWarnings()
		if len(got) != 2 {
			t.Fatalf("PendingBootstrapWarnings() = %#v, want both the staged warning and the message's own", got)
		}
		if got[0].Lines[0] != "staged" || got[1].Lines[0] != "orchestrator" {
			t.Errorf("PendingBootstrapWarnings() = %#v, want the staged warning first", got)
		}
	})

	t.Run("it leaves the loading page's complete-message handling unchanged", func(t *testing.T) {
		receiver := tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} })
		warnings := []tui.BootstrapWarning{{Lines: []string{"Portal's session saver is not running."}}}

		m := tui.Build(tui.Deps{
			Lister:           &mockSessionLister{sessions: []tmux.Session{}},
			ServerStarted:    true,
			ProgressReceiver: receiver,
		})
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})

		updated := model.(tui.Model)
		if updated.ActivePage() == tui.PageLoading {
			t.Error("the loading page did not dismiss on the terminal event")
		}
		if got := updated.BufferedWarnings(); len(got) != 0 {
			t.Errorf("BufferedWarnings() = %#v, want none — the dismissal surfaced them", got)
		}
		if got := updated.PendingBootstrapWarnings(); len(got) != 0 {
			t.Errorf("PendingBootstrapWarnings() = %#v, want none — the gate owned them", got)
		}
	})
}
