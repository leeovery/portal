package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
)

// filterTerm matches four of filterTestSessions and two of
// filterTestProjects: few enough to fit a page the whole list outgrows.
const filterTerm = "relay"

func filterTestSessions() []Session {
	var sessions []Session
	for i := range 12 {
		name := fmt.Sprintf("other-%02d", i)
		if i < 4 {
			name = fmt.Sprintf("%s-%d", filterTerm, i)
		}
		sessions = append(sessions, Session{Name: name})
	}
	return sessions
}

func filterTestProjects() []project.Project {
	var projects []project.Project
	for i := range 12 {
		name := fmt.Sprintf("other-%02d", i)
		if i < 2 {
			name = fmt.Sprintf("%s-%d", filterTerm, i)
		}
		projects = append(projects, project.Project{Name: name, Path: "/p/" + name})
	}
	return projects
}

// launchedWith is m as a launch at w×h draws it: sized, then handed its
// sessions, none where noSessions, and projects.
func launchedWith(t *testing.T, m Model, w, h int, noSessions bool) Model {
	t.Helper()
	m = resizedTo(t, m, w, h)
	var sessions []Session
	if !noSessions {
		sessions = filterTestSessions()
	}
	var model tea.Model = m
	model, _ = model.Update(SessionsMsg{Sessions: sessions})
	model, _ = model.Update(ProjectsLoadedMsg{Projects: filterTestProjects()})
	return model.(Model)
}

// typed is m with the keys of s pressed in turn, each one's filter matches
// fed back as the program feeds them.
func typed(t *testing.T, m Model, s string) Model {
	t.Helper()
	var model tea.Model = m
	for _, r := range s {
		var cmd tea.Cmd
		model, cmd = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		model = fedFilterMatches(model, cmd)
	}
	return model.(Model)
}

// fedFilterMatches is model with the filter matches cmd computes fed back,
// from within any batch.
func fedFilterMatches(model tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return model
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			model = fedFilterMatches(model, c)
		}
	case list.FilterMatchesMsg:
		model, _ = model.Update(msg)
	}
	return model
}

// filteredLaunches are the ways a picker comes to show a filtered list, each
// launched at w×h.
func filteredLaunches(t *testing.T) map[string]func(w, h int) Model {
	return map[string]func(w, h int) Model{
		"an initial filter on Sessions": func(w, h int) Model {
			return launchedWith(t, New(fakeLister{}).WithInitialFilter(filterTerm), w, h, false)
		},
		"an initial filter on Projects": func(w, h int) Model {
			return launchedWith(t, New(fakeLister{}).WithInitialFilter(filterTerm), w, h, true)
		},
		"a search": func(w, h int) Model {
			m := Build(Deps{Lister: fakeLister{}, ProjectStore: stubProjectStore{}, Search: &SearchForm{Term: filterTerm}})
			return launchedWith(t, m, w, h, false)
		},
		"a filter typed on Sessions": func(w, h int) Model {
			return typed(t, launchedWith(t, New(fakeLister{}), w, h, false), "/"+filterTerm)
		},
		"a filter typed on Projects": func(w, h int) Model {
			m := launchedWith(t, New(fakeLister{}), w, h, false)
			m.activePage = PageProjects
			return typed(t, m, "/"+filterTerm)
		},
	}
}

// shownList is the list of the page m shows.
func shownList(m Model) list.Model {
	if m.activePage == PageProjects {
		return m.projectList
	}
	return m.sessionList
}

// dotsMisdrawn is how the list m shows pages other than a fresh sizing at
// its size pages it, or draws its pagination dots other than exactly when its
// visible items outgrow a page; "" where it doesn't.
func dotsMisdrawn(m Model) string {
	l := shownList(m)
	fresh := l
	sizeList(&fresh, l.Width(), l.Height())
	n, perPage := len(l.VisibleItems()), l.Paginator.PerPage
	switch dots := strings.Contains(ansi.Strip(l.View()), paginationDotGlyph); {
	case perPage != fresh.Paginator.PerPage:
		return fmt.Sprintf("%d items at %d a page, where a fresh sizing gives %d a page", n, perPage, fresh.Paginator.PerPage)
	case dots != (n > perPage):
		return fmt.Sprintf("%d items at %d a page draw dots %t", n, perPage, dots)
	}
	return ""
}

// A filtered list launched at a size draws as one resized to it does, from a
// size it pages at or one it fits at, and both draw their dots exactly when
// the matches outgrow a page.
func TestAFilteredPageDrawsAsItsResizeDoes(t *testing.T) {
	for name, launch := range filteredLaunches(t) {
		t.Run(name, func(t *testing.T) {
			if got := len(shownList(launch(resizeWidth, resizeMaxHeight)).VisibleItems()); got == 0 || got == 12 {
				t.Fatalf("the launch shows %d items: it isn't filtered", got)
			}
			for _, from := range []int{resizeMinHeight, resizeMaxHeight} {
				before := launch(resizeWidth, from)
				for to := resizeMinHeight; to <= resizeMaxHeight; to++ {
					fresh := launch(resizeWidth, to)
					if problem := dotsMisdrawn(fresh); problem != "" {
						t.Errorf("launched at %d rows, %s:\n%s", to, problem, drawn(fresh))
					}
					if got, want := drawn(resizedTo(t, before, resizeWidth, to)), drawn(fresh); got != want {
						t.Errorf("resized from %d to %d rows draws\n%s\nwant\n%s", from, to, got, want)
					}
				}
			}
		})
	}
}

// The dots show exactly when the visible items outgrow a page after every
// change of them: a filter keystroke, its matches, a filter set outright and
// a filter cleared.
func TestThePaginationDotsShowExactlyWhenTheItemsOutgrowAPage(t *testing.T) {
	for _, pg := range []page{PageSessions, PageProjects} {
		for height := resizeMinHeight; height <= 20; height++ {
			t.Run(fmt.Sprintf("page %d at %d rows", pg, height), func(t *testing.T) {
				dotsFollowEachChangeOfItems(t, pg, height)
			})
		}
	}
}

func dotsFollowEachChangeOfItems(t *testing.T, pg page, height int) {
	m := launchedWith(t, New(fakeLister{}), resizeWidth, height, false)
	m.activePage = pg
	check := func(step string, m Model) {
		t.Helper()
		if problem := dotsMisdrawn(m); problem != "" {
			t.Errorf("%s, %s", step, problem)
		}
	}
	check("launched", m)
	if shownList(m).Paginator.TotalPages < 2 {
		t.Fatalf("the unfiltered list fits a page at %d rows: nothing to cross", height)
	}
	keyed, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	keyed, _ = keyed.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	check("after a filter keystroke", keyed.(Model))
	matched := typed(t, m, "/"+filterTerm)
	check("after the filter's matches", matched)
	cleared := pressed(t, matched, tea.KeyPressMsg{Code: tea.KeyEnter}, tea.KeyPressMsg{Code: tea.KeyEscape})
	if shownList(cleared).FilterState() != list.Unfiltered {
		t.Fatalf("Enter then Escape leaves the filter %v", shownList(cleared).FilterState())
	}
	check("after the filter's cleared", cleared)
	set := cleared
	if pg == PageProjects {
		set.SetProjectListFilter(filterTerm)
	} else {
		set.SetSessionListFilter(filterTerm)
	}
	check("after a filter's set outright", set)
}
