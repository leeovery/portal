package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeStamper is a stand-in for the session.PaneStamper seam the lazy
// stamp-on-render fallback consumes: it reads the active pane's current_path
// AND records each SetSessionOption stamp so tests can assert the
// derive-use-then-stamp behaviour in the render path.
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

// Compile-time proof the fake satisfies the production seam.
var _ session.PaneStamper = (*fakeStamper)(nil)

// fakeDirRunner is a stand-in for resolver.CommandRunner returning a fixed
// git-root so the render-path resolution can be unit-tested without git.
type fakeDirRunner struct {
	gitRoot string
}

func (r *fakeDirRunner) Run(name string, args ...string) (string, error) {
	return r.gitRoot, nil
}

func TestRebuildSessionListDirResolution(t *testing.T) {
	t.Run("By Project: an empty-Dir session resolving via the stamper appears under its project, not Unknown", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirStamper = &fakeStamper{path: dir}
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		items := m.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		si := asSessionItem(t, items[0])
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
		m.dirStamper = &fakeStamper{path: dir}
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		items := m.sessionList.Items()
		if len(items) != 2 {
			t.Fatalf("len(items) = %d, want 2 (one per tag)", len(items))
		}
		for _, it := range items {
			si := asSessionItem(t, it)
			if si.CatchAll {
				t.Fatalf("session routed to Untagged catch-all, want resolved under its tags")
			}
			if si.GroupKey == "" {
				t.Errorf("By Tag item has empty GroupKey (canonical tag): %+v", si)
			}
		}
	})

	t.Run("stamps the derived directory after the first grouped render", func(t *testing.T) {
		dir := t.TempDir()
		key := project.CanonicalDirKey(dir)
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		stamper := &fakeStamper{path: dir}
		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirStamper = stamper
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		if len(stamper.setCalls) != 1 {
			t.Fatalf("expected exactly 1 stamp write, got %d: %v", len(stamper.setCalls), stamper.setCalls)
		}
		got := stamper.setCalls[0]
		if got.session != "portal-abc" || got.name != session.PortalDirOption || got.value != key {
			t.Errorf("stamp call = %+v, want session=portal-abc name=%s value=%s", got, session.PortalDirOption, key)
		}
	})

	t.Run("an unresolvable empty-Dir session falls through to Unknown", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		// Empty current_path => unresolvable: no stamp, routes to Unknown.
		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirStamper = &fakeStamper{path: ""}
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		items := m.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		if !asSessionItem(t, items[0]).CatchAll {
			t.Fatalf("unresolvable session must route to the Unknown catch-all")
		}
	})

	t.Run("does not mutate the stored session slice", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirStamper = &fakeStamper{path: dir}
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		if m.sessions[0].Dir != "" {
			t.Errorf("m.sessions[0].Dir = %q, want %q (render-path resolution must not mutate m.sessions)", m.sessions[0].Dir, "")
		}
	})

	t.Run("nil seam does not panic and routes an empty-Dir session to Unknown", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: ""}}

		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		// dirStamper / dirRunner left nil — no Option wired.

		m.rebuildSessionList()

		items := m.sessionList.Items()
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, want 1", len(items))
		}
		if !asSessionItem(t, items[0]).CatchAll {
			t.Fatalf("with a nil seam an empty-Dir session must route to the Unknown catch-all")
		}
	})
}
