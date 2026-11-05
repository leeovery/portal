//go:build integration

package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/proctest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// The tmux version's state a migration world holds: lantern, its pane's
// resume hook keyed by its token and registered eager, and tidepool, whose
// hook is keyed by its pane's address; each with its scrollback saved.
const (
	lanternSaved  = "lantern saved before the reboot"
	tidepoolSaved = "tidepool saved before the reboot"
	tmuxHooksJSON = `{
  "tokLan": {"on-resume": {"command": "touch ` + hookMark + `", "resume": "eager"}},
  "tidepool:0.0": {"on-resume": "make watch"}
}`
)

// userTmuxHook is a hook of the user's on an event Portal's hooks share.
const userTmuxHook = `run-shell "true # the user's"`

// daemonLag is how long after its saver session is killed the stand-in for
// the tmux version's daemon hears tmux's SIGHUP: long enough that a
// migration finds it running and waits.
const daemonLag = 500 * time.Millisecond

// migrationWorld is a bootstrap world whose state dir holds the tmux
// version's state, its hooks.json beside it.
type migrationWorld struct {
	*bootWorld
	hooksFile string
	tmuxDir   string
	tmuxState map[string][]byte
}

func newMigrationWorld(t *testing.T) *migrationWorld {
	t.Helper()
	w := &migrationWorld{bootWorld: newBootWorld(t)}
	w.hooksFile = hookstest.ResolveHooksFilePathFromEnv(t, w.env)
	w.tmuxDir = state.TmuxStateDir(w.dir.StateDir)
	work := t.TempDir()
	w.tmuxState = map[string][]byte{
		"sessions.json": fmt.Appendf(nil, `{"version": 1, "saved_at": "2026-11-04T22:15:03Z", "sessions": [%s, %s]}`,
			tmuxSessionJSON("lantern", work, "scrollback/pane-tokLan.bin", "tokLan"),
			tmuxSessionJSON("tidepool", work, "scrollback/tidepool__0.0.bin", "")),
		"scrollback/pane-tokLan.bin":   []byte("$ make\r\n" + lanternSaved + "\r\n$ "),
		"scrollback/tidepool__0.0.bin": []byte("$ make\r\n" + tidepoolSaved + "\r\n$ "),
	}
	for name, data := range w.tmuxState {
		writeMigrationFile(t, filepath.Join(w.dir.StateDir, name), data)
	}
	writeMigrationFile(t, w.hooksFile, []byte(tmuxHooksJSON))
	return w
}

// tmuxSessionJSON is a session of the tmux version's sessions.json: one
// window holding one pane, working in cwd.
func tmuxSessionJSON(name, cwd, scrollback, token string) string {
	return fmt.Sprintf(`{"name": %q, "environment": {}, "windows": [{"index": 0, "name": "sh", "layout": "", "zoomed": false, "active": true,
  "panes": [{"index": 0, "cwd": %q, "active": true, "current_command": "sh", "scrollback_file": %q, "portal_pane_id": %q}]}]}`, name, cwd, scrollback, token)
}

func writeMigrationFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// withTmux gives w the tmux version's server, holding no portal on its PATH:
// the hooks the tmux version registered, a user's hook beside them, and its
// saver session. The world's portal reaches it through $TMUX, as one run in
// its sessions does.
func (w *migrationWorld) withTmux(t *testing.T) *tmuxtest.Socket {
	t.Helper()
	tmuxtest.SkipIfNoTmux(t)
	ts := tmuxtest.New(t, "ptl-")
	ts.StartPortalFree(t)
	ts.RegisterTmuxVersionHooks(t)
	ts.Run(t, "set-hook", "-ga", "session-created", userTmuxHook)
	ts.Run(t, "new-session", "-d", "-s", "_portal-saver", "exec sleep 600")
	pid := strings.TrimSpace(ts.Run(t, "display-message", "-p", "#{pid}"))
	w.env = append(w.env, "TMUX="+ts.SocketPath()+","+pid+",0")
	return ts
}

// startTmuxDaemon starts a portal state daemon standing in for the tmux
// version's, over a state dir, hooks.json and projects.json of its own and no
// zmx directory, so it saves nothing anywhere, and names it in the tmux
// version's daemon.pid. It returns the daemon, held.
func (w *migrationWorld) startTmuxDaemon(t *testing.T) *proctest.Held {
	t.Helper()
	dir := shortTempDir(t)
	portaltest.OwnStateDir(t, dir)
	own := t.TempDir()
	daemon := exec.Command(w.binary, "state", "daemon")
	daemon.Env = append(slices.Clone(w.dir.Env),
		"PORTAL_STATE_DIR="+dir,
		"PORTAL_HOOKS_FILE="+filepath.Join(own, "hooks.json"),
		"PORTAL_PROJECTS_FILE="+filepath.Join(own, "projects.json"),
	)
	held := proctest.Start(t, daemon, "")
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		pid, err := state.ReadPIDFile(dir)
		return err == nil && pid == held.PID()
	}) {
		t.Fatal("the stand-in daemon never wrote its daemon.pid")
	}
	if err := state.WritePIDFile(w.dir.StateDir, held.PID()); err != nil {
		t.Fatal(err)
	}
	return held
}

// hangUpOnSaverKill sends daemon tmux's SIGHUP daemonLag after the tmux
// version's saver session is gone from ts, as tmux sends one to the
// processes of a session it kills. The channel gives the signal's error, or
// why none was sent.
func hangUpOnSaverKill(ts *tmuxtest.Socket, daemon *proctest.Held, done <-chan struct{}) <-chan error {
	sent := make(chan error, 1)
	go func() {
		for {
			select {
			case <-done:
				sent <- errors.New("the saver session was never killed")
				return
			case <-time.After(20 * time.Millisecond):
			}
			if _, err := ts.TryRun("has-session", "-t", "=_portal-saver:"); err != nil {
				time.Sleep(daemonLag)
				sent <- daemon.Signal(daemon.PID(), syscall.SIGHUP)
				return
			}
		}
	}()
	return sent
}

// migrate runs portal migrate with extra in its environment, and returns
// what it printed and how it ended.
func (w *migrationWorld) migrate(extra ...string) (stdout, stderr string, err error) {
	run := w.command("migrate")
	run.Env = append(run.Env, extra...)
	var out, errOut bytes.Buffer
	run.Stdout, run.Stderr = &out, &errOut
	err = run.Run()
	return out.String(), errOut.String(), err
}

// mustMigrate runs portal migrate, failing the test unless it exits 0.
func (w *migrationWorld) mustMigrate(t *testing.T) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, err := w.migrate()
	if err != nil {
		t.Fatalf("portal migrate: %v\nstdout:\n%s\nstderr:\n%s\n--- portal.log ---\n%s", err, stdout, stderr, w.log())
	}
	return stdout, stderr
}

// keptReport is the last lines of every report of a migration: where the
// tmux version's state and hooks.json's backup are kept.
func (w *migrationWorld) keptReport() string {
	return "The tmux version's state is kept at " + w.tmuxDir + ".\n" +
		"hooks.json's backup is at " + migrate.HooksBackup(w.tmuxDir) + ".\n"
}

// assertTmuxStateKept checks state-tmux/ holds the tmux version's state
// byte for byte, and hooks.json's backup the tmux version's hooks.json.
func (w *migrationWorld) assertTmuxStateKept(t *testing.T) {
	t.Helper()
	for name, data := range w.tmuxState {
		if got, err := os.ReadFile(filepath.Join(w.tmuxDir, name)); err != nil || !bytes.Equal(got, data) {
			t.Errorf("state-tmux/%s = %q, %v; want the tmux version's %q", name, got, err, data)
		}
	}
	if got, err := os.ReadFile(migrate.HooksBackup(w.tmuxDir)); err != nil || string(got) != tmuxHooksJSON {
		t.Errorf("hooks.json's backup = %q, %v; want the tmux version's hooks.json", got, err)
	}
}

// assertHooksRekeyed checks hooks.json holds lantern's pane's hook alone,
// under lantern's name, as the tmux version held it.
func (w *migrationWorld) assertHooksRekeyed(t *testing.T) {
	t.Helper()
	held := decodeHooksFile(t, w.hooksFile)
	want := decodeHooksFile(t, migrate.HooksBackup(w.tmuxDir))["tokLan"]
	if got := slices.Sorted(maps.Keys(held)); !slices.Equal(got, []string{"lantern"}) || !bytes.Equal(compactJSON(t, held["lantern"]), compactJSON(t, want)) {
		t.Errorf("hooks.json = %v, want lantern's hook alone, as tokLan held it", held)
	}
}

func decodeHooksFile(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	var held map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &held)
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return held
}

func compactJSON(t *testing.T, raw json.RawMessage) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// assertRestored checks the migrated sessions were made: tidepool showing
// its saved scrollback, lantern parked on its resume panel though its hook
// is registered eager, its command not run; and that each entry, once its
// restore finished, is owed nothing and resumes as its hook says.
func (w *migrationWorld) assertRestored(t *testing.T) {
	t.Helper()
	if got := slices.Sorted(slices.Values(w.userSessions(t))); !slices.Equal(got, []string{"lantern", "tidepool"}) {
		t.Fatalf("zmx sessions = %v, want lantern and tidepool", got)
	}
	awaitSessionHistory(t, w.dir.Backend, "tidepool", "its saved scrollback", func(history string) bool {
		return strings.Contains(ansi.Strip(history), tidepoolSaved)
	})
	panel := awaitSessionHistory(t, w.dir.Backend, "lantern", "its resume panel", func(history string) bool {
		return strings.Contains(ansi.Strip(history), hookMark)
	})
	if !strings.Contains(panel, altScreenEnter) {
		t.Errorf("lantern's history = %q, want its panel, on the alternate screen", panel)
	}
	var entries []sessionindex.Entry
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		idx, err := sessionindex.Read(w.dir.StateDir)
		entries = idx.Entries()
		return err == nil && len(entries) == 2 && !slices.ContainsFunc(entries, func(e sessionindex.Entry) bool { return e.Owed || e.ResumeLazy })
	}) {
		t.Errorf("entries = %+v; want both restored, owed nothing and resuming lazily no longer", entries)
	}
	if pending, err := state.ResumePending.Exists(w.dir.StateDir, "lantern"); err != nil || !pending {
		t.Errorf("lantern's resume-pending = %v, %v; want it raised while its panel waits", pending, err)
	}
	if pending, _ := state.ResumePending.Exists(w.dir.StateDir, "tidepool"); pending {
		t.Error("tidepool waits on a resume panel with no hook to resume")
	}
	if _, err := os.Stat(filepath.Join(w.workOf(t, "lantern"), hookMark)); err == nil {
		t.Error("lantern's resume command ran before Enter")
	}
}

// workOf is the dir the tmux version saved name's pane working in.
func (w *migrationWorld) workOf(t *testing.T, name string) string {
	t.Helper()
	idx, err := sessionindex.Read(w.dir.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := idx.Lookup(name)
	return e.Cwd
}

// liveSessions is each zmx session's name and pid.
func (w *migrationWorld) liveSessions(t *testing.T) map[string]int {
	t.Helper()
	sessions, err := w.dir.Backend.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	live := map[string]int{}
	for _, s := range sessions {
		live[s.Name] = s.PID
	}
	return live
}

// treeOf is every file below dir, by its path relative to dir, with its
// content.
func treeOf(t *testing.T, dir string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		data, err := os.ReadFile(path)
		tree[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestMigrateAgainstZmx(t *testing.T) {
	w := newMigrationWorld(t)
	ts := w.withTmux(t)
	daemon := w.startTmuxDaemon(t)
	done := make(chan struct{})
	hungUp := hangUpOnSaverKill(ts, daemon, done)

	stdout, stderr := w.mustMigrate(t)
	close(done)

	t.Run("it unsets Portal's tmux hooks alone and kills the saver", func(t *testing.T) {
		for _, h := range tmuxtest.TmuxVersionHooks {
			if listed := ts.Run(t, "show-hooks", "-g", h.Event); strings.Contains(listed, "portal state") {
				t.Errorf("%s still holds Portal's hook: %s", h.Event, listed)
			}
		}
		if listed := ts.Run(t, "show-hooks", "-g", "session-created"); !strings.Contains(listed, userTmuxHook) {
			t.Errorf("session-created = %q, want the user's hook kept", listed)
		}
		if _, err := ts.TryRun("has-session", "-t", "=_portal-saver:"); err == nil {
			t.Error("_portal-saver is still there")
		}
		if _, err := ts.TryRun("has-session", "-t", "=_portal-bootstrap:"); err != nil {
			t.Error("the tmux server's anchor session went too")
		}
	})

	t.Run("it waits for the tmux version's daemon to exit", func(t *testing.T) {
		if err := <-hungUp; err != nil {
			t.Fatalf("hang up the stand-in daemon: %v", err)
		}
		select {
		case <-daemon.Exited():
		default:
			t.Fatal("migrate finished while the daemon it waited on still ran")
		}
		pid := strconv.Itoa(daemon.PID())
		if want := "Waiting for the tmux version's save daemon (pid " + pid + ") to save and exit…\n"; !strings.Contains(stderr, want) {
			t.Errorf("stderr = %q, want the wait told: %q", stderr, want)
		}
		if log := portaltest.ReadPortalLogSafe(w.tmuxDir); !strings.Contains(log, "INFO migrate: tmux daemon exited target_pid="+pid+" ") {
			t.Errorf("the tmux version's log holds no record of the daemon's exit:\n%s", log)
		}
	})

	t.Run("it keeps the tmux version's state and hooks.json's backup in state-tmux", func(t *testing.T) {
		w.assertTmuxStateKept(t)
		if pid, err := state.ReadPIDFile(w.tmuxDir); err != nil || pid != daemon.PID() {
			t.Errorf("state-tmux/daemon.pid = %d, %v; want the tmux version's daemon's", pid, err)
		}
		if _, err := os.Stat(state.MigrationStagingDir(w.dir.StateDir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the staging dir is still there: %v", err)
		}
	})

	t.Run("it leaves this version's state and hooks.json rekeyed", func(t *testing.T) {
		if _, err := sessionindex.Read(w.dir.StateDir); err != nil {
			t.Errorf("the state dir's sessions.json: %v; want this version's", err)
		}
		w.assertHooksRekeyed(t)
	})

	t.Run("it restores the sessions, the one with a hook parked on its panel", func(t *testing.T) {
		w.assertRestored(t)
	})

	t.Run("it reports the migration", func(t *testing.T) {
		want := "Migrated 2 tmux sessions to 2 sessions.\n" +
			"Moved 1 resume hook. Removed 1, kept in the backup.\n" +
			"Restored 2 sessions. Every saved session is restored.\n" +
			"Each migrated session with a resume hook waits on its resume panel the first time you open it.\n" +
			"\n" +
			"Sessions (name ← tmux name):\n" +
			"  lantern ← lantern\n" +
			"  tidepool ← tidepool\n" +
			"\n" +
			"Resume hooks moved:\n" +
			"  tokLan → lantern\n" +
			"\n" +
			"Resume hooks removed (kept in the backup):\n" +
			"  tidepool:0.0: a pane address, which hasn't fired since pane tokens\n" +
			"\n" + w.keptReport()
		if stdout != want {
			t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
		}
	})

	t.Run("a re-run changes nothing, and reports", func(t *testing.T) {
		kept := treeOf(t, w.tmuxDir)
		hooksBefore := readFileOrEmpty(t, w.hooksFile)
		live := w.liveSessions(t)

		again, _ := w.mustMigrate(t)

		if want := "Already migrated.\nEvery saved session is restored.\n\n" + w.keptReport(); again != want {
			t.Errorf("stdout:\n%s\nwant:\n%s", again, want)
		}
		if !maps.Equal(treeOf(t, w.tmuxDir), kept) {
			t.Error("state-tmux/ changed")
		}
		hookstest.AssertHooksFileUnchanged(t, w.hooksFile, hooksBefore, "a re-run")
		if got := w.liveSessions(t); !maps.Equal(got, live) {
			t.Errorf("zmx sessions = %v, want %v untouched", got, live)
		}
	})
}

func readFileOrEmpty(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A run stopped between its renames leaves the tmux version's state in
// state-tmux/ and this version's in the staging dir, with only the log the
// stop wrote in the state dir; the re-run finishes it.
func TestMigrateCutShortBetweenItsRenamesAgainstZmx(t *testing.T) {
	w := newMigrationWorld(t)
	staging := state.MigrationStagingDir(w.dir.StateDir)

	if stdout, stderr, err := w.migrate(migrate.StopEnv + "=" + migrate.BetweenRenames); err == nil {
		t.Fatalf("the stopped run exited 0\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}

	if _, err := sessionindex.Read(staging); err != nil {
		t.Fatalf("the staging dir's sessions.json: %v; want this version's waiting there", err)
	}
	if _, err := os.Stat(sessionindex.Path(w.dir.StateDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the state dir holds a sessions.json (%v); want none between the renames", err)
	}
	w.assertTmuxStateKept(t)
	if live := w.userSessions(t); len(live) != 0 {
		t.Fatalf("zmx sessions = %v, want none before the bootstrap", live)
	}

	// Until a run finishes the migration, a command stops on it rather than
	// bootstrap the state dir holding none of the migrated sessions.
	hooksBefore := readFileOrEmpty(t, w.hooksFile)
	said, err := w.command("list").CombinedOutput()
	if want := staging + " holds a migration cut short; run portal migrate to finish it"; err == nil || !strings.Contains(string(said), want) {
		t.Fatalf("portal list = %v, saying %q; want it stopped, saying %q", err, said, want)
	}
	if live := w.liveSessions(t); len(live) != 0 {
		t.Fatalf("zmx sessions = %v after portal list, want none: no bootstrap, no saver", live)
	}
	hookstest.AssertHooksFileUnchanged(t, w.hooksFile, hooksBefore, "the stopped command")

	stdout, _ := w.mustMigrate(t)

	want := "Finished a migration cut short: the migrated state is in place at " + w.dir.StateDir + ".\n" +
		"Restored 2 sessions. Every saved session is restored.\n" +
		"Each migrated session with a resume hook waits on its resume panel the first time you open it.\n" +
		"\n" + w.keptReport()
	if stdout != want {
		t.Errorf("stdout:\n%s\nwant:\n%s", stdout, want)
	}
	if _, err := os.Stat(staging); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the staging dir is still there: %v", err)
	}
	w.assertTmuxStateKept(t)
	w.assertHooksRekeyed(t)
	w.assertRestored(t)
}
