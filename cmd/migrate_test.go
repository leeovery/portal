package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// migrateWorld is a state dir and hooks.json of the test's own, the zmx
// backend, the tmux version's server and the bootstrap migrate ends with
// staged, each noting it was reached.
type migrateWorld struct {
	dir, hooksFile string
	tmux           *fakeMigrateTmux
	bootstraps     int
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
		tmux:      &fakeMigrateTmux{answering: true},
		restored:  migrate.Restore{Ran: true, Restored: 1},
	}
	t.Setenv("PORTAL_STATE_DIR", w.dir)
	t.Setenv("PORTAL_HOOKS_FILE", w.hooksFile)
	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
	withBootstrapDeps(t, BootstrapDeps{Owed: func(context.Context) bool {
		t.Error("the root's bootstrap was asked whether one is owed")
		return false
	}})
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
			return w.restored, w.warnings, w.bootErr
		},
	})
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

func TestMigrateCommand(t *testing.T) {
	t.Run("it migrates a version 1 state past the state check and the root's bootstrap, then bootstraps and reports", func(t *testing.T) {
		w := newMigrateWorld(t).withTmuxState(t)
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
		if e, ok := idx.Lookup("lantern"); !ok || !e.ResumeLazy {
			t.Errorf("lantern's entry %+v (held: %v); want it, resuming lazily", e, ok)
		}
		if held := string(readFileBytes(t, w.hooksFile)); !strings.Contains(held, `"lantern"`) || strings.Contains(held, "tokLan") {
			t.Errorf("hooks.json = %s, want the hook moved to lantern", held)
		}
		want := "Migrated 1 tmux session to 1 session.\n" +
			"Moved 1 resume hook.\n" +
			"Restored 1 session.\n" +
			"Each one with a resume hook waits on its resume panel the first time you open it.\n" +
			"\n" +
			"Sessions (name ← tmux name):\n" +
			"  lantern ← lantern\n" +
			"\n" +
			"Resume hooks moved:\n" +
			"  tokLan → lantern\n" +
			"\n" +
			"The tmux version's state is kept at " + state.TmuxStateDir(w.dir) + ".\n" +
			"hooks.json's backup is at " + migrate.HooksBackup(state.TmuxStateDir(w.dir)) + ".\n"
		if stdout.String() != want {
			t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
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
		w.restored, w.bootErr = migrate.Restore{Unfinished: true}, errors.New("zmx not found on PATH")
		w.stage(t)

		stdout, _, err := runRootCmd(t, "migrate")

		if !errors.Is(err, w.bootErr) {
			t.Errorf("migrate = %v, want the bootstrap's fatal", err)
		}
		if !strings.Contains(stdout.String(), "Couldn't restore the sessions yet; the next portal command tries again.\n") {
			t.Errorf("stdout:\n%s\nwant the report, its restore unfinished", stdout)
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
		"a restore made":                   {&restore.Result{Restored: 3, Failed: 1}, nil, migrate.Restore{Ran: true, Restored: 3, Failed: 1}},
		"a restore made before a fatal":    {&restore.Result{Restored: 2}, fatal, migrate.Restore{Ran: true, Restored: 2}},
		"no restore owed under the lock":   {nil, nil, migrate.Restore{}},
		"a fatal before the restore":       {nil, fatal, migrate.Restore{Unfinished: true}},
		"a sessions.json it couldn't read": {&restore.Result{IndexErr: errors.New("corrupt")}, nil, migrate.Restore{Unfinished: true}},
		"zmx's sessions it couldn't list":  {&restore.Result{ListErr: errors.New("refused")}, nil, migrate.Restore{Unfinished: true}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := restoreOf(tc.res, tc.err); got != tc.want {
				t.Errorf("restoreOf = %+v, want %+v", got, tc.want)
			}
		})
	}
}
