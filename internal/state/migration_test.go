package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

func TestMigrationCutShort(t *testing.T) {
	// dirs makes a state dir's siblings: state-tmux/ as a dir or a file, and
	// the staging dir empty or holding sessions.json.
	type dirs struct{ tmuxDir, tmuxFile, staging, staged bool }
	arrange := func(t *testing.T, d dirs) string {
		t.Helper()
		root := t.TempDir()
		dir := filepath.Join(root, "state")
		if d.tmuxDir {
			mustMkdir(t, state.TmuxStateDir(dir))
		}
		if d.tmuxFile {
			mustWrite(t, state.TmuxStateDir(dir), "")
		}
		if d.staging || d.staged {
			mustMkdir(t, state.MigrationStagingDir(dir))
		}
		if d.staged {
			mustWrite(t, filepath.Join(state.MigrationStagingDir(dir), "sessions.json"), "{}")
		}
		return dir
	}

	for name, tc := range map[string]struct {
		dirs dirs
		want bool
	}{
		"state-tmux/ and a staging dir holding what the migration built": {dirs{tmuxDir: true, staged: true}, true},
		"state-tmux/ and an empty staging dir, everything moved in":      {dirs{tmuxDir: true, staging: true}, false},
		"state-tmux/ alone, a migration landed":                          {dirs{tmuxDir: true}, false},
		"a staging dir alone, a migration before its renames":            {dirs{staged: true}, false},
		"a file named state-tmux beside a staging dir":                   {dirs{tmuxFile: true, staged: true}, false},
		"neither": {dirs{}, false},
	} {
		t.Run("it reads "+name, func(t *testing.T) {
			dir := arrange(t, tc.dirs)

			got, err := state.MigrationCutShort(dir)

			if got != tc.want || err != nil {
				t.Errorf("MigrationCutShort = %v, %v; want %v, nil", got, err, tc.want)
			}
		})
	}

	// No state-tmux/ means no migration reached its renames, so nothing at
	// the staging path is looked at: it would stop every command otherwise.
	for name, block := range map[string]func(t *testing.T, staging string){
		"a file at the staging path": func(t *testing.T, staging string) { mustWrite(t, staging, "not a dir") },
		"a staging dir it can't read": func(t *testing.T, staging string) {
			if os.Geteuid() == 0 {
				t.Skip("root reads a 0000 dir")
			}
			mustMkdir(t, filepath.Join(staging, "sessions"))
			if err := os.Chmod(staging, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(staging, 0o700) })
		},
	} {
		t.Run("it reads "+name+" with no state-tmux/ as no migration", func(t *testing.T) {
			dir := arrange(t, dirs{})
			block(t, state.MigrationStagingDir(dir))

			if got, err := state.MigrationCutShort(dir); got || err != nil {
				t.Errorf("MigrationCutShort = %v, %v; want false, nil", got, err)
			}
		})
	}

	t.Run("it fails on a staging dir it can't read beside state-tmux/", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads a 0000 dir")
		}
		dir := arrange(t, dirs{tmuxDir: true, staged: true})
		staging := state.MigrationStagingDir(dir)
		if err := os.Chmod(staging, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(staging, 0o700) })

		if got, err := state.MigrationCutShort(dir); err == nil {
			t.Errorf("MigrationCutShort = %v, nil; want an error", got)
		}
	})

	t.Run("it reads a state dir beneath a file as no migration", func(t *testing.T) {
		blocker := filepath.Join(t.TempDir(), "not-a-dir")
		mustWrite(t, blocker, "x")

		if got, err := state.MigrationCutShort(filepath.Join(blocker, "state")); got || err != nil {
			t.Errorf("MigrationCutShort = %v, %v; want false, nil", got, err)
		}
	})

	t.Run("it fails on a staging dir it can't look in", func(t *testing.T) {
		dir := arrange(t, dirs{tmuxDir: true})
		mustWrite(t, state.MigrationStagingDir(dir), "not a dir")

		if got, err := state.MigrationCutShort(dir); err == nil {
			t.Errorf("MigrationCutShort = %v, nil; want an error", got)
		}
	})

	t.Run("it fails on a state-tmux it can't look at beside a staging dir holding what was built", func(t *testing.T) {
		dir := arrange(t, dirs{staged: true})
		tmuxDir := state.TmuxStateDir(dir)
		if err := os.Symlink(filepath.Base(tmuxDir), tmuxDir); err != nil { // a loop: every stat of it fails
			t.Fatal(err)
		}

		if got, err := state.MigrationCutShort(dir); err == nil {
			t.Errorf("MigrationCutShort = %v, nil; want an error", got)
		}
	})
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
