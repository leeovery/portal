package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

func newProjectsLoadedRegroupModel(t *testing.T, mode prefs.SessionListMode, sessions []Session) Model {
	t.Helper()
	m := Model{
		themeState:      themeState{active: testDarkTheme(t)},
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
		sessions := []Session{{Name: "portal-abc", Answered: true, Dir: dir}}

		m := newProjectsLoadedRegroupModel(t, prefs.ModeByProject, sessions)

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		rows := sessionRows(got.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
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
		sessions := []Session{{Name: "portal-abc", Answered: true, Dir: dir}}

		m := newProjectsLoadedRegroupModel(t, prefs.ModeByTag, sessions)

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		rows := sessionRows(got.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if si.GroupHeading != "work" || si.GroupKey != "work" {
			t.Errorf("expected session under tag heading %q, got heading=%q key=%q", "work", si.GroupHeading, si.GroupKey)
		}
	})

	t.Run("batches the project-list SetItems command with the rebuild command", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Answered: true, Dir: dir}}

		m := newProjectsLoadedRegroupModel(t, prefs.ModeByProject, sessions)
		m.termWidth, m.termHeight = 80, 24

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		if len(got.projectList.Items()) != 1 {
			t.Errorf("projectList items = %d, want 1 (setItemsCmd dropped?)", len(got.projectList.Items()))
		}
		si := sessionRows(got.sessionList.Items())[0]
		if si.GroupHeading != "Portal" {
			t.Errorf("session not re-grouped; GroupHeading = %q, want Portal", si.GroupHeading)
		}
	})

	t.Run("does NOT re-group in Flat mode", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Answered: true, Dir: dir}}

		m := newProjectsLoadedRegroupModel(t, prefs.ModeFlat, sessions)
		m.rebuildSessionList()

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		items := got.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
		if si.GroupKey != "" || si.GroupHeading != "" || si.CatchAll {
			t.Errorf("item is grouped (key=%q heading=%q catchAll=%v), want flat",
				si.GroupKey, si.GroupHeading, si.CatchAll)
		}
	})

	t.Run("rebuilds the lookup index on each ProjectsLoadedMsg so a later add is reflected and a removed project no longer matches", func(t *testing.T) {
		dirA := t.TempDir()
		dirB := t.TempDir()
		keyB := project.CanonicalDirKey(dirB)
		sessions := []Session{
			{Name: "alpha", Answered: true, Dir: dirA},
			{Name: "bravo", Answered: true, Dir: dirB},
		}

		m := newProjectsLoadedRegroupModel(t, prefs.ModeByProject, sessions)

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: []project.Project{{Path: dirA, Name: "Alpha"}}})
		m = updated.(Model)

		updated, _ = m.Update(ProjectsLoadedMsg{Projects: []project.Project{{Path: dirB, Name: "Bravo"}}})
		got := updated.(Model)

		var alphaItem, bravoItem SessionItem
		for _, si := range sessionRows(got.sessionList.Items()) {
			switch si.Session.Name {
			case "alpha":
				alphaItem = si
			case "bravo":
				bravoItem = si
			}
		}

		if !alphaItem.CatchAll || alphaItem.GroupHeading != unknownHeading {
			t.Errorf("alpha: catchAll=%v heading=%q, want Unknown catch-all (removed project must not match a stale index)",
				alphaItem.CatchAll, alphaItem.GroupHeading)
		}
		if bravoItem.CatchAll || bravoItem.GroupHeading != "Bravo" || bravoItem.GroupKey != keyB {
			t.Errorf("bravo: catchAll=%v heading=%q key=%q, want grouped under Bravo (added project must be reflected)",
				bravoItem.CatchAll, bravoItem.GroupHeading, bravoItem.GroupKey)
		}
	})

	t.Run("does NOT re-group when no sessions are ingested", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}

		m := newProjectsLoadedRegroupModel(t, prefs.ModeByProject, nil)

		updated, _ := m.Update(ProjectsLoadedMsg{Projects: projects})
		got := updated.(Model)

		if n := len(got.sessionList.Items()); n != 0 {
			t.Errorf("session items = %d, want 0 (no re-group with empty sessions)", n)
		}
		if !got.projectsLoaded {
			t.Error("expected projectsLoaded = true")
		}
		if len(got.projects) != 1 {
			t.Errorf("cached projects = %d, want 1", len(got.projects))
		}
	})
}
