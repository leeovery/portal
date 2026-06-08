package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// newProjectsLoadedRegroupModel builds a Model parked on the sessions page with
// sessions already ingested but projects NOT yet loaded (m.projects nil) — the
// SessionsMsg-before-ProjectsLoadedMsg startup ordering this fix targets. The
// dir-resolver seam is intentionally left unwired (no WithDirResolver), so each
// session's seeded Dir is grouped verbatim.
func newProjectsLoadedRegroupModel(mode prefs.SessionListMode, sessions []tmux.Session) Model {
	m := Model{
		sessions:        sessions,
		sessionList:     newSessionList(nil),
		projectList:     newProjectList(),
		activePage:      PageSessions,
		sessionListMode: mode,
	}
	m.applySessionListSize(80, 24)
	return m
}

func TestProjectsLoadedRegroup(t *testing.T) {
	t.Run("re-groups By Project when ProjectsLoadedMsg arrives after sessions", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		// Sessions ingested first; m.projects still nil → the initial render
		// would have grouped the session into Unknown. ProjectsLoadedMsg must
		// correct it WITHOUT pre-seeding m.projects.
		m := newProjectsLoadedRegroupModel(prefs.ModeByProject, sessions)

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		items := got.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.CatchAll {
			t.Fatalf("session landed in Unknown catch-all; expected re-group under project")
		}
		if si.GroupKey != key {
			t.Errorf("GroupKey = %q, want %q", si.GroupKey, key)
		}
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
		}
	})

	t.Run("re-groups By Tag when ProjectsLoadedMsg arrives after sessions", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newProjectsLoadedRegroupModel(prefs.ModeByTag, sessions)

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		items := got.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.GroupHeading != "work" || si.Tag != "work" {
			t.Errorf("expected session under tag heading %q, got heading=%q tag=%q", "work", si.GroupHeading, si.Tag)
		}
	})

	t.Run("batches the project-list SetItems command with the rebuild command", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newProjectsLoadedRegroupModel(prefs.ModeByProject, sessions)
		m.termWidth, m.termHeight = 80, 24

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		// The project list must still be populated (setItemsCmd not dropped).
		if len(got.projectList.Items()) != 1 {
			t.Errorf("projectList items = %d, want 1 (setItemsCmd dropped?)", len(got.projectList.Items()))
		}
		// And the session list must be re-grouped (rebuild cmd not dropped).
		si := asSessionItem(t, got.sessionList.Items()[0])
		if si.GroupHeading != "Portal" {
			t.Errorf("session not re-grouped; GroupHeading = %q, want Portal", si.GroupHeading)
		}
	})

	t.Run("does NOT re-group in Flat mode", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newProjectsLoadedRegroupModel(prefs.ModeFlat, sessions)
		// Establish the initial flat render (as applySessions would have at
		// startup) so we can assert ProjectsLoadedMsg leaves it untouched.
		m.rebuildSessionList()

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		// Flat-mode items must remain ungrouped — no grouping side effect.
		items := got.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.GroupKey != "" || si.GroupHeading != "" || si.Tag != "" || si.CatchAll {
			t.Errorf("item is grouped (key=%q heading=%q tag=%q catchAll=%v), want flat",
				si.GroupKey, si.GroupHeading, si.Tag, si.CatchAll)
		}
	})

	t.Run("does NOT re-group when no sessions are ingested", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}

		m := newProjectsLoadedRegroupModel(prefs.ModeByProject, nil)

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		if n := len(got.sessionList.Items()); n != 0 {
			t.Errorf("session items = %d, want 0 (no re-group with empty sessions)", n)
		}
		// projectsLoaded + projects caching still happen.
		if !got.projectsLoaded {
			t.Error("expected projectsLoaded = true")
		}
		if len(got.projects) != 1 {
			t.Errorf("cached projects = %d, want 1", len(got.projects))
		}
	})
}
