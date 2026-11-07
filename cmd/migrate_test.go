package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// migrateWorld is a state dir and hooks.json of the test's own, the zmx
// backend, the tmux version's server and the bootstrap migrate ends with
// staged, each noting it was reached. The bootstrap's restore makes the
// sessions makes names, as zmx's, and claims them, as their helpers do.
type migrateWorld struct {
	dir, hooksFile string
	backend        *backendtest.Fake
	tmux           *fakeMigrateTmux
	bootstraps     int
	makes          []string
	restored       migrate.Restore
	warnings       []bootstrap.Warning
	bootErr        error
	daemon         migrate.Daemon
}

func newMigrateWorld(t *testing.T) *migrateWorld {
	t.Helper()
	w := &migrateWorld{
		dir:       filepath.Join(t.TempDir(), "state"),
		hooksFile: filepath.Join(t.TempDir(), "hooks.json"),
		backend:   backendtest.New(),
		tmux:      &fakeMigrateTmux{answering: true},
	}
	t.Setenv("PORTAL_STATE_DIR", w.dir)
	t.Setenv("PORTAL_HOOKS_FILE", w.hooksFile)
	withBackendDeps(t, BackendDeps{Backend: w.backend})
	withBootstrapDeps(t, BootstrapDeps{Owed: func(context.Context) bool {
		t.Error("the root's bootstrap was asked whether one is owed")
		return false
	}})
	return w
}

// restores has w's bootstrap restore the sessions named.
func (w *migrateWorld) restores(names ...string) *migrateWorld {
	w.makes = names
	w.restored = migrate.Restore{End: migrate.RestoreRan, Restored: len(names)}
	return w
}

// stage stages w's seams for the run.
func (w *migrateWorld) stage(t *testing.T) {
	t.Helper()
	withMigrateDeps(t, MigrateDeps{
		Tmux:   w.tmux,
		Daemon: w.daemon,
		Bootstrap: func(context.Context, io.Writer) (migrate.Restore, []bootstrap.Warning, error) {
			w.bootstraps++
			for _, name := range w.makes {
				w.backend.Add(backend.Session{Name: name})
			}
			if len(w.makes) > 0 {
				claim(t, w.dir, w.makes)
			}
			return w.restored, w.warnings, w.bootErr
		},
	})
}

// claim clears the marks of each session named in the state dir dir's index,
// as a restored session's helper claiming it does.
func claim(t *testing.T, dir string, names []string) {
	t.Helper()
	var idx struct {
		Version  int                  `json:"version"`
		SavedAt  time.Time            `json:"saved_at"`
		Sessions []sessionindex.Entry `json:"sessions"`
	}
	if err := json.Unmarshal(readFileBytes(t, sessionindex.Path(dir)), &idx); err != nil {
		t.Fatal(err)
	}
	for i, e := range idx.Sessions {
		if slices.Contains(names, e.Name) {
			idx.Sessions[i].Owed, idx.Sessions[i].ResumeLazy = false, false
		}
	}
	data, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, sessionindex.Path(dir), string(data))
}

// withTmuxState gives w the tmux version's state, one session of one pane,
// its hook keyed by its token.
func (w *migrateWorld) withTmuxState(t *testing.T) *migrateWorld {
	t.Helper()
	index := fmt.Sprintf(`{"version": 1, "saved_at": "2026-11-04T22:15:03Z", "sessions": [{"name": "lantern", "environment": {},
  "windows": [{"index": 0, "name": "zsh", "layout": "", "zoomed": false, "active": true,
    "panes": [{"index": 0, "cwd": %q, "active": true, "current_command": "zsh", "scrollback_file": "", "portal_pane_id": "tokLan"}]}]}]}`, t.TempDir())
	writeTestFile(t, sessionindex.Path(w.dir), index)
	writeTestFile(t, w.hooksFile, `{"tokLan": {"on-resume": "claude --resume lantern"}}`)
	return w
}

// migrated runs w to a full migration whose bootstrap restores nothing,
// then forgets that it bootstrapped.
func (w *migrateWorld) migrated(t *testing.T) {
	t.Helper()
	w.stage(t)
	if _, _, err := runRootCmd(t, "migrate"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	w.bootstraps = 0
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

type fakeMigrateTmux struct {
	answering bool
	reached   []string
}

func (f *fakeMigrateTmux) Answering(context.Context) bool {
	f.reached = append(f.reached, "answering")
	return f.answering
}

func (f *fakeMigrateTmux) RemoveHooks(context.Context) (int, error) {
	f.reached = append(f.reached, "remove hooks")
	return 9, nil
}

func (f *fakeMigrateTmux) KillSaver(context.Context) (bool, error) {
	f.reached = append(f.reached, "kill saver")
	return true, nil
}

// keptLines are the report's last lines, where the tmux version's state and
// hooks.json's backup are kept beside the state dir dir.
func keptLines(dir string) string {
	return "The tmux version's state is kept at " + state.TmuxStateDir(dir) + ".\n" +
		"hooks.json's backup is at " + migrate.HooksBackup(state.TmuxStateDir(dir)) + ".\n"
}

// lanternLeft is the report's block naming lantern left to restore.
const lanternLeft = "1 session left to restore. x <name> brings one back now; otherwise they come back at the next restore after a reboot:\n" +
	"  lantern\n"

const panelLine = "Each migrated session with a resume hook waits on its resume panel the first time you open it.\n"

func TestMigrateCommand(t *testing.T) {
	t.Run("it migrates a version 1 state past the state check and the root's bootstrap, then bootstraps and reports", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t).restores("lantern")
		w.stage(t)

		stdout, _, err := runRootCmd(t, "migrate")
		if err != nil {
			t.Fatalf("migrate: %v", err)
		}

		if w.bootstraps != 1 {
			t.Errorf("bootstrapped %d times, want once", w.bootstraps)
		}
		if got := strings.Join(w.tmux.reached, ", "); got != "answering, remove hooks, kill saver" {
			t.Errorf("reached tmux for %s, want its hooks and saver", got)
		}
		idx, err := sessionindex.Read(w.dir)
		if err != nil {
			t.Fatalf("the state dir isn't this version's: %v", err)
		}
		if _, ok := idx.Lookup("lantern"); !ok {
			t.Error("lantern has no entry, want the one migrated, its marks claimed by the restore")
		}
		if held := string(readFileBytes(t, w.hooksFile)); !strings.Contains(held, `"lantern"`) || strings.Contains(held, "tokLan") {
			t.Errorf("hooks.json = %s, want the hook moved to lantern", held)
		}
		want := "Migrated 1 tmux session to 1 session.\n" +
			"Moved 1 resume hook.\n" +
			"Restored 1 session. Every saved session is restored.\n" +
			panelLine +
			"\n" +
			"Sessions (tmux name → name):\n" +
			"  lantern → lantern\n" +
			"\n" +
			"Resume hooks moved:\n" +
			"  tokLan → lantern\n" +
			"\n" +
			keptLines(w.dir)
		if stdout.String() != want {
			t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
		}
	})

	t.Run("it names a session the restore didn't make as left, and what brings it back", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t).restores()
		w.stage(t)

		stdout, _, err := runRootCmd(t, "migrate")
		if err != nil {
			t.Fatalf("migrate: %v", err)
		}

		want := "Restored no sessions.\n" + panelLine + "\n" + lanternLeft + "\nSessions (tmux name → name):\n"
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout:\n%s\nwant it holding:\n%s", stdout, want)
		}
	})

	t.Run("it finds nothing to migrate, bootstrapping nothing, and exits 0", func(t *testing.T) {
		w := newMigrateWorld(t)
		w.stage(t)

		stdout, _, err := runRootCmd(t, "migrate")

		if err != nil || stdout.String() != "Nothing to migrate: there's no saved state at "+w.dir+".\n" {
			t.Errorf("migrate = %q, %v; want nothing to migrate, exit 0", stdout, err)
		}
		if w.bootstraps != 0 || len(w.tmux.reached) != 0 {
			t.Errorf("bootstrapped %d times, reached tmux for %v; want neither", w.bootstraps, w.tmux.reached)
		}
	})

	t.Run("it fails with the refusal, reporting and bootstrapping nothing", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t)
		if err := os.MkdirAll(state.TmuxStateDir(w.dir), 0o700); err != nil {
			t.Fatal(err)
		}
		w.stage(t)

		stdout, _, err := runRootCmd(t, "migrate")

		if !errors.Is(err, migrate.ErrBothStates) || stdout.Len() != 0 || w.bootstraps != 0 {
			t.Errorf("migrate = %q, %v after %d bootstraps; want ErrBothStates alone", stdout, err, w.bootstraps)
		}
	})

	t.Run("it finishes a migration cut short between its renames, then bootstraps and reports", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t)
		w.migrated(t)
		if err := os.Rename(w.dir, state.MigrationStagingDir(w.dir)); err != nil {
			t.Fatal(err)
		}
		w.restores("lantern")

		stdout, _, err := runRootCmd(t, "migrate")
		if err != nil {
			t.Fatalf("migrate: %v", err)
		}

		if w.bootstraps != 1 {
			t.Errorf("bootstrapped %d times, want once", w.bootstraps)
		}
		want := "Finished a migration cut short: the migrated state is in place at " + w.dir + ".\n" +
			"Restored 1 session. Every saved session is restored.\n" +
			panelLine +
			"\n" +
			keptLines(w.dir)
		if stdout.String() != want {
			t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
		}
	})

	t.Run("it reports a re-run owed no bootstrap from the index: the sessions left, and what brings them back", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t)
		w.migrated(t)

		stdout, _, err := runRootCmd(t, "migrate")
		if err != nil {
			t.Fatalf("migrate: %v", err)
		}

		want := "Already migrated.\n" + panelLine + "\n" + lanternLeft + "\n" + keptLines(w.dir)
		if stdout.String() != want {
			t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
		}
	})

	t.Run("it reports a re-run with every session restored, and none waiting", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t)
		w.migrated(t)
		w.backend.Add(backend.Session{Name: "lantern"})
		// As the helper's claim leaves it: owed nothing, resuming as its hook says.
		writeTestFile(t, sessionindex.Path(w.dir), `{"version": 2, "saved_at": "2026-11-05T09:00:00Z", "sessions": [{"name": "lantern", "dir": "/srv/lantern"}]}`)

		stdout, _, err := runRootCmd(t, "migrate")
		if err != nil {
			t.Fatalf("migrate: %v", err)
		}

		if want := "Already migrated.\nEvery saved session is restored.\n\n" + keptLines(w.dir); stdout.String() != want {
			t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
		}
	})

	t.Run("it tells of the wait for the tmux version's daemon on stderr", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t)
		if err := state.WritePIDFile(w.dir, 4242); err != nil {
			t.Fatal(err)
		}
		reads := 0
		w.daemon = migrate.Daemon{
			Identify: func(int) (state.IdentifyResult, error) {
				reads++
				if reads < 3 {
					return state.IdentifyIsPortalDaemon, nil
				}
				return state.IdentifyDead, nil
			},
			Sleep: func(context.Context, time.Duration) error { return nil },
		}
		w.stage(t)

		_, stderr, err := runRootCmd(t, "migrate")

		if err != nil || stderr.String() != "Waiting for the tmux version's save daemon (pid 4242) to save and exit…\n" {
			t.Errorf("migrate = %v, stderr %q; want the wait told once", err, stderr)
		}
	})

	t.Run("it reports, then fails with the bootstrap's fatal", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t)
		w.restored, w.bootErr = migrate.Restore{End: migrate.RestoreStopped}, errors.New("zmx not found on PATH")
		w.stage(t)

		stdout, _, err := runRootCmd(t, "migrate")

		if !errors.Is(err, w.bootErr) {
			t.Errorf("migrate = %v, want the bootstrap's fatal", err)
		}
		want := "Couldn't restore the sessions: the error below says why.\n" + panelLine + "\n" +
			"1 session left to restore. Once the error below is fixed, x <name> brings one back; otherwise they come back at the next restore after a reboot:\n" +
			"  lantern\n"
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout:\n%s\nwant the report holding:\n%s", stdout, want)
		}
	})

	t.Run("it shows the bootstrap's warnings on stderr", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t)
		w.warnings = []bootstrap.Warning{bootstrap.CorruptSessionsJSONWarning()}
		w.stage(t)

		_, stderr, err := runRootCmd(t, "migrate")

		if err != nil || !strings.HasPrefix(stderr.String(), "Portal state file unusable — restoration skipped.\n") {
			t.Errorf("migrate = %v, stderr %q; want the warning", err, stderr)
		}
	})

	t.Run("it words and logs a failure before the run as a run stopped", func(t *testing.T) {
		newMigrateWorld(t).stage(t)
		t.Setenv("PORTAL_STATE_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		sink := logtest.Install(t)

		stdout, _, err := runRootCmd(t, "migrate")

		if err == nil || !strings.HasPrefix(err.Error(), "portal migrate stopped: ") || !strings.HasSuffix(err.Error(), ". Once that's fixed, run portal migrate again.") {
			t.Fatalf("migrate = %v; want the failure worded as a run stopped", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want no report", stdout)
		}
		logged := sink.Records().Matching("migrate", "stopped").AtExactLevel(slog.LevelError).Only(t, "the failure")
		if logged.ErrorAttr(t, "error").Error() != err.Error() {
			t.Errorf("logged %v, want the failure returned", logged)
		}
	})

	t.Run("it is exempt from the state check and from the root's bootstrap", func(t *testing.T) {
		if !stateCheckExempt["migrate"] || !bootstrapExempt["migrate"] {
			t.Errorf("state check exempt %v, bootstrap exempt %v; want both", stateCheckExempt["migrate"], bootstrapExempt["migrate"])
		}
	})
}

func TestRestoreOf(t *testing.T) {
	fatal := errors.New("fatal")
	for name, tc := range map[string]struct {
		res  *restore.Result
		err  error
		want migrate.Restore
	}{
		"a restore made, some failing":     {&restore.Result{Restored: 3, Failed: 1}, nil, migrate.Restore{End: migrate.RestoreRan, Restored: 3}},
		"a restore stopped by a fatal":     {&restore.Result{Restored: 2, Unsupported: fatal}, fatal, migrate.Restore{End: migrate.RestoreStopped, Restored: 2}},
		"no restore owed under the lock":   {nil, nil, migrate.Restore{}},
		"a fatal before the restore":       {nil, fatal, migrate.Restore{End: migrate.RestoreStopped}},
		"a sessions.json it couldn't read": {&restore.Result{IndexErr: errors.New("corrupt")}, nil, migrate.Restore{End: migrate.RestoreUnread}},
		"zmx's sessions it couldn't list":  {&restore.Result{ListErr: errors.New("refused")}, nil, migrate.Restore{End: migrate.RestoreUnread}},
	} {
		t.Run(name, func(t *testing.T) {
			got := restoreOf(tc.res, tc.err)
			if got.End != tc.want.End || got.Restored != tc.want.Restored {
				t.Errorf("restoreOf = %+v, want %+v", got, tc.want)
			}
		})
	}
}
