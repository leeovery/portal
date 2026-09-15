package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

func ingestLanding(t *testing.T, m Model, sessions []tmux.Session, projects []project.Project) Model {
	t.Helper()
	var model tea.Model = m
	model, _ = model.Update(SessionsMsg{Sessions: sessions})
	model, _ = model.Update(ProjectsLoadedMsg{Projects: projects})
	return model.(Model)
}

func searchLanding(t *testing.T, mode prefs.SessionListMode, term string, sessions []tmux.Session, projects []project.Project) Model {
	t.Helper()
	m := Build(Deps{
		Lister:       fakeLister{},
		ProjectStore: stubProjectStore{},
		InitialMode:  mode,
		Search:       &SearchForm{Term: term},
	})
	return ingestLanding(t, m, sessions, projects)
}

func sessionNames(items []list.Item) []string {
	names := make([]string, 0, len(items))
	for _, it := range items {
		if si, ok := it.(SessionItem); ok {
			names = append(names, si.Session.Name)
		}
	}
	return names
}

func TestSearchFormLanding(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "myapp-dev"},
		{Name: "other"},
		{Name: "myapp-prod"},
	}

	t.Run("it lands a search term on the Sessions page with the filter committed", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "myapp", sessions, nil)

		if m.activePage != PageSessions {
			t.Errorf("activePage = %v, want PageSessions", m.activePage)
		}
		if got := m.sessionList.FilterState(); got != list.FilterApplied {
			t.Errorf("session filter state = %v, want FilterApplied", got)
		}
		if got := m.sessionList.FilterValue(); got != "myapp" {
			t.Errorf("session filter value = %q, want %q", got, "myapp")
		}
		got := sessionNames(m.sessionList.VisibleItems())
		if len(got) != 2 {
			t.Fatalf("visible sessions = %v, want the two myapp sessions", got)
		}
		for _, name := range got {
			if name == "other" {
				t.Errorf("visible sessions = %v, want %q filtered out", got, "other")
			}
		}
	})

	t.Run("it puts the cursor on the first surviving session row", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "myapp", sessions, nil)

		si, ok := m.sessionList.SelectedItem().(SessionItem)
		if !ok {
			t.Fatalf("selected item = %T, want SessionItem", m.sessionList.SelectedItem())
		}
		if want := sessionNames(m.sessionList.VisibleItems())[0]; si.Session.Name != want {
			t.Errorf("selected session = %q, want %q (first surviving row)", si.Session.Name, want)
		}
	})

	for _, mode := range []struct {
		label string
		mode  prefs.SessionListMode
	}{
		{"By Project", prefs.ModeByProject},
		{"By Tag", prefs.ModeByTag},
	} {
		t.Run("it puts the cursor on a session row rather than a group header in "+mode.label, func(t *testing.T) {
			dirA, dirB := t.TempDir(), t.TempDir()
			projects := []project.Project{
				{Path: dirA, Name: "Alpha", Tags: []string{"work"}},
				{Path: dirB, Name: "Bravo", Tags: []string{"play"}},
			}
			grouped := []tmux.Session{
				{Name: "myapp-alpha", Dir: dirA},
				{Name: "other", Dir: dirB},
				{Name: "myapp-bravo", Dir: dirB},
			}

			m := searchLanding(t, mode.mode, "myapp", grouped, projects)

			if m.activePage != PageSessions {
				t.Errorf("activePage = %v, want PageSessions", m.activePage)
			}
			if got := m.sessionList.FilterValue(); got != "myapp" {
				t.Errorf("session filter value = %q, want %q", got, "myapp")
			}
			for _, name := range sessionNames(m.sessionList.VisibleItems()) {
				if name == "other" {
					t.Errorf("visible sessions = %v, want %q filtered out",
						sessionNames(m.sessionList.VisibleItems()), "other")
				}
			}
			if _, isHeader := m.sessionList.SelectedItem().(HeaderItem); isHeader {
				t.Fatalf("selection rests on a group header, want a session row")
			}
			si, ok := m.sessionList.SelectedItem().(SessionItem)
			if !ok {
				t.Fatalf("selected item = %T, want SessionItem", m.sessionList.SelectedItem())
			}
			if want := sessionNames(m.sessionList.VisibleItems())[0]; si.Session.Name != want {
				t.Errorf("selected session = %q, want %q (first surviving row)", si.Session.Name, want)
			}
		})
	}

	t.Run("it pins the Sessions page when no sessions are live", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "myapp", nil, nil)

		if m.activePage != PageSessions {
			t.Errorf("activePage = %v, want PageSessions", m.activePage)
		}
		if got := m.sessionList.FilterValue(); got != "myapp" {
			t.Errorf("session filter value = %q, want %q", got, "myapp")
		}
		if got := m.projectList.FilterValue(); got != "" {
			t.Errorf("project filter value = %q, want empty", got)
		}
	})

	t.Run("it pins the Sessions page when nothing survives the term", func(t *testing.T) {
		m := searchLanding(t, prefs.ModeFlat, "zzqx", sessions, nil)

		if m.activePage != PageSessions {
			t.Errorf("activePage = %v, want PageSessions", m.activePage)
		}
		if !m.sessionListNoMatches() {
			t.Errorf("sessionListNoMatches() = false, want the no-matches state (state %v, %d visible)",
				m.sessionList.FilterState(), len(m.sessionList.VisibleItems()))
		}
		if got := m.projectList.FilterValue(); got != "" {
			t.Errorf("project filter value = %q, want empty", got)
		}
	})

	t.Run("it leaves the -f landing unchanged", func(t *testing.T) {
		m := Build(Deps{
			Lister:        fakeLister{},
			ProjectStore:  stubProjectStore{},
			InitialMode:   prefs.ModeFlat,
			InitialFilter: "myapp",
		})
		m = ingestLanding(t, m, sessions, nil)

		if m.activePage != PageSessions {
			t.Errorf("activePage = %v, want PageSessions", m.activePage)
		}
		if got := m.sessionList.FilterState(); got != list.FilterApplied {
			t.Errorf("session filter state = %v, want FilterApplied", got)
		}
		if got := m.sessionList.FilterValue(); got != "myapp" {
			t.Errorf("session filter value = %q, want %q", got, "myapp")
		}
		if got := m.initialFilter; got != "" {
			t.Errorf("initialFilter = %q, want it consumed", got)
		}
		if got := m.projectList.FilterValue(); got != "" {
			t.Errorf("project filter value = %q, want empty", got)
		}
	})

	t.Run("it leaves the command-pending Projects redirect unchanged", func(t *testing.T) {
		projects := []project.Project{
			{Path: t.TempDir(), Name: "myapp"},
			{Path: t.TempDir(), Name: "other"},
		}
		m := Build(Deps{
			Lister:        fakeLister{},
			ProjectStore:  stubProjectStore{},
			InitialMode:   prefs.ModeFlat,
			InitialFilter: "myapp",
			Command:       []string{"claude"},
		})
		m = ingestLanding(t, m, sessions, projects)

		if m.activePage != PageProjects {
			t.Errorf("activePage = %v, want PageProjects", m.activePage)
		}
		if got := m.projectList.FilterState(); got != list.FilterApplied {
			t.Errorf("project filter state = %v, want FilterApplied", got)
		}
		if got := m.projectList.FilterValue(); got != "myapp" {
			t.Errorf("project filter value = %q, want %q", got, "myapp")
		}
		if got := m.sessionList.FilterValue(); got != "" {
			t.Errorf("session filter value = %q, want empty", got)
		}
	})
}

func TestSearchFormPrecedenceOverInitialFilter(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "myapp-dev"},
		{Name: "other"},
	}

	t.Run("it lands on the search term when a Deps carries both a search form and an initial filter", func(t *testing.T) {
		m := Build(Deps{
			Lister:        fakeLister{},
			ProjectStore:  stubProjectStore{},
			InitialMode:   prefs.ModeFlat,
			Search:        &SearchForm{Term: "myapp"},
			InitialFilter: "other",
		})

		if got := m.initialFilter; got != "" {
			t.Errorf("initialFilter = %q, want the search form to have kept it off the model", got)
		}

		m = ingestLanding(t, m, sessions, nil)

		if m.activePage != PageSessions {
			t.Errorf("activePage = %v, want PageSessions", m.activePage)
		}
		if got := m.sessionList.FilterState(); got != list.FilterApplied {
			t.Errorf("session filter state = %v, want FilterApplied", got)
		}
		if got := m.sessionList.FilterValue(); got != "myapp" {
			t.Errorf("session filter value = %q, want the search term %q", got, "myapp")
		}
	})
}
