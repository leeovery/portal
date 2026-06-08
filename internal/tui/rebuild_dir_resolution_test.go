package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeStamper is a stand-in for the session.PaneCurrentPathReader seam the lazy
// directory-resolution fallback consumes: it reads the active pane's
// current_path and records each read so tests can count resolutions. It also
// implements SetSessionOption so tests can assert the fallback NEVER stamps
// (the derived dir is cached in-memory only, never frozen onto @portal-dir).
type fakeStamper struct {
	path string
	err  error

	setErr   error
	setCalls []stamperSetCall
	reads    []string
}

type stamperSetCall struct {
	session string
	name    string
	value   string
}

func (f *fakeStamper) ActivePaneCurrentPath(sess string) (string, error) {
	f.reads = append(f.reads, sess)
	return f.path, f.err
}

func (f *fakeStamper) SetSessionOption(sess, name, value string) error {
	f.setCalls = append(f.setCalls, stamperSetCall{session: sess, name: name, value: value})
	return f.setErr
}

// Compile-time proof the fake satisfies the production reader seam.
var _ session.PaneCurrentPathReader = (*fakeStamper)(nil)

// fakeDirRunner is a stand-in for resolver.CommandRunner returning a fixed
// git-root so the render-path resolution can be unit-tested without git.
type fakeDirRunner struct {
	gitRoot string
}

func (r *fakeDirRunner) Run(name string, args ...string) (string, error) {
	return r.gitRoot, nil
}

func TestRebuildSessionListDirResolution(t *testing.T) {
	t.Run("By Project: an empty-Dir session resolving via the reader appears under its project, not Unknown", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirReader = &fakeStamper{path: dir}
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
	})

	t.Run("By Tag: an empty-Dir session resolving to a tagged project appears under its tags, not Untagged", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
		m.dirReader = &fakeStamper{path: dir}
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
		}
	})

	t.Run("caches the derived directory into m.sessions and never stamps tmux", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		reader := &fakeStamper{path: dir}
		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirReader = reader
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		// The lazy fallback must NOT stamp @portal-dir: freezing a (possibly
		// drifted) pane cwd would permanently mis-group the session.
		if len(reader.setCalls) != 0 {
			t.Fatalf("expected 0 stamp writes (no freezing), got %d: %v", len(reader.setCalls), reader.setCalls)
		}
		// It MUST cache the derived dir back onto m.sessions so subsequent
		// rebuilds take the fast path.
		if m.sessions[0].Dir != key {
			t.Errorf("m.sessions[0].Dir = %q, want %q (cached)", m.sessions[0].Dir, key)
		}
	})

	t.Run("second rebuild reuses the cache and performs no further pane read", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		reader := &fakeStamper{path: dir}
		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirReader = reader
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()
		if len(reader.reads) != 1 {
			t.Fatalf("first rebuild reads = %d, want 1", len(reader.reads))
		}

		reader.reads = nil
		m.rebuildSessionList()
		if len(reader.reads) != 0 {
			t.Errorf("second rebuild performed %d pane reads, want 0 (cache fast-path)", len(reader.reads))
		}
	})

	t.Run("an unresolvable empty-Dir session falls through to Unknown and is not cached", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		// Empty current_path => unresolvable: no cache, routes to Unknown.
		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirReader = &fakeStamper{path: ""}
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		rows := sessionRows(m.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(session rows) = %d, want 1", len(rows))
		}
		if !rows[0].CatchAll {
			t.Fatalf("unresolvable session must route to the Unknown catch-all")
		}
		if m.sessions[0].Dir != "" {
			t.Errorf("m.sessions[0].Dir = %q, want \"\" (unresolvable, nothing to cache)", m.sessions[0].Dir)
		}
	})

	t.Run("nil seam does not panic and routes an empty-Dir session to Unknown", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		// dirReader / dirRunner left nil — no Option wired.

		m.rebuildSessionList()

		rows := sessionRows(m.sessionList.Items())
		if len(rows) != 1 {
			t.Fatalf("len(session rows) = %d, want 1", len(rows))
		}
		if !rows[0].CatchAll {
			t.Fatalf("with a nil seam an empty-Dir session must route to the Unknown catch-all")
		}
	})
}
