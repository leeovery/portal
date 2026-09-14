package tui_test

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui"
)

// stagedMintProjects is the two-project list the staging fixtures pick from;
// the second is what a re-pick would re-target the staged mint to.
var stagedMintProjects = []project.Project{
	{Name: "alpha", Path: "/tmp/alpha"},
	{Name: "beta", Path: "/tmp/beta"},
}

// coldCommandPendingModel is what a cold `portal open -- <command>` builds: the
// Projects page painted from frame one with the orchestrator still running.
func coldCommandPendingModel(t *testing.T, creator *mockSessionCreator) tea.Model {
	t.Helper()
	return commandPendingModel(t, creator, tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} }))
}

func commandPendingModel(t *testing.T, creator *mockSessionCreator, receiver tea.Cmd) tea.Model {
	t.Helper()
	deps := commandPendingDeps(receiver)
	deps.Creator = creator
	deps.CWD = "/tmp/cwd"

	var model tea.Model = tui.Build(deps)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: stagedMintProjects})
	if page := model.(tui.Model).ActivePage(); page != tui.PageProjects {
		t.Fatalf("ActivePage() = %d, want PageProjects", page)
	}
	return model
}

// runCmd executes cmd and returns its message, or nil for a nil cmd.
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestCommandPendingStagedMint(t *testing.T) {
	t.Run("it stages the pick instead of minting while the concurrent bootstrap is still running", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "alpha-abc123"}
		model := coldCommandPendingModel(t, creator)

		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		if msg := runCmd(cmd); msg != nil {
			t.Errorf("Enter during a live bootstrap produced %T; want nothing — no mint, no quit", msg)
		}
		if creator.createdDir != "" {
			t.Errorf("minted a session in %q against a half-bootstrapped server", creator.createdDir)
		}
		if got := model.(tui.Model).Selected(); got != "" {
			t.Errorf("Selected() = %q, want none", got)
		}
	})

	t.Run("it mints the staged pick when the bootstrap's complete event arrives", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "alpha-abc123"}
		model := coldCommandPendingModel(t, creator)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		warnings := []tui.BootstrapWarning{{Lines: []string{"Portal's session saver is not running."}}}
		model, cmd := model.Update(tui.BootstrapCompleteMsg{Warnings: warnings})

		// The teardown writes these before the mint's connect takes the process,
		// so the staged-mint return must sit below the append that stages them.
		if got := model.(tui.Model).PendingBootstrapWarnings(); len(got) != 1 {
			t.Errorf("PendingBootstrapWarnings() = %#v, want the one warning the teardown owes", got)
		}

		msg, ok := runCmd(cmd).(tui.SessionCreatedMsg)
		if !ok {
			t.Fatalf("complete produced %T, want the staged mint's SessionCreatedMsg", runCmd(cmd))
		}
		if msg.SessionName != "alpha-abc123" {
			t.Errorf("SessionName = %q, want the minted session", msg.SessionName)
		}
		if creator.createdDir != stagedMintProjects[0].Path {
			t.Errorf("minted in %q, want the staged %q", creator.createdDir, stagedMintProjects[0].Path)
		}
		if len(creator.createdCommand) == 0 {
			t.Error("minted with no command; the staged mint must forward the pending command")
		}
	})

	t.Run("it stages n's cwd mint on the same route", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "cwd-abc123"}
		model := coldCommandPendingModel(t, creator)

		model, cmd := model.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		if msg := runCmd(cmd); msg != nil {
			t.Errorf("n during a live bootstrap produced %T; want nothing", msg)
		}
		if creator.createdDir != "" {
			t.Fatalf("minted a session in %q against a half-bootstrapped server", creator.createdDir)
		}

		_, cmd = model.Update(tui.BootstrapCompleteMsg{})
		if _, ok := runCmd(cmd).(tui.SessionCreatedMsg); !ok {
			t.Fatal("complete did not issue the staged cwd mint")
		}
		if creator.createdDir != "/tmp/cwd" {
			t.Errorf("minted in %q, want the staged cwd", creator.createdDir)
		}
	})

	t.Run("it keeps the first pick when a second is made while the mint is staged", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "alpha-abc123"}
		model := coldCommandPendingModel(t, creator)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if msg := runCmd(cmd); msg != nil {
			t.Errorf("the second pick produced %T; want nothing", msg)
		}

		_, cmd = model.Update(tui.BootstrapCompleteMsg{})
		if _, ok := runCmd(cmd).(tui.SessionCreatedMsg); !ok {
			t.Fatal("complete did not issue the staged mint")
		}
		if creator.createdDir != stagedMintProjects[0].Path {
			t.Errorf("minted in %q, want the first pick %q — a silent re-target is worse than a keypress that changes nothing", creator.createdDir, stagedMintProjects[0].Path)
		}
	})

	t.Run("it mints immediately on the warm command-pending route", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "alpha-abc123"}
		model := commandPendingModel(t, creator, nil)

		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		if _, ok := runCmd(cmd).(tui.SessionCreatedMsg); !ok {
			t.Fatalf("warm Enter produced %T, want an immediate mint", runCmd(cmd))
		}
		if creator.createdDir != stagedMintProjects[0].Path {
			t.Errorf("minted in %q, want %q", creator.createdDir, stagedMintProjects[0].Path)
		}
	})

	t.Run("it mints nothing when the bootstrap ends in a fatal, on the fatal itself and on a complete after it", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "alpha-abc123"}
		model := coldCommandPendingModel(t, creator)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		model, cmd := model.Update(tui.BootstrapFatalMsg{FailedStep: 8, Message: "bootstrap failed", Err: errors.New("boom")})
		if _, ok := runCmd(cmd).(tea.QuitMsg); !ok {
			t.Errorf("fatal produced %T, want tea.QuitMsg and no mint", runCmd(cmd))
		}

		model, cmd = model.Update(tui.BootstrapCompleteMsg{})
		if msg := runCmd(cmd); msg != nil {
			t.Errorf("a complete after a fatal produced %T; want nothing", msg)
		}
		if creator.createdDir != "" {
			t.Errorf("minted a session in %q after a fatal", creator.createdDir)
		}
		if got := model.(tui.Model).Selected(); got != "" {
			t.Errorf("Selected() = %q, want none", got)
		}
	})

	t.Run("it still cancels on Esc while a mint is staged", func(t *testing.T) {
		creator := &mockSessionCreator{sessionName: "alpha-abc123"}
		model := coldCommandPendingModel(t, creator)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

		if _, ok := runCmd(cmd).(tea.QuitMsg); !ok {
			t.Errorf("Esc while staged produced %T, want tea.QuitMsg — the picker is never a dead end", runCmd(cmd))
		}
		if creator.createdDir != "" {
			t.Errorf("minted a session in %q on the way out", creator.createdDir)
		}
	})
}
