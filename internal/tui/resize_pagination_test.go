package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
)

// The sizes the resize tests move between: from the least height to the
// greatest, the pickers' lists go from paging to fitting on one page.
const (
	resizeWidth     = 100
	resizeMinHeight = 13
	resizeMaxHeight = 40
)

func resizeTestSessions() []Session {
	sessions := make([]Session, 12)
	for i := range sessions {
		sessions[i] = Session{Name: fmt.Sprintf("session-%02d", i)}
	}
	return sessions
}

func resizeTestProjects() []project.Project {
	projects := make([]project.Project, 12)
	for i := range projects {
		projects[i] = project.Project{Name: fmt.Sprintf("project-%02d", i), Path: fmt.Sprintf("/p/project-%02d", i)}
	}
	return projects
}

// loadedAt is the picker showing pg as a launch at w×h draws it: sized
// first, then handed its sessions and projects.
func loadedAt(t *testing.T, pg page, w, h int) Model {
	t.Helper()
	m := resizedTo(t, New(fakeLister{}), w, h)
	m.applySessions(resizeTestSessions(), nil)
	updated, _ := m.Update(ProjectsLoadedMsg{Projects: resizeTestProjects()})
	m = updated.(Model)
	m.activePage = pg
	return m
}

func resizedTo(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(Model)
}

// pages is how many pages the list pg shows has.
func pages(m Model, pg page) int {
	if pg == PageProjects {
		return m.projectList.Paginator.TotalPages
	}
	return m.sessionList.Paginator.TotalPages
}

// A resize lands each picker page where a launch at the new size draws it,
// from a size its list pages at or one it fits at, to every height between:
// the list never keeps a pagination row it no longer shows, nor leaves out
// one it now does, so the footer neither goes off the foot nor floats.
func TestAResizeDrawsThePageAsALaunchAtThatSizeDoes(t *testing.T) {
	for _, pg := range []page{PageSessions, PageProjects} {
		for _, from := range []int{resizeMinHeight, resizeMaxHeight} {
			t.Run(fmt.Sprintf("page %d from %d rows", pg, from), func(t *testing.T) {
				before := loadedAt(t, pg, resizeWidth, from)
				if (pages(before, pg) > 1) != (from == resizeMinHeight) {
					t.Fatalf("at %d rows the list has %d pages: the heights no longer span paging", from, pages(before, pg))
				}
				for to := resizeMinHeight; to <= resizeMaxHeight; to++ {
					got := resizedTo(t, before, resizeWidth, to).View().Content
					if want := loadedAt(t, pg, resizeWidth, to).View().Content; got != want {
						t.Errorf("resized to %d rows draws\n%s\nwant\n%s", to, ansi.Strip(got), ansi.Strip(want))
					}
					if footer := strings.Split(ansi.Strip(got), "\n")[to-1-Vinset]; !strings.Contains(footer, "? help") {
						t.Errorf("resized to %d rows, the footer row reads %q", to, footer)
					}
				}
			})
		}
	}
}
