package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

func rowNames(items []list.Item) []string {
	var names []string
	for _, si := range sessionRows(items) {
		names = append(names, si.Session.Name)
	}
	return names
}

func TestFilterMatchesLabels(t *testing.T) {
	sessions := []Session{
		{Name: "api-x7Kd9a", Label: "checkout flow"},
		{Name: "web-Qm3pLz"},
	}
	for _, tc := range []struct{ query, want string }{
		{"checkout", "api-x7Kd9a"},
		{"x7Kd9a", "api-x7Kd9a"},
		{"Qm3pLz", "web-Qm3pLz"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			m := newRebuildTestModel(t, prefs.ModeFlat, sessions, nil)
			m.rebuildSessionList()

			m.sessionList.SetFilterText(tc.query)
			m.sessionList.SetFilterState(list.FilterApplied)

			if got := rowNames(m.sessionList.VisibleItems()); !slices.Equal(got, []string{tc.want}) {
				t.Errorf("filter %q left %v, want [%s]", tc.query, got, tc.want)
			}
		})
	}
}

func TestSearchMatchesLabels(t *testing.T) {
	sessions := []Session{
		{Name: "api-x7Kd9a", Label: "checkout flow"},
		{Name: "web-Qm3pLz", Label: "landing page"},
		{Name: "checkout-Hq7wNe"},
	}

	m := searchColumnModel(t, prefs.ModeFlat, "checkout", sessions, nil)

	if got, want := rowNames(m.sessionList.VisibleItems()), []string{"api-x7Kd9a", "checkout-Hq7wNe"}; !slices.Equal(got, want) {
		t.Errorf("search narrowed to %v, want %v", got, want)
	}
}

func TestRowsOrderByDisplayName(t *testing.T) {
	dir := "/home/user/code/portal"
	projects := []project.Project{{Path: dir, Name: "portal", Tags: []string{"work"}}}
	// zeta's label reads first; alpha and beta show the same text, so their
	// names settle their order; the catch-all sorts the same way.
	sessions := []Session{
		{Name: "alpha", Label: "same", Dir: dir},
		{Name: "zeta", Label: "aardvark", Dir: dir},
		{Name: "beta", Label: "same", Dir: dir},
		{Name: "mid", Dir: dir},
		{Name: "loose-b", Label: "a loose one"},
		{Name: "loose-a"},
	}
	inGroup := []string{"zeta", "mid", "alpha", "beta"}
	catchAll := []string{"loose-b", "loose-a"}

	for _, tc := range []struct {
		mode prefs.SessionListMode
		want []string
	}{
		{prefs.ModeFlat, []string{"loose-b", "zeta", "loose-a", "mid", "alpha", "beta"}},
		{prefs.ModeByProject, slices.Concat(inGroup, catchAll)},
		{prefs.ModeByTag, slices.Concat(inGroup, catchAll)},
	} {
		t.Run(tc.mode.String(), func(t *testing.T) {
			m := newRebuildTestModel(t, tc.mode, sessions, projects)
			m.rebuildSessionList()

			if got := rowNames(m.sessionList.Items()); !slices.Equal(got, tc.want) {
				t.Errorf("rows = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("the signpost's flat list too", func(t *testing.T) {
		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, []project.Project{{Path: dir, Name: "portal"}})
		m.rebuildSessionList()

		if !m.byTagSignpost {
			t.Fatal("precondition: no tags anywhere must raise the signpost")
		}
		if got, want := rowNames(m.sessionList.Items()), []string{"loose-b", "zeta", "loose-a", "mid", "alpha", "beta"}; !slices.Equal(got, want) {
			t.Errorf("rows = %v, want %v", got, want)
		}
	})
}

func TestCurrentSessionTitleShowsItsLabel(t *testing.T) {
	sessions := []Session{
		{Name: "api-x7Kd9a", Label: "checkout flow"},
		{Name: "web-Qm3pLz"},
	}
	m := Build(Deps{Lister: fakeLister{}, CurrentSession: "api-x7Kd9a"})
	m.termWidth, m.termHeight = 100, 24
	m.applySessionListSize(m.contentWidth(), m.contentHeight())

	if got := m.sessionList.Title; !strings.Contains(got, "(current: api-x7Kd9a)") {
		t.Errorf("title before the listing = %q, want the current session by its name", got)
	}

	m.applySessions(sessions, nil)

	if got := m.sessionList.Title; !strings.Contains(got, "(current: checkout flow)") {
		t.Errorf("title = %q, want the current session by its label", got)
	}
	if got := ansi.Strip(m.viewSessionList()); !strings.Contains(got, "current: checkout flow") {
		t.Errorf("section header does not name the current session by its label:\n%s", got)
	}
	if got := rowNames(m.sessionList.Items()); !slices.Equal(got, []string{"web-Qm3pLz"}) {
		t.Errorf("rows = %v, want the current session left out", got)
	}
}

func TestNoPendingOrSkeletonReaders(t *testing.T) {
	m := Build(Deps{Lister: fakeLister{}})

	if cmd := m.startPendingSettle(); cmd != nil {
		t.Error("with no readers the settle loop started")
	}
	if pending := m.readPendingSessions(); pending != nil {
		t.Errorf("pending = %v, want none with no reader", pending)
	}
	m.applySessions([]Session{{Name: "api-x7Kd9a"}}, m.readPendingSessions())
	if got := rowNames(m.sessionList.Items()); !slices.Equal(got, []string{"api-x7Kd9a"}) {
		t.Errorf("rows = %v, want the session listed", got)
	}
}
