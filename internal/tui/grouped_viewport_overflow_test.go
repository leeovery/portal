package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

func TestGroupedViewDoesNotOverflowViewport(t *testing.T) {
	const w, h = 100, 30

	var sessions []Session
	var projects []project.Project
	for i := range 12 {
		dir := t.TempDir()
		projects = append(projects, project.Project{Name: fmt.Sprintf("proj%02d", i), Path: dir})
		sessions = append(sessions,
			Session{Name: fmt.Sprintf("proj%02d-a", i), Dir: dir},
			Session{Name: fmt.Sprintf("proj%02d-b", i), Dir: dir},
		)
	}

	for _, mode := range []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag} {
		m := Model{
			themeState:      themeState{active: testDarkTheme(t)},
			sessions:        sessions,
			projects:        projects,
			projectIndex:    project.NewIndex(projects),
			sessionList:     newSessionList(nil),
			projectList:     newProjectList(),
			activePage:      PageSessions,
			sessionListMode: mode,
			termWidth:       w,
			termHeight:      h,
		}
		if mode == prefs.ModeByTag {
			for i := range m.projects {
				m.projects[i].Tags = []string{"work"}
			}
			m.projectIndex = project.NewIndex(m.projects)
		}
		m.applySessionListSize(w, h)
		m.rebuildSessionList()

		view := m.viewSessionList()
		gotHeight := lipgloss.Height(view)
		if gotHeight > h {
			t.Errorf("mode %v: rendered view height = %d, want <= %d (viewport overflow)", mode, gotHeight, h)
		}

		lines := strings.Split(view, "\n")
		if len(lines) > h {
			lines = lines[:h]
		}
		if !strings.Contains(strings.Join(lines, "\n"), "Sessions") {
			t.Errorf("mode %v: title 'Sessions' not visible in the rendered frame", mode)
		}
	}
}
