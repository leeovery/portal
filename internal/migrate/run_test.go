package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

// The tmux version's state the run fixtures migrate: lantern, one pane with
// a token and its scrollback, and tidepool, two panes, the first with its
// scrollback; its daemon's pid; and hooks.json, holding lantern's pane's hook
// by its token and a hook by a pane address.
const (
	lanternScrollback  = "scrollback/pane-tokLan.bin"
	tidepoolScrollback = "scrollback/tidepool__0.0.bin"
	runHooks           = `{
  "tokLan": {"on-resume": "claude --resume lantern"},
  "tidepool:0.1": {"on-resume": "make watch"}
}`
)

func runSessions() []v1Session {
	return []v1Session{
		session("lantern", pane(0, codeDir+"lantern").token("tokLan").scrollback(lanternScrollback)),
		session("tidepool", pane(0, codeDir+"tidepool").scrollback(tidepoolScrollback), pane(1, codeDir+"tidepool/web")),
	}
}

var runFiles = map[string]string{
	lanternScrollback:  "lantern transcript\n",
	tidepoolScrollback: "tidepool transcript\n",
	"daemon.pid":       pidFile,
}

// runWorld is a state dir in a root of its own, hooks.json beside it, and
// fakes for the tmux version's server and daemon and for zmx's listing, each
// noting what was on disk when it was reached.
type runWorld struct {
	t                                      *testing.T
	root, dir, tmuxDir, staging, hooksFile string
	tmux                                   *fakeTmux
	clock                                  *fakeClock
	daemon                                 func(elapsed time.Duration) reading
	live                                   []string
	liveErr                                error
	steps                                  []step
	told                                   []int
}

// step is a fake reached, and whether hooks.json's backup and the staging
// dir were there as it was.
type step struct {
	name             string
	backedUp, staged bool
}

func newRunWorld(t *testing.T) *runWorld {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "state")
	w := &runWorld{
		t:         t,
		root:      root,
		dir:       dir,
		tmuxDir:   state.TmuxStateDir(dir),
		staging:   state.MigrationStagingDir(dir),
		hooksFile: filepath.Join(root, "hooks.json"),
		clock:     newFakeClock(),
		daemon:    always(readDead),
	}
	w.tmux = &fakeTmux{w: w, answering: true, saver: true, unset: 9}
	return w
}

// withTmuxState gives w the tmux version's state dir and hooks.json.
func (w *runWorld) withTmuxState() *runWorld {
	w.t.Helper()
	legacyAt(w.t, w.dir, runSessions(), runFiles)
	writeFile(w.t, w.hooksFile, runHooks)
	return w
}

func (w *runWorld) note(name string) {
	w.steps = append(w.steps, step{name: name, backedUp: present(w.t, migrate.HooksBackup(w.dir)), staged: present(w.t, w.staging)})
}

func (w *runWorld) config() migrate.Config {
	return migrate.Config{
		StateDir:  w.dir,
		HooksFile: w.hooksFile,
		Tmux:      w.tmux,
		Daemon: migrate.Daemon{
			Identify: func(int) (state.IdentifyResult, error) {
				w.note("identify")
				r := w.daemon(w.clock.elapsed())
				return r.result, r.err
			},
			Sleep: w.clock.Sleep,
			Now:   w.clock.Now,
		},
		Live: func(context.Context) ([]string, error) {
			w.note("live")
			return w.live, w.liveErr
		},
		Naming:  naming(w.t),
		Waiting: func(pid int) { w.told = append(w.told, pid) },
	}
}

func (w *runWorld) run() (migrate.Outcome, error) {
	return migrate.Run(context.Background(), w.config())
}

// migrated runs w to a full migration, failing the test on anything else,
// and forgets the steps it took.
func (w *runWorld) migrated() migrate.Outcome {
	w.t.Helper()
	o, err := w.run()
	if err != nil || o.Course != migrate.Migrated {
		w.t.Fatalf("Run = %v, %v; want a full migration", o.Course, err)
	}
	w.steps = nil
	return o
}

// cutShort leaves w as a migration cut short between its renames leaves it:
// the state dir moved to state-tmux/, the zmx version's still in staging.
func (w *runWorld) cutShort() {
	w.t.Helper()
	w.migrated()
	if err := os.Rename(w.dir, w.staging); err != nil {
		w.t.Fatalf("move the migrated state back to staging: %v", err)
	}
}

// stepNames is the name of each step, a run of the same step named once.
func (w *runWorld) stepNames() []string {
	var names []string
	for _, s := range w.steps {
		if len(names) == 0 || names[len(names)-1] != s.name {
			names = append(names, s.name)
		}
	}
	return names
}

type fakeTmux struct {
	w                  *runWorld
	answering, saver   bool
	unset              int
	hooksErr, saverErr error
}

func (f *fakeTmux) Answering(context.Context) bool {
	f.w.note("answering")
	return f.answering
}

func (f *fakeTmux) RemoveHooks(context.Context) (int, error) {
	f.w.note("remove hooks")
	return f.unset, f.hooksErr
}

func (f *fakeTmux) KillSaver(context.Context) (bool, error) {
	f.w.note("kill saver")
	return f.saver, f.saverErr
}

func present(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("look for %s: %v", path, err)
	}
	return err == nil
}

// assertUnchanged fails unless w's root holds the tree before held.
func assertUnchanged(t *testing.T, w *runWorld, before map[string]string, why string) {
	t.Helper()
	if after := w.tree(); !maps.Equal(before, after) {
		t.Errorf("%s: the tree changed\nbefore: %v\nafter:  %v", why, before, after)
	}
}

// tree is everything under w's root, by its path relative to it: a file's
// content, a dir as "dir", a link as "-> " and its target.
func (w *runWorld) tree() map[string]string {
	w.t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(w.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == w.root {
			return err
		}
		rel, _ := filepath.Rel(w.root, path)
		switch {
		case d.IsDir():
			tree[rel] = "dir"
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			tree[rel] = "-> " + target
			return err
		default:
			data, err := os.ReadFile(path)
			tree[rel] = string(data)
			return err
		}
		return nil
	})
	if err != nil {
		w.t.Fatalf("walk %s: %v", w.root, err)
	}
	return tree
}

// assertMigrated fails unless w holds the full migration o: the tmux
// version's state in state-tmux/ byte for byte, with hooks.json's backup
// holding backup; the zmx version's state in the state dir, one entry for
// each planned session, each owed a restore and resuming lazily; hooks.json
// rewritten from backup by the plan; and no staging dir.
func assertMigrated(t *testing.T, w *runWorld, o migrate.Outcome, tmuxState map[string][]byte, backup string) {
	t.Helper()
	if o.Course != migrate.Migrated || o.StateDir != w.dir || o.TmuxDir != w.tmuxDir || o.Backup != migrate.HooksBackup(w.tmuxDir) {
		t.Errorf("Outcome = %v, %s, %s, %s; want Migrated, the state dir, state-tmux/ and the backup in it", o.Course, o.StateDir, o.TmuxDir, o.Backup)
	}
	kept := dirFiles(t, w.tmuxDir)
	for name, data := range tmuxState {
		if !bytes.Equal(kept[name], data) {
			t.Errorf("state-tmux/%s = %q, want the tmux version's %q", name, kept[name], data)
		}
	}
	if got := string(kept[filepath.Base(migrate.HooksBackup(w.tmuxDir))]); got != backup {
		t.Errorf("hooks.json's backup holds %q, want %q", got, backup)
	}
	idx := readIndex(t, w.dir)
	var names []string
	for _, e := range idx.Entries() {
		names = append(names, e.Name)
		if !e.Owed || !e.ResumeLazy {
			t.Errorf("entry %s: owed %v, resume lazily %v; want both", e.Name, e.Owed, e.ResumeLazy)
		}
	}
	if !slices.Equal(names, namesOf(o.Plan)) {
		t.Errorf("entries = %v, want the plan's %v", names, namesOf(o.Plan))
	}
	held := rawHooks(t, w.hooksFile)
	var wantKeys []string
	for _, m := range o.Plan.Moves {
		wantKeys = append(wantKeys, m.To)
	}
	if got := slices.Sorted(maps.Keys(held)); !slices.Equal(got, slices.Sorted(slices.Values(wantKeys))) {
		t.Errorf("hooks.json keys = %v, want the moved %v", got, wantKeys)
	}
	if present(t, w.staging) {
		t.Error("the staging dir is still there")
	}
}

func TestRun_MigratesInFull(t *testing.T) {
	t.Run("it stops tmux, waits out its daemon, backs up, plans, builds, rewrites and swaps, in that order", func(t *testing.T) {
		sink := logtest.Install(t)
		w := newRunWorld(t).withTmuxState()
		w.daemon = until(time.Second, readRunning, readDead)
		tmuxState := dirFiles(t, w.dir)

		o, err := w.run()
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if got, want := w.stepNames(), []string{"answering", "remove hooks", "kill saver", "identify", "live"}; !slices.Equal(got, want) {
			t.Fatalf("steps = %v, want %v", got, want)
		}
		for _, s := range w.steps {
			wantBackup := s.name == "live"
			if s.backedUp != wantBackup || s.staged {
				t.Errorf("at %s: backed up %v, staged %v; want backed up %v and nothing staged", s.name, s.backedUp, s.staged, wantBackup)
			}
		}
		if len(w.told) != 1 || w.told[0] != daemonPID {
			t.Errorf("told %v of the wait; want the daemon's pid once", w.told)
		}
		assertMigrated(t, w, o, tmuxState, runHooks)
		removals := map[string]migrate.HookKeyKind{}
		for _, r := range o.Plan.Removals {
			removals[r.Key] = r.Kind
		}
		if len(o.Plan.Moves) != 1 || o.Plan.Moves[0].From != "tokLan" || removals["tidepool:0.1"] != migrate.PaneAddressKey {
			t.Errorf("moves %v, removals %v; want tokLan moved and the pane address removed", o.Plan.Moves, o.Plan.Removals)
		}
		done := sink.Records().Matching("migrate", "migrated").Only(t, "the run's summary")
		if done.IntAttr(t, "sessions") != 3 || done.AttrString(t, "path") != w.tmuxDir {
			t.Errorf("summary = %v; want 3 sessions and state-tmux/", done)
		}
		for _, msg := range []string{"tmux hooks removed", "tmux saver killed", "tmux daemon exited", "hooks backed up", "state built"} {
			sink.Records().Matching("migrate", msg).AtExactLevel(slog.LevelInfo).Only(t, msg)
		}
	})

	t.Run("it names no session as one zmx holds", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.live = []string{"lantern"}

		o := w.migrated()

		lantern := o.Plan.Sessions[0]
		if lantern.From.Session != "lantern" || lantern.Name == "lantern" || lantern.Generated == nil {
			t.Errorf("lantern's pane became %s (generated: %v); want a generated name clear of zmx's", lantern.Name, lantern.Generated)
		}
	})

	t.Run("it migrates with no tmux server answering, its hooks and saver gone with it", func(t *testing.T) {
		sink := logtest.Install(t)
		w := newRunWorld(t).withTmuxState()
		w.tmux.answering = false
		tmuxState := dirFiles(t, w.dir)

		o, err := w.run()
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if got, want := w.stepNames(), []string{"answering", "identify", "live"}; !slices.Equal(got, want) {
			t.Errorf("steps = %v, want %v", got, want)
		}
		assertMigrated(t, w, o, tmuxState, runHooks)
		sink.Records().Matching("migrate", "no tmux server").Only(t, "no tmux server")
	})

	t.Run("it migrates with no hooks.json, keeping no backup", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		if err := os.Remove(w.hooksFile); err != nil {
			t.Fatal(err)
		}

		o := w.migrated()

		if o.Backup != "" || present(t, migrate.HooksBackup(w.tmuxDir)) || present(t, w.hooksFile) {
			t.Errorf("backup %q; want none kept and no hooks.json written", o.Backup)
		}
	})

	t.Run("it never overwrites a backup a run cut short kept", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		writeFile(t, migrate.HooksBackup(w.dir), runHooks)
		writeFile(t, w.hooksFile, `{"lantern": {"on-resume": "claude --resume lantern"}}`)
		tmuxState := dirFiles(t, w.dir)

		o := w.migrated()

		assertMigrated(t, w, o, tmuxState, runHooks)
	})

	t.Run("it builds afresh over a staging dir a run cut short left", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		writeFile(t, filepath.Join(w.staging, "left-over"), "from a run cut short")
		writeFile(t, sessionindex.Path(w.staging), `{"version": 2, "sessions": []}`)

		o := w.migrated()

		if present(t, filepath.Join(w.dir, "left-over")) || readIndex(t, w.dir).Len() != len(o.Plan.Sessions) {
			t.Error("the state dir holds what the staging dir held before the run")
		}
	})

	t.Run("it lands the swap when the parent dir won't sync, warning after each rename", func(t *testing.T) {
		sink := logtest.Install(t)
		fileutil.FailDirSyncForTest(t, syscall.EIO)
		w := newRunWorld(t).withTmuxState()

		w.migrated()

		var paths []string
		for _, r := range sink.Records().Matching("migrate", "unsynced") {
			paths = append(paths, r.AttrString(t, "path"))
		}
		if want := []string{w.dir, w.tmuxDir}; !slices.Equal(slices.Sorted(slices.Values(paths)), want) {
			t.Errorf("unsynced warnings for %v, want one for each rename, %v", paths, want)
		}
	})
}

func TestRun_RefusesWritingNothing(t *testing.T) {
	stoppedBeforeTheBackup := []string{"answering", "remove hooks", "kill saver", "identify"}
	for _, tc := range []struct {
		name    string
		arrange func(w *runWorld)
		is      error
		says    string
		steps   []string
	}{
		{
			name:    "Portal's tmux hooks can't be removed",
			arrange: func(w *runWorld) { w.tmux.hooksErr = errors.New("tmux show-hooks -g pane-focus-out: exit status 1") },
			says:    "Portal couldn't remove its tmux hooks, so nothing was migrated: tmux show-hooks -g pane-focus-out: exit status 1. Run portal migrate again.",
			steps:   []string{"answering", "remove hooks"},
		},
		{
			name:    "the saver can't be killed",
			arrange: func(w *runWorld) { w.tmux.saverErr = errors.New("tmux kill-session: exit status 1") },
			says:    "Portal couldn't stop the tmux version's saver (_portal-saver), so nothing was migrated: tmux kill-session: exit status 1. Run portal migrate again.",
			steps:   []string{"answering", "remove hooks", "kill saver"},
		},
		{
			name:    "the daemon is still running at the bound",
			arrange: func(w *runWorld) { w.daemon = always(readRunning) },
			is:      migrate.ErrDaemonRunning,
			steps:   stoppedBeforeTheBackup,
		},
		{
			name:    "hooks.json doesn't parse",
			arrange: func(w *runWorld) { writeFile(w.t, w.hooksFile, `{"tokLan": `) },
			is:      hooks.ErrMalformed,
			says:    "Portal can't migrate: hooks.json at ",
			steps:   stoppedBeforeTheBackup,
		},
	} {
		t.Run("it stops when "+tc.name, func(t *testing.T) {
			sink := logtest.Install(t)
			w := newRunWorld(t).withTmuxState()
			tc.arrange(w)
			before := w.tree()

			_, err := w.run()

			if err == nil || tc.is != nil && !errors.Is(err, tc.is) || !strings.HasPrefix(err.Error(), tc.says) {
				t.Fatalf("Run = %v; want a refusal matching %v, saying %q", err, tc.is, tc.says)
			}
			if got := w.stepNames(); !slices.Equal(got, tc.steps) {
				t.Errorf("steps = %v, want %v", got, tc.steps)
			}
			assertUnchanged(t, w, before, "refused")
			refusal := sink.Records().Matching("migrate", "stopped").AtExactLevel(slog.LevelError).Only(t, "the refusal")
			if logged := refusal.ErrorAttr(t, "error"); logged.Error() != err.Error() {
				t.Errorf("logged %v, want the refusal returned, %v", logged, err)
			}
		})
	}

	t.Run("it stops before building when zmx's sessions can't be listed", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.liveErr = errors.New("zmx: permission denied")
		hooksBefore := readBytes(t, w.hooksFile)
		indexBefore := readBytes(t, sessionindex.Path(w.dir))

		_, err := w.run()

		if want := "Portal can't migrate: zmx's sessions can't be listed, so nothing was migrated: zmx: permission denied"; err == nil || err.Error() != want {
			t.Fatalf("Run = %v; want %q", err, want)
		}
		assertNotSwapped(t, w, hooksBefore, indexBefore)
		if present(t, w.staging) {
			t.Error("a staging dir was made")
		}
	})

	t.Run("it rewrites no hooks.json and swaps nothing when the build fails, worded as a run stopped", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		unreadable := filepath.Join(w.dir, lanternScrollback)
		if err := os.Chmod(unreadable, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })
		hooksBefore := readBytes(t, w.hooksFile)
		indexBefore := readBytes(t, sessionindex.Path(w.dir))

		_, err := w.run()

		if err == nil || !errors.Is(err, os.ErrPermission) ||
			!strings.HasPrefix(err.Error(), "portal migrate stopped: ") ||
			!strings.HasSuffix(err.Error(), ". Once that's fixed, run portal migrate again: it picks up where this run stopped.") {
			t.Fatalf("Run = %v; want the build's failure, worded as a run stopped", err)
		}
		assertNotSwapped(t, w, hooksBefore, indexBefore)
	})

	t.Run("it swaps nothing when hooks.json can't be rewritten, and a re-run then migrates", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 60*time.Millisecond)
		w := newRunWorld(t).withTmuxState()
		release := hookstest.HoldHooksSidecar(t, w.hooksFile)
		hooksBefore := readBytes(t, w.hooksFile)
		indexBefore := readBytes(t, sessionindex.Path(w.dir))
		tmuxState := dirFiles(t, w.dir)

		if _, err := w.run(); err == nil {
			t.Fatal("Run succeeded without rewriting hooks.json")
		}
		assertNotSwapped(t, w, hooksBefore, indexBefore)

		release()
		o, err := w.run()
		if err != nil {
			t.Fatalf("the re-run: %v", err)
		}
		assertMigrated(t, w, o, tmuxState, runHooks)
	})
}

// assertNotSwapped fails unless the state dir is still the tmux version's,
// sessions.json and hooks.json as they were, with no state-tmux/.
func assertNotSwapped(t *testing.T, w *runWorld, hooksBefore, indexBefore []byte) {
	t.Helper()
	hookstest.AssertHooksFileUnchanged(t, w.hooksFile, hooksBefore, "before the swap")
	if got := readBytes(t, sessionindex.Path(w.dir)); !bytes.Equal(got, indexBefore) {
		t.Errorf("sessions.json = %s, want the tmux version's still", got)
	}
	if present(t, w.tmuxDir) {
		t.Error("state-tmux/ was made")
	}
}

func TestRun_ReRuns(t *testing.T) {
	t.Run("it finishes a run cut short between its renames", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.cutShort()
		staged := dirFiles(t, w.staging)
		hooksBefore := readBytes(t, w.hooksFile)

		o, err := w.run()

		if err != nil || o.Course != migrate.Finished || o.Backup != migrate.HooksBackup(w.tmuxDir) {
			t.Fatalf("Run = %v (backup %q), %v; want Finished, with the backup", o.Course, o.Backup, err)
		}
		if got := dirFiles(t, w.dir); !maps.EqualFunc(got, staged, bytes.Equal) {
			t.Errorf("the state dir holds %v, want what staging held, %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(staged)))
		}
		if present(t, w.staging) || len(w.steps) != 0 {
			t.Errorf("staging still there: %v; steps %v; want it gone, with tmux, the daemon and zmx unreached", present(t, w.staging), w.steps)
		}
		hookstest.AssertHooksFileUnchanged(t, w.hooksFile, hooksBefore, "finishing the renames")
	})

	t.Run("it finishes into a state dir Portal's log made since, keeping the logs", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.cutShort()
		staged := dirFiles(t, w.staging)
		writeFile(t, filepath.Join(w.dir, "portal.log.2026-11-05"), "a log line\n")
		if err := os.Symlink("portal.log.2026-11-05", state.PortalLog(w.dir)); err != nil {
			t.Fatal(err)
		}

		o, err := w.run()

		if err != nil || o.Course != migrate.Finished {
			t.Fatalf("Run = %v, %v; want Finished", o.Course, err)
		}
		got := dirFiles(t, w.dir)
		for name, data := range staged {
			if !bytes.Equal(got[name], data) {
				t.Errorf("%s = %q, want staging's %q", name, got[name], data)
			}
		}
		if string(got["portal.log.2026-11-05"]) != "a log line\n" || !present(t, state.PortalLog(w.dir)) || present(t, w.staging) {
			t.Error("the logs weren't kept, or staging is still there")
		}
	})

	t.Run("it finishes a move into the state dir cut short", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.cutShort()
		staged := dirFiles(t, w.staging)
		writeFile(t, filepath.Join(w.dir, "portal.log.2026-11-05"), "a log line\n")
		if err := os.Rename(state.ScrollbackDir(w.staging), state.ScrollbackDir(w.dir)); err != nil {
			t.Fatal(err)
		}

		o, err := w.run()

		if err != nil || o.Course != migrate.Finished {
			t.Fatalf("Run = %v, %v; want Finished", o.Course, err)
		}
		got := dirFiles(t, w.dir)
		for name, data := range staged {
			if !bytes.Equal(got[name], data) {
				t.Errorf("%s = %q, want staging's %q", name, got[name], data)
			}
		}
	})

	t.Run("it moves sessions.json into the state dir last, so a move cut short leaves it no index", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.cutShort()
		writeFile(t, filepath.Join(w.dir, "portal.log.2026-11-05"), "a log line\n")
		// An entry sorting after sessions.json that can't be moved: a directory
		// moved to another parent needs write permission on itself.
		stuck := mkdir(t, filepath.Join(w.staging, "zz-stuck"))
		if err := os.Chmod(stuck, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(stuck, 0o700) })

		if _, err := w.run(); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("Run = %v; want the stuck entry's move refused", err)
		}

		if present(t, sessionindex.Path(w.dir)) || !present(t, sessionindex.Path(w.staging)) {
			t.Error("sessions.json moved ahead of an entry left behind")
		}
		if err := os.Chmod(stuck, 0o700); err != nil {
			t.Fatal(err)
		}
		if o, err := w.run(); err != nil || o.Course != migrate.Finished {
			t.Errorf("the re-run = %v, %v; want it finished", o.Course, err)
		}
	})

	for name, arrange := range map[string]func(w *runWorld){
		"a file no migration put there": func(w *runWorld) {
			writeFile(w.t, filepath.Join(w.dir, "bootstrap.lock"), "")
		},
		"a scrollback dir while staging holds one still": func(w *runWorld) {
			writeFile(w.t, filepath.Join(state.ScrollbackDir(w.dir), "stray.bin"), "stray")
		},
	} {
		t.Run("it refuses to finish into a state dir holding "+name, func(t *testing.T) {
			w := newRunWorld(t).withTmuxState()
			w.cutShort()
			arrange(w)
			before := w.tree()

			_, err := w.run()

			if !errors.Is(err, migrate.ErrInTheWay) || !strings.HasPrefix(err.Error(), "Portal can't finish migrating: "+w.dir+" holds ") || !strings.Contains(err.Error(), w.staging) {
				t.Fatalf("Run = %v; want ErrInTheWay naming where the migrated state waits", err)
			}
			assertUnchanged(t, w, before, "refused")
		})
	}

	t.Run("it removes the empty staging dir a move cut short as it ended leaves, and skips to the bootstrap", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.migrated()
		mkdir(t, w.staging)

		o, err := w.run()

		if err != nil || o.Course != migrate.AlreadyMigrated || present(t, w.staging) {
			t.Errorf("Run = %v, %v, staging there: %v; want AlreadyMigrated, the empty staging dir gone", o.Course, err, present(t, w.staging))
		}
	})

	t.Run("it skips to the bootstrap when the migration landed", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.migrated()
		before := w.tree()

		o, err := w.run()

		if err != nil || o.Course != migrate.AlreadyMigrated || o.Backup != migrate.HooksBackup(w.tmuxDir) || o.TmuxDir != w.tmuxDir {
			t.Fatalf("Run = %+v, %v; want AlreadyMigrated, with state-tmux/ and the backup", o, err)
		}
		if len(w.steps) != 0 {
			t.Errorf("steps = %v; want tmux, the daemon and zmx unreached", w.steps)
		}
		assertUnchanged(t, w, before, "already migrated")
	})

	t.Run("it refuses the tmux version's state beside an earlier migration's", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		mkdir(t, w.tmuxDir)
		before := w.tree()

		_, err := w.run()

		if !errors.Is(err, migrate.ErrBothStates) || !strings.Contains(err.Error(), w.dir+" is still") || !strings.Contains(err.Error(), w.tmuxDir) {
			t.Fatalf("Run = %v; want ErrBothStates naming both dirs", err)
		}
		if len(w.steps) != 0 {
			t.Errorf("steps = %v; want nothing reached", w.steps)
		}
		assertUnchanged(t, w, before, "refused")
	})

	t.Run("it refuses a staging dir beside state-tmux/ and a state dir written since", func(t *testing.T) {
		w := newRunWorld(t).withTmuxState()
		w.migrated()
		writeFile(t, filepath.Join(w.staging, "sessions.json"), `{"version": 2, "sessions": []}`)
		before := w.tree()

		_, err := w.run()

		if !errors.Is(err, migrate.ErrStagingBesideState) || !strings.Contains(err.Error(), w.staging) || !strings.Contains(err.Error(), w.dir+" has been written") {
			t.Fatalf("Run = %v; want ErrStagingBesideState naming both dirs", err)
		}
		assertUnchanged(t, w, before, "refused")
	})

	for name, tc := range map[string]struct {
		arrange func(w *runWorld)
		course  migrate.Course
	}{
		"no state dir":                     {func(*runWorld) {}, migrate.NoState},
		"a state dir holding no index":     {func(w *runWorld) { writeFile(w.t, state.PortalLog(w.dir), "") }, migrate.NoState},
		"no state dir's parent":            {func(w *runWorld) { w.dir = filepath.Join(w.root, "absent", "state") }, migrate.NoState},
		"this version's state, never tmux": {func(w *runWorld) { writeFile(w.t, sessionindex.Path(w.dir), `{"version": 2, "sessions": []}`) }, migrate.NotTmux},
	} {
		t.Run("it finds nothing to migrate in "+name, func(t *testing.T) {
			w := newRunWorld(t)
			tc.arrange(w)
			before := w.tree()

			o, err := w.run()

			if err != nil || o.Course != tc.course || o.Course.Bootstraps() {
				t.Fatalf("Run = %v, %v; want %v, which bootstraps nothing", o.Course, err, tc.course)
			}
			if len(w.steps) != 0 {
				t.Errorf("steps = %v; want nothing reached", w.steps)
			}
			assertUnchanged(t, w, before, "nothing to migrate")
		})
	}

	t.Run("it refuses a sessions.json it can't read", func(t *testing.T) {
		w := newRunWorld(t)
		writeFile(t, sessionindex.Path(w.dir), `{"version": `)
		before := w.tree()

		_, err := w.run()

		if err == nil || !strings.HasPrefix(err.Error(), "Portal can't migrate: "+sessionindex.Path(w.dir)+" can't be read: ") {
			t.Fatalf("Run = %v; want a refusal naming sessions.json", err)
		}
		assertUnchanged(t, w, before, "refused")
	})
}

func TestRun_OneAtATime(t *testing.T) {
	w := newRunWorld(t).withTmuxState()
	held, err := os.Open(w.root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = held.Close() })
	if err := unix.Flock(int(held.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	before := w.tree()

	_, err = w.run()

	if !errors.Is(err, migrate.ErrLocked) || err.Error() != "Another portal migrate is running. Wait for it to finish." {
		t.Fatalf("Run = %v; want ErrLocked", err)
	}
	if len(w.steps) != 0 {
		t.Errorf("steps = %v; want nothing reached", w.steps)
	}
	assertUnchanged(t, w, before, "locked out")

	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	if o, err := w.run(); err != nil || o.Course != migrate.Migrated {
		t.Errorf("Run after the lock's release = %v, %v; want a full migration", o.Course, err)
	}
}
