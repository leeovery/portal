package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/spawn"
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

	t.Run("initial detection seeds the resolved unsupported cache", func(t *testing.T) {
		id := spawn.Identity{Name: "Apple Terminal", BundleID: "com.apple.Terminal"}
		m := tui.Build(tui.Deps{
			Lister:           &mockSessionLister{},
			InitialMode:      prefs.ModeFlat,
			InitialDetection: &id,
		})

		if !m.DetectResolved() {
			t.Fatal("DetectResolved() = false, want true")
		}
		if !m.DetectUnsupported() {
			t.Error("DetectUnsupported() = false, want true (non-NULL Apple Terminal resolves unsupported)")
		}
	})

	t.Run("initial gone-flagged seeds the abort banner over a multi-select model", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister: &mockSessionLister{sessions: []tmux.Session{
				{Name: "agentic-workflows-codify", Windows: 1},
				{Name: "fab-flowx-explore", Windows: 2},
				{Name: "designlab-web-r8suyU", Windows: 3},
			}},
			InitialMode:        prefs.ModeFlat,
			Appearance:         prefs.AppearanceDark,
			InitialMultiSelect: []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"},
			InitialGoneFlagged: []string{"fab-flowx-explore"},
		})

		if !m.MultiSelectActive() {
			t.Error("MultiSelectActive() = false, want true (survivors stay marked)")
		}

		// Size + ingest the sessions so the section-header row renders (a refresh does
		// NOT clear the abort banner — only an actionable key / Esc does).
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{
			{Name: "agentic-workflows-codify", Windows: 1},
			{Name: "fab-flowx-explore", Windows: 2},
			{Name: "designlab-web-r8suyU", Windows: 3},
		}})
		model, _ = model.Update(tui.ProjectsLoadedMsg{})

		visible := ansi.Strip(model.(tui.Model).View().Content)
		if !strings.Contains(visible, "'fab-flowx-explore' is gone — nothing opened") {
			t.Errorf("view missing the abort banner:\n%s", visible)
		}
		if !strings.Contains(visible, "session gone") {
			t.Errorf("view missing the gone-row badge:\n%s", visible)
		}
	})

	t.Run("initial burst opening seeds the pending Opening band", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister:              &mockSessionLister{},
			InitialMode:         prefs.ModeFlat,
			InitialBurstOpening: [2]int{2, 3},
		})

		if !m.BurstPending() {
			t.Fatal("BurstPending() = false, want true")
		}
		if got, want := m.BurstDone(), 2; got != want {
			t.Errorf("BurstDone() = %d, want %d", got, want)
		}
		if got, want := m.BurstTotal(), 3; got != want {
			t.Errorf("BurstTotal() = %d, want %d", got, want)
		}
	})
}
