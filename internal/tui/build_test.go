package tui_test

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// stubKiller / stubRenamer / stubCreator are no-op seam implementations used
// only to populate tui.Deps in the Build tests below.
type stubKiller struct{}

func (stubKiller) KillSession(string) error { return nil }

type stubRenamer struct{}

func (stubRenamer) RenameSession(string, string) error { return nil }

type stubCreator struct{}

func (stubCreator) CreateFromDir(string, []string) (string, error) { return "", nil }

// TestBuild verifies the shared tui.Build constructor wires the seam set onto
// the model identically to the equivalent hand-written New(...) option list, so
// production (cmd/open.go) and the capture tool produce the same model.
func TestBuild(t *testing.T) {
	t.Run("wires the seam set and applies the initial mode", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tmux.Session{{Name: "dev", Windows: 2}}}

		m := tui.Build(tui.Deps{
			Lister:      lister,
			Killer:      stubKiller{},
			Renamer:     stubRenamer{},
			Creator:     stubCreator{},
			CWD:         "/home/user",
			InitialMode: prefs.ModeByTag,
		})

		if m.ActivePage() != tui.PageSessions {
			t.Errorf("ActivePage() = %d, want PageSessions", m.ActivePage())
		}
		if m.CWD() != "/home/user" {
			t.Errorf("CWD() = %q, want /home/user", m.CWD())
		}
		// InitialMode is reflected in the title once options apply.
		if got, want := m.SessionListTitle(), "Sessions — by tag"; got != want {
			t.Errorf("SessionListTitle() = %q, want %q", got, want)
		}
	})

	t.Run("server-started routes to the loading page", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister:        &mockSessionLister{},
			InitialMode:   prefs.ModeFlat,
			ServerStarted: true,
		})

		if m.ActivePage() != tui.PageLoading {
			t.Errorf("ActivePage() = %d, want PageLoading", m.ActivePage())
		}
		if !m.ServerStarted() {
			t.Error("ServerStarted() = false, want true")
		}
	})

	t.Run("command puts the model in command-pending mode", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister:      &mockSessionLister{},
			InitialMode: prefs.ModeFlat,
			Command:     []string{"claude"},
		})

		if !m.CommandPending() {
			t.Error("CommandPending() = false, want true")
		}
		if m.ActivePage() != tui.PageProjects {
			t.Errorf("ActivePage() = %d, want PageProjects", m.ActivePage())
		}
	})

	t.Run("inside-tmux excludes the current session and decorates the title", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister:         &mockSessionLister{},
			InitialMode:    prefs.ModeFlat,
			InsideTmux:     true,
			CurrentSession: "current",
		})

		if !m.InsideTmux() {
			t.Error("InsideTmux() = false, want true")
		}
		if got, want := m.SessionListTitle(), "Sessions (current: current)"; got != want {
			t.Errorf("SessionListTitle() = %q, want %q", got, want)
		}
	})

	t.Run("initial filter is threaded onto the model", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister:        &mockSessionLister{},
			InitialMode:   prefs.ModeFlat,
			InitialFilter: "myapp",
		})

		if got, want := m.InitialFilter(), "myapp"; got != want {
			t.Errorf("InitialFilter() = %q, want %q", got, want)
		}
	})
}
