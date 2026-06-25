package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// TestGroupedViewDoesNotOverflowViewport is the regression guard for the
// cursor-invisible / missing-title / left-shift cluster: with group headings
// drawn as height-1 list rows, a rendered page of the grouped session list must
// never exceed the terminal height. The earlier in-delegate heading injection
// drew uncounted extra lines, so the frame overflowed and the terminal scrolled
// the title row (and the cursor on the first session) off the top.
func TestGroupedViewDoesNotOverflowViewport(t *testing.T) {
	const w, h = 100, 30

	// 12 projects × 2 sessions => ~12 group headers interleaved with 24 rows,
	// which under the old (Height()=1, headers-inside-render) design overflowed
	// a single page well past the viewport.
	var sessions []tmux.Session
	var projects []project.Project
	for i := range 12 {
		dir := t.TempDir()
		projects = append(projects, project.Project{Name: fmt.Sprintf("proj%02d", i), Path: dir})
		sessions = append(sessions,
			tmux.Session{Name: fmt.Sprintf("proj%02d-a", i), Dir: dir, Windows: 1},
			tmux.Session{Name: fmt.Sprintf("proj%02d-b", i), Dir: dir, Windows: 1},
		)
	}

	for _, mode := range []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag} {
		m := Model{
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
		// By-Tag with zero tags degrades to the signpost flat list; give every
		// project a tag so the genuine grouped (header-bearing) path renders.
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

		// The title row must be present within the visible frame (a terminal
		// shows the bottom `h` lines on overflow, which would hide a top title).
		lines := strings.Split(view, "\n")
		if len(lines) > h {
			lines = lines[:h]
		}
		if !strings.Contains(strings.Join(lines, "\n"), "Sessions") {
			t.Errorf("mode %v: title 'Sessions' not visible in the rendered frame", mode)
		}
	}
}
