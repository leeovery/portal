package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"charm.land/bubbles/v2/list"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

// fakeDirRunner answers git's root query with gitRoot, recording the directory
// each query asked about.
type fakeDirRunner struct {
	gitRoot string
	reads   []string
}

func (r *fakeDirRunner) Run(name string, args ...string) (string, error) {
	r.reads = append(r.reads, args[1])
	return r.gitRoot, nil
}

func TestRebuildSessionListDirResolution(t *testing.T) {
	t.Run("By Project: an empty-Dir session resolving via its cwd's git root appears under its project, not Unknown", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Cwd: dir}}

		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		rows := sessionRows(m.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(session rows) = %d, want 1", len(rows))
		}
		si := rows[0]
		if si.CatchAll {
			t.Fatalf("session routed to Unknown catch-all, want resolved under its project")
		}
		if si.GroupKey != key {
			t.Errorf("GroupKey = %q, want %q", si.GroupKey, key)
		}
		if si.GroupHeading != "Portal" {
			t.Errorf("GroupHeading = %q, want %q", si.GroupHeading, "Portal")
		}
		if m.sessions[0].Dir != "" {
			t.Errorf("m.sessions[0].Dir = %q, want \"\" (the derived value must not land in the recorded one)", m.sessions[0].Dir)
		}
		if si.Session.Dir != "" {
			t.Errorf("SessionItem.Session.Dir = %q, want \"\" (the item carries the recorded directory verbatim)", si.Session.Dir)
		}
	})

	t.Run("By Tag: an empty-Dir session resolving to a tagged project appears under its tags, not Untagged", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}}}
		sessions := []Session{{Name: "portal-abc", Cwd: dir}}

		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		rows := sessionRows(m.sessionList.Items())
		if len(rows) != 2 {
			t.Fatalf("len(session rows) = %d, want 2 (one per tag)", len(rows))
		}
		for _, si := range rows {
			if si.CatchAll {
				t.Fatalf("session routed to Untagged catch-all, want resolved under its tags")
			}
			if si.GroupKey == "" {
				t.Errorf("By Tag item has empty GroupKey (canonical tag): %+v", si)
			}
			if si.Session.Dir != "" {
				t.Errorf("SessionItem.Session.Dir = %q, want \"\" (the item carries the recorded directory verbatim)", si.Session.Dir)
			}
		}
		if m.sessions[0].Dir != "" {
			t.Errorf("m.sessions[0].Dir = %q, want \"\" (the derived value must not land in the recorded one)", m.sessions[0].Dir)
		}
	})

	t.Run("it asks git for the root of the row's own cwd", func(t *testing.T) {
		dir := t.TempDir()
		cwd := filepath.Join(dir, "internal")
		if err := os.Mkdir(cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Cwd: cwd}}

		runner := &fakeDirRunner{gitRoot: dir}
		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.dirRunner = runner

		m.rebuildSessionList()

		if !slices.Equal(runner.reads, []string{cwd}) {
			t.Errorf("git-root queries = %v, want one for the cwd %q", runner.reads, cwd)
		}
		if rows := sessionRows(m.sessionList.Items()); len(rows) != 1 || rows[0].GroupHeading != "Portal" {
			t.Errorf("rows = %+v, want the session under its cwd's project", rows)
		}
	})

	t.Run("it never resolves a session whose directory the index records", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Dir: dir, Cwd: t.TempDir()}}

		runner := &fakeDirRunner{gitRoot: t.TempDir()}
		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.dirRunner = runner

		m.rebuildSessionList()

		if len(runner.reads) != 0 {
			t.Errorf("git-root queries = %v, want none for a session with a recorded directory", runner.reads)
		}
		if rows := sessionRows(m.sessionList.Items()); len(rows) != 1 || rows[0].GroupHeading != "Portal" {
			t.Errorf("rows = %+v, want the session under its recorded directory's project", rows)
		}
	})

	t.Run("caches the derived directory outside the recorded one", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Cwd: dir}}

		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		if m.sessions[0].Dir != "" {
			t.Errorf("m.sessions[0].Dir = %q, want \"\" (the recorded directory stays untouched)", m.sessions[0].Dir)
		}
		if got := m.derivedDirs["portal-abc"]; got != key {
			t.Errorf("m.derivedDirs[%q] = %q, want %q (cached)", "portal-abc", got, key)
		}
	})

	t.Run("second rebuild reuses the cache and asks git nothing more", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Cwd: dir}}

		runner := &fakeDirRunner{gitRoot: dir}
		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.dirRunner = runner

		m.rebuildSessionList()
		if len(runner.reads) != 1 {
			t.Fatalf("first rebuild reads = %d, want 1", len(runner.reads))
		}

		runner.reads = nil
		m.rebuildSessionList()
		if len(runner.reads) != 0 {
			t.Errorf("second rebuild asked git %d times, want 0 (cache fast-path)", len(runner.reads))
		}
	})

	// A session that did not answer, or whose backend names no cwd, reaches the
	// picker with an empty one.
	t.Run("it treats an empty cwd as unresolved in the grouped render", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc"}}

		runner := &fakeDirRunner{gitRoot: dir}
		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.dirRunner = runner

		m.rebuildSessionList()

		rows := sessionRows(m.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(session rows) = %d, want 1", len(rows))
		}
		if !rows[0].CatchAll {
			t.Fatalf("an empty cwd must route the session to the Unknown catch-all")
		}
		if rows[0].GroupHeading != unknownHeading {
			t.Errorf("GroupHeading = %q, want %q (nothing was resolved)", rows[0].GroupHeading, unknownHeading)
		}
		if m.sessions[0].Dir != "" {
			t.Errorf("m.sessions[0].Dir = %q, want \"\" (unresolvable, nothing to cache)", m.sessions[0].Dir)
		}
		if got, ok := m.derivedDirs["portal-abc"]; ok {
			t.Errorf("m.derivedDirs[%q] = %q, want absent (unresolvable sessions are not negative-cached)", "portal-abc", got)
		}
		if len(runner.reads) != 0 {
			t.Errorf("git-root queries = %v, want none for an empty cwd", runner.reads)
		}
	})

	t.Run("it discards derived values when the session list is refreshed", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Cwd: dir}}

		runner := &fakeDirRunner{gitRoot: dir}
		m := newRebuildTestModel(t, prefs.ModeByProject, nil, projects)
		m.dirRunner = runner

		m.applySessions(sessions, nil)
		if len(runner.reads) != 1 {
			t.Fatalf("first refresh reads = %d, want 1", len(runner.reads))
		}

		runner.reads = nil
		m.applySessions(sessions, nil)
		if len(runner.reads) != 1 {
			t.Errorf("second refresh asked git %d times, want 1 (the refresh discards the derived value)", len(runner.reads))
		}
	})

	t.Run("nil seam does not panic and routes an empty-Dir session to Unknown", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Cwd: dir}}

		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)

		m.rebuildSessionList()

		rows := sessionRows(m.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(session rows) = %d, want 1", len(rows))
		}
		if !rows[0].CatchAll {
			t.Fatalf("with a nil seam an empty-Dir session must route to the Unknown catch-all")
		}
	})

	t.Run("it does not match a session on a grouping-derived directory", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []Session{{Name: "portal-abc", Cwd: dir}}

		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		if m.derivedDirs["portal-abc"] != key {
			t.Fatalf("precondition: derivedDirs[%q] = %q, want %q", "portal-abc", m.derivedDirs["portal-abc"], key)
		}
		rows := sessionRows(m.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(session rows) = %d, want 1", len(rows))
		}
		if got := rows[0].FilterValue(); got != "portal-abc" {
			t.Errorf("FilterValue() = %q, want %q (the derived directory is no part of the matched text)", got, "portal-abc")
		}

		m.sessionList.SetFilterText(key)
		m.sessionList.SetFilterState(list.FilterApplied)

		if visible := m.sessionList.VisibleItems(); len(visible) != 0 {
			t.Errorf("filtering on the derived directory left %d visible items, want 0", len(visible))
		}
	})
}
