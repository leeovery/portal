package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

func newProjectsTransitionModel(t *testing.T, lister SessionLister, projects []project.Project, mode prefs.SessionListMode) Model {
	t.Helper()
	m := Model{
		themeState:      themeState{active: testDarkTheme(t)},
		projects:        projects,
		projectIndex:    project.NewIndex(projects),
		projectList:     newProjectList(),
		sessionList:     newSessionList(nil),
		activePage:      PageProjects,
		sessionListMode: mode,
		sessionLister:   lister,
	}
	m.projectList.SetItems(ProjectsToListItems(projects))
	return m
}

func pressProjectsKey(t *testing.T, m Model, r rune) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(keyRune(r))
	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after %q keypress, got %T", string(r), updated)
	}
	return got, cmd
}

func TestProjectsTransitionDispatchesRefresh(t *testing.T) {
	for _, key := range []rune{'x'} {
		t.Run(string(key), func(t *testing.T) {
			lister := &stepListerStub{steps: [][]Session{{
				{Name: "alpha", Answered: true, Attached: false},
			}}}
			m := newProjectsTransitionModel(t, lister, nil, prefs.ModeFlat)

			got, cmd := pressProjectsKey(t, m, key)

			if got.activePage != PageSessions {
				t.Fatalf("expected PageSessions after %q, got %v", string(key), got.activePage)
			}
			if cmd == nil {
				t.Fatalf("expected a non-nil refresh cmd on %q transition, got nil", string(key))
			}
			msg := cmd()
			if _, ok := msg.(previewSessionsRefreshedMsg); !ok {
				t.Fatalf("expected refresh cmd to yield previewSessionsRefreshedMsg, got %T", msg)
			}
			if lister.calls != 1 {
				t.Errorf("expected exactly 1 ListSessions call from the refresh, got %d", lister.calls)
			}
		})
	}
}

func TestProjectsTransitionRegroupsWithUpdatedTags(t *testing.T) {
	projects := []project.Project{
		{Path: "/p/one", Name: "one", Tags: []string{"work"}},
	}
	sessions := []Session{
		{Name: "alpha", Answered: true, Attached: false, Dir: "/p/one"},
	}
	lister := &stepListerStub{steps: [][]Session{sessions}}
	m := newProjectsTransitionModel(t, lister, projects, prefs.ModeByTag)

	got, cmd := pressProjectsKey(t, m, 'x')
	if cmd == nil {
		t.Fatalf("expected a non-nil refresh cmd, got nil")
	}

	updated, refilter := got.Update(cmd())
	got2, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model after refresh msg, got %T", updated)
	}
	final, ok := drainCmdThroughUpdate(t, got2, refilter).(Model)
	if !ok {
		t.Fatalf("expected Model after refilter drain, got %T", final)
	}

	items := final.sessionList.VisibleItems()
	rows := sessionRows(items)
	if len(rows) != 1 {
		t.Fatalf("expected 1 visible session row after re-group, got %d (items=%v)", len(rows), items)
	}
	si := rows[0]
	if si.GroupHeading != "work" || si.GroupKey != "work" {
		t.Errorf("expected session re-grouped under tag heading %q, got heading=%q key=%q", "work", si.GroupHeading, si.GroupKey)
	}
	headers := headerRows(items)
	if len(headers) != 1 || headers[0].Heading != "work" {
		t.Errorf("expected a single 'work' header, got %v", headers)
	}
}

func TestProjectsTransitionToleratesNilLister(t *testing.T) {
	for _, key := range []rune{'x'} {
		t.Run(string(key), func(t *testing.T) {
			m := newProjectsTransitionModel(t, nil, nil, prefs.ModeFlat)

			got, cmd := pressProjectsKey(t, m, key)

			if got.activePage != PageSessions {
				t.Errorf("expected PageSessions even with nil lister on %q, got %v", string(key), got.activePage)
			}
			if cmd != nil {
				t.Errorf("expected nil refresh cmd when no SessionLister wired on %q, got non-nil", string(key))
			}
		})
	}
}

func TestProjectsTransitionPreservesCommandPendingGuard(t *testing.T) {
	for _, key := range []rune{'s', 'x'} {
		t.Run(string(key), func(t *testing.T) {
			lister := &stepListerStub{steps: [][]Session{{
				{Name: "alpha", Answered: true, Attached: false},
			}}}
			m := newProjectsTransitionModel(t, lister, nil, prefs.ModeFlat)
			m.commandPending = true

			got, cmd := pressProjectsKey(t, m, key)

			if got.activePage != PageProjects {
				t.Errorf("expected to stay on PageProjects in command-pending mode on %q, got %v", string(key), got.activePage)
			}
			if cmd != nil {
				t.Errorf("expected no refresh cmd in command-pending mode on %q, got non-nil", string(key))
			}
			if lister.calls != 0 {
				t.Errorf("expected no ListSessions call in command-pending mode on %q, got %d", string(key), lister.calls)
			}
		})
	}
}

func TestProjectsNonTransitionKeyDoesNotRefresh(t *testing.T) {
	lister := &stepListerStub{steps: [][]Session{{
		{Name: "alpha", Answered: true, Attached: false},
	}}}
	m := newProjectsTransitionModel(t, lister, nil, prefs.ModeFlat)

	got, cmd := pressProjectsKey(t, m, '?')

	if got.activePage != PageProjects {
		t.Errorf("expected to stay on PageProjects, got %v", got.activePage)
	}
	if cmd != nil {
		t.Errorf("expected no refresh cmd on a non-transition key, got non-nil")
	}
	if lister.calls != 0 {
		t.Errorf("expected no ListSessions call on a non-transition key, got %d", lister.calls)
	}
}
