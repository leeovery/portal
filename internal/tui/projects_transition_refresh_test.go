package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// newProjectsTransitionModel builds a Model parked on the projects page with a
// wired SessionLister, so the projects-page s/x → sessions transition has a
// lister to dispatch its re-group refresh against. projects seeds the cached
// project records (consulted by the mode-aware re-render path) and mode sets
// the active grouping mode.
func newProjectsTransitionModel(lister SessionLister, projects []project.Project, mode prefs.SessionListMode) Model {
	m := Model{
		projects:        projects,
		projectList:     newProjectList(),
		sessionList:     newSessionList(nil),
		activePage:      PageProjects,
		sessionListMode: mode,
		sessionLister:   lister,
	}
	m.projectList.SetItems(ProjectsToListItems(projects))
	return m
}

// pressProjectsKey drives a single printable rune through the projects-page
// handler via the top-level Update (which routes to updateProjectsPage while
// activePage == PageProjects) and returns the resulting Model plus the
// dispatched tea.Cmd.
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
	for _, key := range []rune{'s', 'x'} {
		t.Run(string(key), func(t *testing.T) {
			lister := &stepListerStub{steps: [][]tmux.Session{{
				{Name: "alpha", Windows: 1, Attached: false},
			}}}
			m := newProjectsTransitionModel(lister, nil, prefs.ModeFlat)

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
	// By Tag mode active. The project carries a freshly-edited tag ("work"); the
	// live session's Dir resolves to that project. After the s-transition
	// refresh, the session must re-group under the "work" heading (mode-aware
	// re-render path), proving tags are re-resolved live on return.
	projects := []project.Project{
		{Path: "/p/one", Name: "one", Tags: []string{"work"}},
	}
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false, Dir: "/p/one"},
	}
	lister := &stepListerStub{steps: [][]tmux.Session{sessions}}
	m := newProjectsTransitionModel(lister, projects, prefs.ModeByTag)

	got, cmd := pressProjectsKey(t, m, 's')
	if cmd == nil {
		t.Fatalf("expected a non-nil refresh cmd, got nil")
	}

	// Round-trip the refresh message through Update so applySessions runs and
	// re-groups per the active ByTag mode.
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
	if len(items) != 1 {
		t.Fatalf("expected 1 visible session item after re-group, got %d", len(items))
	}
	si, ok := items[0].(SessionItem)
	if !ok {
		t.Fatalf("expected SessionItem, got %T", items[0])
	}
	if si.GroupHeading != "work" || si.Tag != "work" {
		t.Errorf("expected session re-grouped under tag heading %q, got heading=%q tag=%q", "work", si.GroupHeading, si.Tag)
	}
}

func TestProjectsTransitionToleratesNilLister(t *testing.T) {
	for _, key := range []rune{'s', 'x'} {
		t.Run(string(key), func(t *testing.T) {
			m := newProjectsTransitionModel(nil, nil, prefs.ModeFlat)

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
			lister := &stepListerStub{steps: [][]tmux.Session{{
				{Name: "alpha", Windows: 1, Attached: false},
			}}}
			m := newProjectsTransitionModel(lister, nil, prefs.ModeFlat)
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
	// A projects-page key that does NOT transition to the sessions page must not
	// dispatch the re-group refresh. "?" is swallowed by updateProjectsPage and
	// keeps the page on PageProjects.
	lister := &stepListerStub{steps: [][]tmux.Session{{
		{Name: "alpha", Windows: 1, Attached: false},
	}}}
	m := newProjectsTransitionModel(lister, nil, prefs.ModeFlat)

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
