package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

func TestRebuildSessionListResolutionGate(t *testing.T) {
	t.Run("Flat (default) performs zero resolutions over sessions with no recorded directory", func(t *testing.T) {
		cwd := t.TempDir()
		sessions := []Session{
			{Name: "portal-a", Cwd: cwd},
			{Name: "portal-b", Cwd: cwd},
			{Name: "portal-c", Cwd: cwd},
		}

		runner := &fakeDirRunner{gitRoot: cwd}
		m := newRebuildTestModel(t, prefs.ModeFlat, sessions, nil)
		m.dirRunner = runner

		m.rebuildSessionList()

		if len(runner.reads) != 0 {
			t.Errorf("Flat performed %d resolutions (reads=%v), want 0", len(runner.reads), runner.reads)
		}
	})

	t.Run("byTagSignpost (ByTag, zero tags anywhere) performs zero resolutions", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		cwd := t.TempDir()
		sessions := []Session{
			{Name: "portal-a", Cwd: cwd},
			{Name: "portal-b", Cwd: cwd},
		}

		runner := &fakeDirRunner{gitRoot: dir}
		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.dirRunner = runner

		m.rebuildSessionList()

		if !m.byTagSignpost {
			t.Fatalf("expected byTagSignpost to be set (zero tags anywhere)")
		}
		if len(runner.reads) != 0 {
			t.Errorf("signpost performed %d resolutions (reads=%v), want 0", len(runner.reads), runner.reads)
		}
	})

	t.Run("ModeByProject resolves every session with no recorded directory", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		cwd := t.TempDir()
		sessions := []Session{
			{Name: "portal-a", Cwd: cwd},
			{Name: "portal-b", Cwd: cwd},
			{Name: "portal-c", Cwd: cwd},
		}

		runner := &fakeDirRunner{gitRoot: dir}
		m := newRebuildTestModel(t, prefs.ModeByProject, sessions, projects)
		m.dirRunner = runner

		m.rebuildSessionList()

		if len(runner.reads) != len(sessions) {
			t.Errorf("ModeByProject performed %d resolutions (reads=%v), want %d", len(runner.reads), runner.reads, len(sessions))
		}
	})

	t.Run("ModeByTag (tags present) resolves every session with no recorded directory", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		cwd := t.TempDir()
		sessions := []Session{
			{Name: "portal-a", Cwd: cwd},
			{Name: "portal-b", Cwd: cwd},
		}

		runner := &fakeDirRunner{gitRoot: dir}
		m := newRebuildTestModel(t, prefs.ModeByTag, sessions, projects)
		m.dirRunner = runner

		m.rebuildSessionList()

		if m.byTagSignpost {
			t.Fatalf("did not expect byTagSignpost (tags present)")
		}
		if len(runner.reads) != len(sessions) {
			t.Errorf("ModeByTag performed %d resolutions (reads=%v), want %d", len(runner.reads), runner.reads, len(sessions))
		}
	})
}
