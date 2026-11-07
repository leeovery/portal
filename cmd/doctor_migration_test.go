package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// Between portal migrate's renames the state dir holds no index of the
// migrated sessions, so doctor reports it and repairs nothing: a prune would
// judge every migrated hook stale.
func TestDoctorWhileAMigrationIsCutShort(t *testing.T) {
	// unknowable makes the staging dir a file, so whether a migration is
	// cut short can't be told.
	unknowable := func(t *testing.T, staging string) {
		t.Helper()
		if err := os.RemoveAll(staging); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(staging, []byte("not a dir"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("it fails sessions.json, saying where the migrated state waits", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		staging := cutShortBeside(t, dir)

		got := findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: dir})), "sessions.json")

		assertCheck(t, got, checkFail, "a migration was cut short, its state waiting in "+staging+"; run portal migrate to finish it")
	})

	t.Run("it fails sessions.json when it can't tell, saying why", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		unknowable(t, cutShortBeside(t, dir))

		got := findCheck(t, diagnose(t, withHealthyRuntime(t, &DoctorDeps{StateDir: dir})), "sessions.json")

		if got.status != checkFail || !strings.HasPrefix(got.detail, "can't tell whether a migration was cut short: ") {
			t.Errorf("sessions.json = %v %q, want a failure saying it can't tell", got.status, got.detail)
		}
	})

	for name, tc := range map[string]struct {
		arrange func(t *testing.T, staging string)
		line    string
	}{
		"cut short":       {func(*testing.T, string) {}, "Skipped every repair: a migration was cut short; run portal migrate to finish it"},
		"maybe cut short": {unknowable, "Skipped every repair: can't tell whether a migration was cut short"},
	} {
		t.Run("--fix repairs nothing with a migration "+name+", and exits non-zero", func(t *testing.T) {
			in := newStaleHooksInstall(t, hookstest.StaleHookSeed)
			projects, projectsFile := seedProjectsJSON(t, filepath.Join(t.TempDir(), "gone"))
			in.deps.ProjectStore = projects
			in.deps.StateDir = filepath.Join(t.TempDir(), "state")
			tc.arrange(t, cutShortBeside(t, in.deps.StateDir))
			hooksBefore, projectsBefore := readFileBytes(t, in.hooksPath), readFileBytes(t, projectsFile)

			outBuf, _, err := runDoctorWith(t, in.deps, "--fix")

			if !errors.Is(err, ErrDoctorUnhealthy) {
				t.Errorf("Execute err = %v, want ErrDoctorUnhealthy", err)
			}
			out := outBuf.String()
			if !strings.Contains(out, "  ✗ sessions.json: ") || !strings.HasSuffix(out, "\n"+tc.line+"\n") || strings.Count(out, "Portal doctor:") != 1 {
				t.Errorf("output = %q, want one diagnosis naming sessions.json, then %q", out, tc.line)
			}
			assertHooksFileUnchanged(t, in.hooksPath, hooksBefore)
			if got := readFileBytes(t, projectsFile); !bytes.Equal(got, projectsBefore) {
				t.Errorf("projects.json = %s, want %s untouched", got, projectsBefore)
			}
			if _, err := os.Stat(sessionindex.Path(in.deps.StateDir)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("sessions.json in the state dir: %v; want none written", err)
			}
		})
	}

	t.Run("--fix repairs once the migration is finished", func(t *testing.T) {
		in := newStaleHooksInstall(t, hookstest.StaleHookSeed)
		staging := cutShortBeside(t, in.deps.StateDir)
		if err := os.Remove(sessionindex.Path(staging)); err != nil {
			t.Fatal(err)
		}

		outBuf, _, err := runDoctorWith(t, in.deps, "--fix")

		if err != nil || !strings.Contains(outBuf.String(), "Pruned 1 stale hook:") {
			t.Errorf("Execute = %v, output %q; want the prune made", err, outBuf.String())
		}
		if _, err := os.Stat(state.TmuxStateDir(in.deps.StateDir)); err != nil {
			t.Errorf("state-tmux/: %v; want it left", err)
		}
	})
}
