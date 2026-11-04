package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tui"
)

type stubKiller struct{}

func (stubKiller) KillSession(string) error { return nil }

type stubLabeller struct{}

func (stubLabeller) SetLabel(string, string) error { return nil }

type stubCreator struct{}

func (stubCreator) CreateFromDir(string, []string) (string, error) { return "", nil }

func TestBuild(t *testing.T) {
	t.Run("wires the seam set and applies the initial mode", func(t *testing.T) {
		lister := &mockSessionLister{sessions: []tui.Session{{Name: "dev"}}}

		m := tui.Build(tui.Deps{
			Lister:      lister,
			Killer:      stubKiller{},
			Labeller:    stubLabeller{},
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
		if got, want := m.SessionListTitle(), "Sessions — by tag"; got != want {
			t.Errorf("SessionListTitle() = %q, want %q", got, want)
		}
	})

	t.Run("server-started routes to the loading page", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister:        &mockSessionLister{},
			InitialMode:   prefs.ModeFlat,
			Bootstrapping: true,
		})

		if m.ActivePage() != tui.PageLoading {
			t.Errorf("ActivePage() = %d, want PageLoading", m.ActivePage())
		}
		if !m.Bootstrapping() {
			t.Error("Bootstrapping() = false, want true")
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

	t.Run("a current session name decorates the title", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister:         &mockSessionLister{},
			InitialMode:    prefs.ModeFlat,
			CurrentSession: "current",
		})

		if got, want := m.CurrentSession(), "current"; got != want {
			t.Errorf("CurrentSession() = %q, want %q", got, want)
		}
		if got, want := m.SessionListTitle(), "Sessions (current: current)"; got != want {
			t.Errorf("SessionListTitle() = %q, want %q", got, want)
		}
	})

	t.Run("no current session name leaves the title undecorated", func(t *testing.T) {
		m := tui.Build(tui.Deps{
			Lister:      &mockSessionLister{},
			InitialMode: prefs.ModeFlat,
		})

		if got := m.CurrentSession(); got != "" {
			t.Errorf("CurrentSession() = %q, want empty", got)
		}
		if got, want := m.SessionListTitle(), "Sessions"; got != want {
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
			Lister:      &mockSessionLister{},
			InitialMode: prefs.ModeFlat,
			Capture:     tui.CaptureSeeds{Detection: &id},
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
			Lister: &mockSessionLister{sessions: []tui.Session{
				{Name: "agentic-workflows-codify"},
				{Name: "fab-flowx-explore"},
				{Name: "designlab-web-r8suyU"},
			}},
			InitialMode: prefs.ModeFlat,
			Capture: tui.CaptureSeeds{
				MultiSelect: []string{"agentic-workflows-codify", "fab-flowx-explore", "designlab-web-r8suyU"},
				GoneFlagged: []string{"fab-flowx-explore"},
			},
		})

		if !m.MultiSelectActive() {
			t.Error("MultiSelectActive() = false, want true (survivors stay marked)")
		}

		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		model, _ = model.Update(tui.SessionsMsg{Sessions: []tui.Session{
			{Name: "agentic-workflows-codify"},
			{Name: "fab-flowx-explore"},
			{Name: "designlab-web-r8suyU"},
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
			Lister:      &mockSessionLister{},
			InitialMode: prefs.ModeFlat,
			Capture:     tui.CaptureSeeds{BurstOpening: [2]int{2, 3}},
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
