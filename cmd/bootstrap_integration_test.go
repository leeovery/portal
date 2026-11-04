//go:build integration

package cmd_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/bootid"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

// bootSettle bounds each wait on a restored session's helper and on a saver's
// exit, its final flush included.
const bootSettle = 15 * time.Second

// savedBefore is a creation before this boot, so restore brings back an entry
// of it whether a saver ran or not.
var savedBefore = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// bootWorld is a zmx directory and a state directory of the test's own, with
// no saver until a portal command's bootstrap starts one, and the environment
// the built portal runs in against them.
type bootWorld struct {
	binary string
	dir    portaltest.SaverDir
	env    []string
}

func newBootWorld(t *testing.T) *bootWorld {
	t.Helper()
	dir := portaltest.NewSaverDir(t)
	env := slices.Concat(dir.Env, dir.Backend.ReachEnv(), []string{
		"PATH=" + filepath.Dir(zmxtest.Binary(t)) + string(os.PathListSeparator) + os.Getenv("PATH"),
		"PORTAL_PROJECTS_FILE=" + filepath.Join(t.TempDir(), "projects.json"),
		"SHELL=/bin/sh",
	})
	return &bootWorld{binary: buildPortalBinary(t), dir: dir, env: env}
}

func (w *bootWorld) command(args ...string) *exec.Cmd {
	cmd := exec.Command(w.binary, args...)
	cmd.Env = w.env
	return cmd
}

// portal runs portal with args, failing the test on an error, and returns
// what it printed.
func (w *bootWorld) portal(t *testing.T, args ...string) string {
	t.Helper()
	out, err := w.command(args...).Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		t.Fatalf("portal %s: %v\n%s\n--- portal.log ---\n%s", strings.Join(args, " "), err, exit.Stderr, w.log())
	}
	if err != nil {
		t.Fatalf("portal %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

// save records an entry for name, made in a directory of the test's own
// before this boot.
func (w *bootWorld) save(t *testing.T, name string) {
	t.Helper()
	dir := t.TempDir()
	if _, err := sessionindex.Update(w.dir.StateDir, "cli", func(idx *sessionindex.Index) error {
		idx.Claim(name, dir, savedBefore, 0, state.ClaimRule(w.dir.StateDir))
		return nil
	}); err != nil {
		t.Fatalf("save %s: %v", name, err)
	}
}

// open makes a session with portal open -p, its attach detaching at once on
// stdin's end, and returns its name.
func (w *bootWorld) open(t *testing.T) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "myproj")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := w.command("open", "-p", work).CombinedOutput(); err != nil {
		t.Fatalf("portal open -p %s: %v\n%s", work, err, out)
	}
	users := w.userSessions(t)
	if len(users) != 1 {
		t.Fatalf("user sessions = %v, want the one portal open made", users)
	}
	return users[0]
}

// userSessions is the name of every session the zmx directory lists but
// Portal's own.
func (w *bootWorld) userSessions(t *testing.T) []string {
	t.Helper()
	sessions, err := w.dir.Backend.List(t.Context())
	if err != nil {
		t.Fatalf("list %s: %v", w.dir.Backend.Dir(), err)
	}
	var names []string
	for _, s := range slices.DeleteFunc(sessions, backend.Session.PortalOwned) {
		names = append(names, s.Name)
	}
	return names
}

func (w *bootWorld) saverPID(t *testing.T) int {
	t.Helper()
	s, err := w.dir.Backend.Session(t.Context(), saver.SessionName)
	if err != nil || s.PID <= 0 {
		t.Fatalf("read the saver: %+v, %v", s, err)
	}
	return s.PID
}

// endSaver kills the saver and waits out its daemon's final flush, so it
// sees no session end after it.
func (w *bootWorld) endSaver(t *testing.T) {
	t.Helper()
	pid := w.saverPID(t)
	if err := w.dir.Backend.Kill(t.Context(), saver.SessionName); err != nil {
		t.Fatalf("end the saver: %v", err)
	}
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		return zmxtest.Signal(w.dir.Backend, pid, 0) != nil
	}) {
		t.Fatalf("the saver's daemon %d outlived its session", pid)
	}
}

func (w *bootWorld) end(t *testing.T, name string) {
	t.Helper()
	if err := w.dir.Backend.Kill(t.Context(), name); err != nil {
		t.Fatalf("end %s: %v", name, err)
	}
}

func (w *bootWorld) log() string { return portaltest.ReadPortalLogSafe(w.dir.StateDir) }

// bootstraps counts the full bootstraps portal.log records.
func (w *bootWorld) bootstraps() int {
	return strings.Count(w.log(), "bootstrap: orchestration complete ")
}

// awaitNoFlags waits for every flag in the state directory to be lowered,
// the restored sessions' helpers done.
func (w *bootWorld) awaitNoFlags(t *testing.T) {
	t.Helper()
	var raised []string
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		raised = w.flags(t)
		return len(raised) == 0
	}) {
		t.Fatalf("flags %v stayed raised\n--- portal.log ---\n%s", raised, w.log())
	}
}

func (w *bootWorld) flags(t *testing.T) []string {
	t.Helper()
	var raised []string
	if up, err := state.Restoring.Exists(w.dir.StateDir); err != nil || up {
		raised = append(raised, "restoring")
	}
	for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
		names, err := flag.List(w.dir.StateDir)
		if err != nil {
			t.Fatalf("list %s: %v", flag, err)
		}
		for _, name := range names {
			raised = append(raised, string(flag)+"/"+name)
		}
	}
	return raised
}

func TestColdBootstrapRestoresAndWarmSkipsAgainstZmx(t *testing.T) {
	w := newBootWorld(t)
	w.save(t, "proj-saved")

	if got, want := w.portal(t, "list", "--short"), "proj-saved\n"; got != want {
		t.Fatalf("cold portal list = %q, want %q, the saved session restored", got, want)
	}
	w.awaitNoFlags(t)

	t.Run("it writes the latch of this boot and this binary", func(t *testing.T) {
		id, err := bootid.Read()
		if err != nil {
			t.Fatalf("read the boot id: %v", err)
		}
		version := strings.TrimPrefix(strings.TrimSpace(w.portal(t, "version")), "portal version ")
		latch, err := state.ReadLatch(w.dir.StateDir)
		if want := (state.Latch{BootID: id, Version: version}); err != nil || latch != want {
			t.Errorf("latch = %+v, %v; want %+v", latch, err, want)
		}
	})

	t.Run("a warm run skips the bootstrap, keeping the saver", func(t *testing.T) {
		pid := w.saverPID(t)

		if got, want := w.portal(t, "list", "--short"), "proj-saved\n"; got != want {
			t.Errorf("warm portal list = %q, want %q", got, want)
		}

		if n := w.bootstraps(); n != 1 {
			t.Errorf("bootstraps = %d, want the cold run's alone\n--- portal.log ---\n%s", n, w.log())
		}
		if got := w.saverPID(t); got != pid {
			t.Errorf("saver pid = %d, want %d kept", got, pid)
		}
	})
}

func TestConcurrentColdBootstrapsRestoreOnceAgainstZmx(t *testing.T) {
	w := newBootWorld(t)
	saved := []string{"api-saved", "web-saved"}
	for _, name := range saved {
		w.save(t, name)
	}

	cmds := []*exec.Cmd{w.command("list", "--short"), w.command("list", "--short")}
	outs := make([]strings.Builder, len(cmds))
	for i, cmd := range cmds {
		cmd.Stdout = &outs[i]
		if err := cmd.Start(); err != nil {
			t.Fatalf("start portal list: %v", err)
		}
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("portal list %d: %v\n--- portal.log ---\n%s", i, err, w.log())
		}
		if got, want := outs[i].String(), "api-saved\nweb-saved\n"; got != want {
			t.Errorf("portal list %d = %q, want %q", i, got, want)
		}
	}
	w.awaitNoFlags(t)

	log := w.log()
	if n := strings.Count(log, "bootstrap: orchestration complete "); n != 1 {
		t.Errorf("bootstraps = %d, want one, the other waiting it out\n--- portal.log ---\n%s", n, log)
	}
	if n := strings.Count(log, "restore: restore complete "); n != 1 || !strings.Contains(log, "restored=2 ") {
		t.Errorf("restores = %d, want one restoring both sessions\n--- portal.log ---\n%s", n, log)
	}
	if got := w.userSessions(t); !slices.Equal(got, saved) {
		t.Errorf("sessions = %v, want %v", got, saved)
	}
}

// Decision 58: the bootstrap brings back every session of this boot when no
// saver ran to see them end, and none a running saver saw end.
func TestBootstrapAfterTheSaverEndedAgainstZmx(t *testing.T) {
	t.Run("a logout ending the saver and the sessions brings this boot's back", func(t *testing.T) {
		w := newBootWorld(t)
		w.portal(t, "list")
		made := w.open(t)

		w.endSaver(t)
		w.end(t, made)

		if got, want := w.portal(t, "list", "--short"), made+"\n"; got != want {
			t.Errorf("portal list = %q, want %q, this boot's session back", got, want)
		}
		if n := w.bootstraps(); n != 2 {
			t.Errorf("bootstraps = %d, want a second once the saver was gone\n--- portal.log ---\n%s", n, w.log())
		}
		w.awaitNoFlags(t)
	})

	t.Run("a session ending under a running saver stays ended", func(t *testing.T) {
		w := newBootWorld(t)
		w.portal(t, "list")
		made := w.open(t)
		id, err := bootid.Read()
		if err != nil {
			t.Fatalf("read the boot id: %v", err)
		}

		w.end(t, made)
		// Owed by an older portal's latch, so this bootstrap runs with the
		// saver up, before it has dropped the entry.
		if err := state.WriteLatch(w.dir.StateDir, state.Latch{BootID: id, Version: "an older portal"}); err != nil {
			t.Fatalf("owe a bootstrap: %v", err)
		}
		idx, err := sessionindex.Read(w.dir.StateDir)
		if _, held := idx.Lookup(made); err != nil || !held {
			t.Fatalf("the entry for %s is gone before the bootstrap (%v); the bootstrap would have nothing to leave", made, err)
		}

		if got := w.portal(t, "list", "--short"); got != "" {
			t.Errorf("portal list = %q, want nothing: the ended session stays ended", got)
		}
		if n := w.bootstraps(); n != 2 {
			t.Errorf("bootstraps = %d, want a second for the owed latch\n--- portal.log ---\n%s", n, w.log())
		}
	})
}
