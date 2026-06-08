package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// TestRebuildSessionListResolutionGate locks the placement guard: lazy
// stamp-on-render resolution is a grouped-render mechanism. It MUST fire on the
// ModeByProject / ModeByTag (non-signpost) render paths and MUST NOT fire on the
// Flat (default) or byTagSignpost (zero-tags) render paths. fakeStamper's
// ActivePaneCurrentPath reads are the per-session resolution counter: an
// empty-Dir session resolves via one pane read, so len(reads) == number of
// resolutions performed this rebuild.
func TestRebuildSessionListResolutionGate(t *testing.T) {
	t.Run("Flat (default) performs zero resolutions over un-stamped sessions", func(t *testing.T) {
		sessions := []tmux.Session{
			{Name: "portal-a", Dir: ""},
			{Name: "portal-b", Dir: ""},
			{Name: "portal-c", Dir: ""},
		}

		stamper := &fakeStamper{path: t.TempDir()}
		m := newRebuildTestModel(prefs.ModeFlat, sessions, nil)
		m.dirReader = stamper
		m.dirRunner = &fakeDirRunner{gitRoot: t.TempDir()}

		m.rebuildSessionList()

		if len(stamper.reads) != 0 {
			t.Errorf("Flat performed %d resolutions (reads=%v), want 0", len(stamper.reads), stamper.reads)
		}
		if len(stamper.setCalls) != 0 {
			t.Errorf("Flat performed %d stamp writes, want 0", len(stamper.setCalls))
		}
	})

	t.Run("byTagSignpost (ByTag, zero tags anywhere) performs zero resolutions", func(t *testing.T) {
		dir := t.TempDir()
		// Project carries NO tags => anyTagsExist is false => signpost arm.
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{
			{Name: "portal-a", Dir: ""},
			{Name: "portal-b", Dir: ""},
		}

		stamper := &fakeStamper{path: dir}
		m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
		m.dirReader = stamper
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		if !m.byTagSignpost {
			t.Fatalf("expected byTagSignpost to be set (zero tags anywhere)")
		}
		if len(stamper.reads) != 0 {
			t.Errorf("signpost performed %d resolutions (reads=%v), want 0", len(stamper.reads), stamper.reads)
		}
		if len(stamper.setCalls) != 0 {
			t.Errorf("signpost performed %d stamp writes, want 0", len(stamper.setCalls))
		}
	})

	t.Run("ModeByProject resolves every un-stamped session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{
			{Name: "portal-a", Dir: ""},
			{Name: "portal-b", Dir: ""},
			{Name: "portal-c", Dir: ""},
		}

		stamper := &fakeStamper{path: dir}
		m := newRebuildTestModel(prefs.ModeByProject, sessions, projects)
		m.dirReader = stamper
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		if len(stamper.reads) != len(sessions) {
			t.Errorf("ModeByProject performed %d resolutions (reads=%v), want %d", len(stamper.reads), stamper.reads, len(sessions))
		}
	})

	t.Run("ModeByTag (tags present) resolves every un-stamped session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{
			{Name: "portal-a", Dir: ""},
			{Name: "portal-b", Dir: ""},
		}

		stamper := &fakeStamper{path: dir}
		m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
		m.dirReader = stamper
		m.dirRunner = &fakeDirRunner{gitRoot: dir}

		m.rebuildSessionList()

		if m.byTagSignpost {
			t.Fatalf("did not expect byTagSignpost (tags present)")
		}
		if len(stamper.reads) != len(sessions) {
			t.Errorf("ModeByTag performed %d resolutions (reads=%v), want %d", len(stamper.reads), stamper.reads, len(sessions))
		}
	})
}
