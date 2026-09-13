package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

func newRebuildTestModel(t *testing.T, mode prefs.SessionListMode, sessions []tmux.Session, projects []project.Project) Model {
	t.Helper()
	m := Model{
		themeState:      themeState{active: testDarkTheme(t)},
		sessions:        sessions,
		projects:        projects,
		projectIndex:    project.NewIndex(projects),
		sessionList:     newSessionList(nil),
		projectList:     newProjectList(),
		activePage:      PageSessions,
		sessionListMode: mode,
	}
	m.applySessionListSize(80, 24)
	return m
}

func TestRebuildSessionList(t *testing.T) {
	t.Run("builds flat items when the mode is Flat", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha"},
			{Name: "bravo"},
		}
		m := newRebuildTestModel(t, prefs.ModeFlat, sessions, nil)

		m.rebuildSessionList()

		items := m.sessionList.Items()
		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2", len(items))
		}
		for i, it := range items {
			si := asSessionItem(t, it)
			if si.GroupKey != "" || si.GroupHeading != "" || si.CatchAll {
				t.Errorf("item %d is grouped (key=%q heading=%q catchAll=%v), want flat",
					i, si.GroupKey, si.GroupHeading, si.CatchAll)
			}
		}
	})

	t.Run("flat-mode output is byte-for-byte today's flat items", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha", Windows: 2, Attached: true},
			{Name: "bravo", Windows: 1},
		}
		m := newRebuildTestModel(t, prefs.ModeFlat, sessions, nil)

		m.rebuildSessionList()

		got := m.sessionList.Items()
		want := ToListItems(sessions)
		if len(got) != len(want) {
			t.Fatalf("len(got) = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if asSessionItem(t, got[i]) != asSessionItem(t, want[i]) {
				t.Errorf("item %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("builds By Project items when the mode is By Project", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.rebuildSessionList()

		items := m.sessionList.Items()
		want := buildByProject(sessions, project.NewIndex(projects), nil)
		if len(items) != len(want) {
			t.Fatalf("len(items) = %d, want %d", len(items), len(want))
		}
		rows := sessionRows(items)
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if si.GroupKey != key {
			t.Errorf("GroupKey = %q, want %q", si.GroupKey, key)
		}
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
		}
	})

	t.Run("builds By Tag items when the mode is By Tag", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.rebuildSessionList()

		items := m.sessionList.Items()
		want := buildByTag(sessions, project.NewIndex(projects), nil)
		if len(items) != len(want) {
			t.Fatalf("len(items) = %d, want %d", len(items), len(want))
		}
		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("len(rows) = %d, want 2 (one per tag)", len(rows))
		}
		for _, si := range rows {
			if si.GroupKey == "" {
				t.Errorf("By Tag item has empty GroupKey (canonical tag): %+v", si)
			}
		}
	})

	t.Run("feeds cached project records to the grouping builders", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		without := newRebuildTestModel(t, prefs.ModeByProject, sessions, nil)
		without.rebuildSessionList()
		if !sessionRows(without.sessionList.Items())[0].CatchAll {
			t.Fatalf("without cached projects: expected Unknown catch-all item")
		}

		with := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		with.rebuildSessionList()
		si := sessionRows(with.sessionList.Items())[0]
		if si.CatchAll {
			t.Fatalf("with cached projects: expected resolved item, got catch-all")
		}
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
		}
	})

	t.Run("produces an empty list for zero live sessions in every mode", func(t *testing.T) {
		modes := []prefs.SessionListMode{prefs.ModeFlat, prefs.ModeByProject, prefs.ModeByTag}
		for _, mode := range modes {
			m := newRebuildTestModel(t, mode, nil, nil)
			m.rebuildSessionList()
			if got := len(m.sessionList.Items()); got != 0 {
				t.Errorf("mode %v: len(items) = %d, want 0", mode, got)
			}
		}
	})

	t.Run("is idempotent when re-rendering the same mode with the same inputs", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.rebuildSessionList()
		first := append([]list.Item(nil), m.sessionList.Items()...)

		m.rebuildSessionList()
		second := m.sessionList.Items()

		if len(first) != len(second) {
			t.Fatalf("len mismatch: first %d, second %d", len(first), len(second))
		}
		for i := range first {
			if first[i] != second[i] {
				t.Errorf("item %d differs across re-renders: %+v vs %+v", i, first[i], second[i])
			}
		}
	})

	t.Run("preserves the active mode across a SessionsMsg refresh", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.activePage = PageSessions

		updated, _ := m.Update(SessionsMsg{Sessions: sessions})
		mm := updated.(Model)

		if mm.sessionListMode != prefs.ModeByProject {
			t.Fatalf("sessionListMode = %v after refresh, want ModeByProject", mm.sessionListMode)
		}
		rows := sessionRows(mm.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q (mode reverted to Flat?)", si.GroupHeading, "Portal")
		}
	})

	t.Run("excludes the current session when inside tmux", func(t *testing.T) {
		sessions := []tmux.Session{{Name: "alpha"}, {Name: "current"}}
		m := newRebuildTestModel(t, prefs.ModeFlat, sessions, nil)
		m.insideTmux = true
		m.currentSession = "current"

		m.rebuildSessionList()

		items := m.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1 (current excluded)", len(items))
		}
		if asSessionItem(t, items[0]).Session.Name != "alpha" {
			t.Errorf("remaining item = %q, want alpha", asSessionItem(t, items[0]).Session.Name)
		}
	})
}

func TestWithInsideTmuxRoutesThroughRebuild(t *testing.T) {
	t.Run("routes through rebuildSessionList so an already-populated grouped list is grouped", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{
			{Name: "portal-abc", Dir: dir},
			{Name: "current", Dir: dir},
		}

		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m = m.WithInsideTmux("current")

		rows := sessionRows(m.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(rows) = %d, want 1 (current excluded)", len(rows))
		}
		si := rows[0]
		if si.Session.Name == "current" {
			t.Error("current session should be excluded from list items")
		}
		if si.GroupKey != key {
			t.Errorf("GroupKey = %q, want %q (items not grouped — chokepoint bypassed?)", si.GroupKey, key)
		}
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q (items not grouped — chokepoint bypassed?)", si.GroupHeading, "Portal")
		}
	})

	t.Run("preserves construction behaviour: empty sessions yields empty list and mode-aware inside-tmux title", func(t *testing.T) {
		m := newRebuildTestModel(t, prefs.ModeByTag, nil, nil)
		m = m.WithInsideTmux("current")

		if got := len(m.sessionList.Items()); got != 0 {
			t.Fatalf("len(items) = %d, want 0 for empty sessions at construction", got)
		}
		want := "Sessions — by tag (current: current)"
		if got := m.sessionList.Title; got != want {
			t.Errorf("title = %q, want %q", got, want)
		}
	})

	t.Run("still excludes the current session under WithInsideTmux in flat mode", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "alpha"},
			{Name: "current"},
			{Name: "bravo"},
		}
		m := newRebuildTestModel(t, prefs.ModeFlat, sessions, nil)
		m = m.WithInsideTmux("current")

		items := m.sessionList.Items()
		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2 (current excluded)", len(items))
		}
		for _, it := range items {
			if asSessionItem(t, it).Session.Name == "current" {
				t.Error("current session should be excluded from list items")
			}
		}
	})
}
