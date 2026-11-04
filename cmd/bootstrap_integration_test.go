//go:build integration

package cmd_test

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/bootid"
	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

// bootSettle bounds each wait on a restored session's helper and on a saver's
// exit, its final flush included.
const bootSettle = 15 * time.Second

// dropSettle bounds a wait on a running saver dropping an ended session: two
// of its polls at least dropAfter apart, and the poll after.
const dropSettle = 30 * time.Second

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
	before := w.userSessions(t)
	work := filepath.Join(t.TempDir(), "myproj")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := w.command("open", "-p", work).CombinedOutput(); err != nil {
		t.Fatalf("portal open -p %s: %v\n%s", work, err, out)
	}
	made := slices.DeleteFunc(w.userSessions(t), func(name string) bool { return slices.Contains(before, name) })
	if len(made) != 1 {
		t.Fatalf("new user sessions = %v, want the one portal open made", made)
	}
	return made[0]
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
	w.awaitSaverExit(t, pid)
}

// crashSaver ends the saver's zmx daemon by sig, as a crash would, and waits
// out the saver's own exit, its final flush included.
func (w *bootWorld) crashSaver(t *testing.T, sig syscall.Signal) {
	t.Helper()
	pid := w.saverPID(t)
	w.crash(t, saver.SessionName, sig)
	w.awaitSaverExit(t, pid)
}

func (w *bootWorld) awaitSaverExit(t *testing.T, pid int) {
	t.Helper()
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		return zmxtest.Signal(w.dir.Backend, pid, 0) != nil
	}) {
		t.Fatalf("the saver's daemon %d outlived its session", pid)
	}
}

// crash ends the zmx daemon hosting name by sig, proved as it is sent, and
// waits for the socket it leaves behind to refuse.
func (w *bootWorld) crash(t *testing.T, name string, sig syscall.Signal) {
	t.Helper()
	if err := zmxtest.Signal(w.dir.Backend, zmxtest.Daemon(t, w.dir.Backend, name), sig); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(w.dir.Backend.Dir(), name)
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		c, err := net.Dial("unix", socket)
		if err == nil {
			_ = c.Close()
		}
		return errors.Is(err, syscall.ECONNREFUSED)
	}) {
		t.Fatalf("the socket of %s's dead daemon never refused", name)
	}
}

// assertGone checks name reads as no session at all, not one held and not
// answering.
func (w *bootWorld) assertGone(t *testing.T, name string) {
	t.Helper()
	if exists, err := w.dir.Backend.Exists(t.Context(), name); exists || err != nil {
		t.Errorf("Exists(%s) = %v, %v; want false, nil", name, exists, err)
	}
}

// awaitDropped waits for the running saver to drop name's entry.
func (w *bootWorld) awaitDropped(t *testing.T, name string) {
	t.Helper()
	if !harnesstest.PollUntil(t, dropSettle, 100*time.Millisecond, func() bool {
		_, held := w.entry(name)
		return strings.Contains(w.log(), "session dropped session="+name+" ") && !held
	}) {
		t.Fatalf("the saver never dropped %s\n--- portal.log ---\n%s", name, w.log())
	}
}

// entry is name's entry in sessions.json, as it reads now.
func (w *bootWorld) entry(name string) (sessionindex.Entry, bool) {
	idx, err := sessionindex.Read(w.dir.StateDir)
	if err != nil {
		return sessionindex.Entry{}, false
	}
	return idx.Lookup(name)
}

// oweBootstrap writes an older portal's latch for this boot, so the next
// command bootstraps with the saver up.
func (w *bootWorld) oweBootstrap(t *testing.T) {
	t.Helper()
	id, err := bootid.Read()
	if err != nil {
		t.Fatalf("read the boot id: %v", err)
	}
	if err := state.WriteLatch(w.dir.StateDir, state.Latch{BootID: id, Version: "an older portal"}); err != nil {
		t.Fatalf("owe a bootstrap: %v", err)
	}
}

// loggedOut is a world whose latch is current and whose saver, then n
// sessions of this boot, ended, as at a logout. It returns the sessions'
// names, in the order they were made.
func loggedOut(t *testing.T, n int) (*bootWorld, []string) {
	t.Helper()
	w := newBootWorld(t)
	w.portal(t, "list")
	var made []string
	for range n {
		made = append(made, w.open(t))
	}
	w.endSaver(t)
	for _, name := range made {
		w.end(t, name)
	}
	return w, made
}

// sortedNames is the names portal list --short printed, sorted, whatever
// order it listed them in.
func sortedNames(out string) []string {
	return slices.Sorted(slices.Values(strings.Fields(out)))
}

// background is a portal command left running, its output going to files of
// the test's own.
type background struct {
	cmd            *exec.Cmd
	stdout, stderr string
	done           chan struct{}
	err            error
}

// start runs portal with args in the background, and kills and reaps it at
// cleanup should the test leave it running.
func (w *bootWorld) start(t *testing.T, args ...string) *background {
	t.Helper()
	dir := t.TempDir()
	b := &background{cmd: w.command(args...), stdout: filepath.Join(dir, "stdout"), stderr: filepath.Join(dir, "stderr"), done: make(chan struct{})}
	stdout, err := os.Create(b.stdout)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdout.Close() }()
	stderr, err := os.Create(b.stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stderr.Close() }()
	b.cmd.Stdout, b.cmd.Stderr = stdout, stderr
	if err := b.cmd.Start(); err != nil {
		t.Fatalf("start portal %s: %v", strings.Join(args, " "), err)
	}
	go func() {
		b.err = b.cmd.Wait()
		close(b.done)
	}()
	t.Cleanup(func() {
		_ = b.cmd.Process.Kill()
		<-b.done
	})
	return b
}

// wait waits for the command to end, and returns what it printed and how it
// ended.
func (b *background) wait(t *testing.T) (stdout, stderr string, err error) {
	t.Helper()
	select {
	case <-b.done:
	case <-time.After(time.Minute):
		t.Fatalf("portal %s never ended", strings.Join(b.cmd.Args[1:], " "))
	}
	return b.read(t, b.stdout), b.read(t, b.stderr), b.err
}

func (b *background) read(t *testing.T, path string) string {
	t.Helper()
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// stalledRestore starts portal list in a logged-out world. Once its bootstrap
// has marked the ended sessions owed, it takes the commit lock, which the
// restore waits on to record the first session it makes; release lets it go.
// It returns once the restore has made that session and no other, the saver
// up and restoring raised.
func (w *bootWorld) stalledRestore(t *testing.T, made []string) (run *background, release func()) {
	t.Helper()
	run = w.start(t, "list", "--short")
	if !harnesstest.PollUntil(t, bootSettle, 5*time.Millisecond, func() bool {
		e, _ := w.entry(made[0])
		return e.Owed
	}) {
		t.Fatalf("the bootstrap never marked %s owed\n--- portal.log ---\n%s", made[0], w.log())
	}
	lock := commitlock.HoldForTest(t, w.dir.StateDir)
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		return len(w.userSessions(t)) > 0
	}) {
		t.Fatalf("the restore never made a session\n--- portal.log ---\n%s", w.log())
	}
	if got := w.userSessions(t); !slices.Equal(got, made[:1]) {
		t.Fatalf("sessions mid-restore = %v, want %v alone", got, made[:1])
	}
	if up, err := state.Restoring.Exists(w.dir.StateDir); !up || err != nil {
		t.Fatalf("restoring raised = %v, %v mid-restore; want it raised", up, err)
	}
	if exists, err := w.dir.Backend.Exists(t.Context(), saver.SessionName); !exists || err != nil {
		t.Fatalf("the saver mid-restore: %v, %v; want it running", exists, err)
	}
	return run, func() { _ = lock.Close() }
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
		w, ended := loggedOut(t, 1)
		made := ended[0]

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

		w.end(t, made)
		// Owed by an older portal's latch, so this bootstrap runs with the
		// saver up, before it has dropped the entry.
		w.oweBootstrap(t)
		if _, held := w.entry(made); !held {
			t.Fatalf("the entry for %s is gone before the bootstrap; the bootstrap would have nothing to leave", made)
		}

		if got := w.portal(t, "list", "--short"); got != "" {
			t.Errorf("portal list = %q, want nothing: the ended session stays ended", got)
		}
		if n := w.bootstraps(); n != 2 {
			t.Errorf("bootstraps = %d, want a second for the owed latch\n--- portal.log ---\n%s", n, w.log())
		}
	})
}

// Decision 62: a session a restore after a logout can't make again stays
// owed, which no saver drops, until a bootstrap brings it back.
func TestBootstrapKeepsWhatItCouldNotRestoreAgainstZmx(t *testing.T) {
	w, ended := loggedOut(t, 1)
	made := ended[0]
	// zmx refuses to make a session whose socket path holds anything but a
	// socket.
	blocker := filepath.Join(w.dir.Backend.Dir(), made)
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := w.start(t, "list", "--short").wait(t)
	if err != nil || stdout != "" {
		t.Fatalf("portal list = %q, %v; want nothing listed, the restore failing\n%s", stdout, err, stderr)
	}
	if want := "Portal couldn't restore 1 saved session — kept for the next restore."; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}

	// A session of this boot the saver watched end, which it drops while the
	// owed entry, missing longer, stays.
	watched := w.open(t)
	w.end(t, watched)
	w.awaitDropped(t, watched)
	if e, held := w.entry(made); !held || !e.Owed {
		t.Fatalf("entry for %s = %+v, %v once the saver dropped %s; want it kept, owed", made, e, held, watched)
	}

	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	w.oweBootstrap(t)
	if got, want := w.portal(t, "list", "--short"), made+"\n"; got != want {
		t.Errorf("portal list = %q, want %q, the owed session back", got, want)
	}
	if e, held := w.entry(made); !held || e.Owed {
		t.Errorf("entry for %s = %+v, %v once restored; want it held, owed no longer", made, e, held)
	}
	w.awaitNoFlags(t)
}

// interrupt kills run once until holds, and waits out its end.
func (w *bootWorld) interrupt(t *testing.T, run *background, what string, until func() bool) {
	t.Helper()
	if !harnesstest.PollUntil(t, bootSettle, 5*time.Millisecond, until) {
		t.Fatalf("the bootstrap never reached the point to interrupt: %s\n--- portal.log ---\n%s", what, w.log())
	}
	if err := run.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill the bootstrapping portal: %v", err)
	}
	_, _, _ = run.wait(t)
}

// stepsComplete counts the bootstraps portal.log records completing step.
func (w *bootWorld) stepsComplete(step string) int {
	return strings.Count(w.log(), "bootstrap: step complete step="+step+" ")
}

// assertLatchGone checks the latch is gone, so the next command bootstraps.
func (w *bootWorld) assertLatchGone(t *testing.T) {
	t.Helper()
	if latch, err := state.ReadLatch(w.dir.StateDir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("latch = %+v, %v; want it gone, so the next command finishes the bootstrap", latch, err)
	}
}

func (w *bootWorld) assertRestoring(t *testing.T, want bool) {
	t.Helper()
	if up, err := state.Restoring.Exists(w.dir.StateDir); up != want || err != nil {
		t.Errorf("restoring raised = %v, %v; want %v", up, err, want)
	}
}

// assertFinishedBy checks the next command finishes the bootstrap an
// interrupt left: it lists made, back, the second bootstrap of the world, and
// leaves the latch current and every flag lowered.
func (w *bootWorld) assertFinishedBy(t *testing.T, made string) {
	t.Helper()
	if got, want := w.portal(t, "list", "--short"), made+"\n"; got != want {
		t.Errorf("portal list = %q, want %q, the session back", got, want)
	}
	if n := w.bootstraps(); n != 2 {
		t.Errorf("bootstraps = %d, want the cold run's and the one finishing the interrupted one's\n--- portal.log ---\n%s", n, w.log())
	}
	if _, err := state.ReadLatch(w.dir.StateDir); err != nil {
		t.Errorf("read the latch: %v; want the finished bootstrap's", err)
	}
	w.awaitNoFlags(t)
}

// holdSaverLock takes saver.lock in dir as another process would, until the
// returned release or the test's end.
func holdSaverLock(t *testing.T, dir string) (release func()) {
	t.Helper()
	f, err := os.OpenFile(saverhost.LockPath(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("flock saver.lock: %v", err)
	}
	return func() { _ = f.Close() }
}

// Decision 67: a bootstrap removes the latch before its first change and
// writes it only once it has finished, so an interrupt anywhere leaves the
// next command to finish the work; the restoring flag covers restore's
// creates alone.
func TestInterruptedBootstrapAgainstZmx(t *testing.T) {
	t.Run("interrupted before the owed marks, the next command marks them and restores", func(t *testing.T) {
		w, ended := loggedOut(t, 1)
		made := ended[0]
		// The owed marks wait on the commit lock, so the bootstrap goes no
		// further than them.
		lock := commitlock.HoldForTest(t, w.dir.StateDir)
		run := w.start(t, "list", "--short")

		w.interrupt(t, run, "past the sweep", func() bool { return w.stepsComplete("Sweep") == 2 })
		_ = lock.Close()

		w.assertLatchGone(t)
		w.assertRestoring(t, false)
		w.assertGone(t, saver.SessionName)
		if e, held := w.entry(made); !held || e.Owed {
			t.Errorf("entry for %s = %+v, %v; want it held, not yet owed", made, e, held)
		}
		w.assertFinishedBy(t, made)
	})

	t.Run("interrupted between the owed marks and the saver, the next command restores", func(t *testing.T) {
		w, ended := loggedOut(t, 1)
		made := ended[0]
		release := holdSaverLock(t, w.dir.StateDir)
		run := w.start(t, "list", "--short")

		w.interrupt(t, run, "the owed marks", func() bool {
			e, _ := w.entry(made)
			return e.Owed
		})
		release()

		w.assertLatchGone(t)
		w.assertRestoring(t, false)
		w.assertGone(t, saver.SessionName)
		w.assertFinishedBy(t, made)
	})

	t.Run("interrupted between the saver and restore's first create, the next command lowers the flag and restores", func(t *testing.T) {
		w, ended := loggedOut(t, 1)
		made := ended[0]
		// A session whose daemon is stopped holds every listing to the probe
		// ceiling, restore's before its first create among them.
		if _, err := w.dir.Backend.Create(t.Context(), "stalled", t.TempDir(), []string{"sleep", "1000"}); err != nil {
			t.Fatal(err)
		}
		stalled := zmxtest.StopDaemon(t, w.dir.Backend, "stalled")
		run := w.start(t, "list", "--short")

		w.interrupt(t, run, "the restoring flag raised", func() bool {
			up, _ := state.Restoring.Exists(w.dir.StateDir)
			return up
		})
		if err := zmxtest.Signal(w.dir.Backend, stalled, syscall.SIGCONT); err != nil {
			t.Fatal(err)
		}
		w.end(t, "stalled")

		w.assertLatchGone(t)
		w.assertRestoring(t, true)
		w.assertGone(t, made)
		if e, _ := w.entry(made); !e.Owed {
			t.Errorf("entry for %s = %+v, want it owed", made, e)
		}
		w.assertFinishedBy(t, made)
		if !strings.Contains(w.log(), "bootstrap: restoring flag lowered ") {
			t.Errorf("portal.log records no flag left raised lowered\n--- portal.log ---\n%s", w.log())
		}
	})

	t.Run("interrupted mid-restore, the next command lowers the flag and finishes it", func(t *testing.T) {
		w, ended := loggedOut(t, 1)
		made := ended[0]
		run, release := w.stalledRestore(t, ended)

		if err := run.cmd.Process.Kill(); err != nil {
			t.Fatalf("kill the bootstrapping portal: %v", err)
		}
		_, _, _ = run.wait(t)
		release()

		w.assertLatchGone(t)
		w.assertRestoring(t, true)
		w.assertFinishedBy(t, made)
		if !strings.Contains(w.log(), "bootstrap: restoring flag lowered ") {
			t.Errorf("portal.log records no flag left raised lowered\n--- portal.log ---\n%s", w.log())
		}
		if !harnesstest.PollUntil(t, dropSettle, 100*time.Millisecond, func() bool {
			e, held := w.entry(made)
			return held && !e.Owed
		}) {
			e, held := w.entry(made)
			t.Errorf("entry for %s = %+v, %v; want the saver, finding it live, to repay it", made, e, held)
		}
	})

	t.Run("a command during a restore waits for it, then lists every session", func(t *testing.T) {
		w, made := loggedOut(t, 2)
		first, release := w.stalledRestore(t, made)

		second := w.start(t, "list", "--short")
		if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
			return strings.Contains(second.read(t, second.stderr), "Waiting for another Portal to finish starting…\n")
		}) {
			t.Fatalf("the second command never said it waits\n--- portal.log ---\n%s", w.log())
		}
		release()

		for i, run := range []*background{first, second} {
			stdout, stderr, err := run.wait(t)
			if got, want := sortedNames(stdout), sortedNames(strings.Join(made, "\n")); err != nil || !slices.Equal(got, want) {
				t.Errorf("portal list %d = %q, %v; want %q\n%s", i, got, err, want, stderr)
			}
		}
		if n := w.bootstraps(); n != 2 {
			t.Errorf("bootstraps = %d, want the cold run's and the restoring one's, the waiter running none\n--- portal.log ---\n%s", n, w.log())
		}
		w.awaitNoFlags(t)
	})
}

// A zmx daemon that dies without its kill leaves its socket behind, which
// reads as no session once no process holds it.
func TestBootstrapAfterZmxDaemonsDiedAgainstZmx(t *testing.T) {
	for name, sig := range map[string]syscall.Signal{"SIGKILL": syscall.SIGKILL, "SIGHUP": syscall.SIGHUP} {
		t.Run("the saver's and the sessions' daemons ended by "+name+": the next command brings this boot's sessions back", func(t *testing.T) {
			w := newBootWorld(t)
			w.portal(t, "list")
			made := w.open(t)

			w.crashSaver(t, sig)
			w.crash(t, made, sig)
			w.assertGone(t, saver.SessionName)
			w.assertGone(t, made)

			if got, want := w.portal(t, "list", "--short"), made+"\n"; got != want {
				t.Errorf("portal list = %q, want %q, this boot's session back", got, want)
			}
			if n := w.bootstraps(); n != 2 {
				t.Errorf("bootstraps = %d, want a second once the saver was gone\n--- portal.log ---\n%s", n, w.log())
			}
			w.saverPID(t)
			w.awaitNoFlags(t)
		})
	}

	t.Run("a session's daemon SIGKILLed under a running saver is dropped and stays ended", func(t *testing.T) {
		w := newBootWorld(t)
		w.portal(t, "list")
		made := w.open(t)

		w.crash(t, made, syscall.SIGKILL)
		w.assertGone(t, made)
		w.awaitDropped(t, made)

		w.oweBootstrap(t)
		if got := w.portal(t, "list", "--short"); got != "" {
			t.Errorf("portal list = %q, want nothing: the ended session stays ended", got)
		}
	})

	// zmx kill asks the daemon over its socket, and the daemon unlinks the
	// socket before it hangs up the session's program and exits.
	t.Run("a zmx kill from outside Portal leaves no socket, is dropped and stays ended", func(t *testing.T) {
		w := newBootWorld(t)
		w.portal(t, "list")
		made := w.open(t)

		kill := exec.Command(zmxtest.Binary(t), "kill", made)
		kill.Env = w.env
		if out, err := kill.CombinedOutput(); err != nil {
			t.Fatalf("zmx kill %s: %v\n%s", made, err, out)
		}
		if _, err := os.Lstat(filepath.Join(w.dir.Backend.Dir(), made)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("stat %s's socket after zmx kill = %v; want it gone", made, err)
		}
		w.assertGone(t, made)
		w.awaitDropped(t, made)

		w.oweBootstrap(t)
		if got := w.portal(t, "list", "--short"); got != "" {
			t.Errorf("portal list = %q, want nothing: the killed session stays ended", got)
		}
	})
}

// Decision 67: a bootstrap that can't judge the saver, as when its probes
// time out under load, leaves the saver saving, the missing latch alone owing
// the next command the bootstrap.
func TestSlowProbesLeaveTheSaverSavingAgainstZmx(t *testing.T) {
	w := newBootWorld(t)
	w.portal(t, "list")
	stopped := zmxtest.StopDaemon(t, w.dir.Backend, saver.SessionName)

	_, stderr, err := w.start(t, "list", "--short").wait(t)

	if err != nil {
		t.Fatalf("portal list = %v\n%s", err, stderr)
	}
	if want := "Portal couldn't tell which protocol zmx speaks"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want %q: the probes timed out", stderr, want)
	}
	if err := zmxtest.Signal(w.dir.Backend, stopped, syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	w.assertRestoring(t, false)
	w.assertLatchGone(t)

	if _, err := w.dir.Backend.Create(t.Context(), "outside", t.TempDir(), []string{"sleep", "1000"}); err != nil {
		t.Fatalf("make a session outside Portal: %v", err)
	}
	if !harnesstest.PollUntil(t, dropSettle, 100*time.Millisecond, func() bool {
		_, held := w.entry("outside")
		return held
	}) {
		t.Errorf("the saver never claimed a session made outside Portal: it stopped saving\n--- portal.log ---\n%s", w.log())
	}
}

// olderZmxFirst is the environment of w with older, a zmx 0.8.1 build, first
// on PATH: only the portal under test finds it, the test's own zmx calls
// running the pinned build.
func (w *bootWorld) olderZmxFirst(t *testing.T, older string) []string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(older, filepath.Join(dir, "zmx")); err != nil {
		t.Fatal(err)
	}
	return append(slices.Clone(w.env), "PATH="+dir+string(os.PathListSeparator)+filepath.Dir(zmxtest.Binary(t))+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// olderSaver leaves a saver session made on older, a zmx 0.8.1 build,
// running this portal's daemon, as a bootstrap cut short between making it
// and ending it leaves one, and waits for the daemon to hold its lock.
func (w *bootWorld) olderSaver(t *testing.T, older string) {
	t.Helper()
	start := exec.Command(older, "attach", saver.SessionName, w.binary, "state", "daemon")
	start.Env = w.env
	start.Dir = t.TempDir()
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("make the saver on zmx 0.8.1: %v\n%s", err, out)
	}
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		return strings.Contains(w.log(), "daemon: lock acquired ")
	}) {
		t.Fatalf("the daemon in the saver on zmx 0.8.1 never took its lock\n--- portal.log ---\n%s", w.log())
	}
}

// Decisions 63 and 68: a saver on zmx 0.8.1, found or made, is ended, and the
// bootstrap goes on as if no saver ran, so the fatal shows on every command
// until the supported zmx is first on PATH, which recovers.
func TestBootstrapRefusesOlderZmxAgainstZmx(t *testing.T) {
	older := zmxtest.OlderBinary(t)

	t.Run("a saver it makes on zmx 0.8.1 is ended, and fatal on every command until the supported zmx recovers", func(t *testing.T) {
		w := newBootWorld(t)
		w.save(t, "proj-saved")
		env := w.olderZmxFirst(t, older)

		for i := range 2 {
			list := w.command("list", "--short")
			list.Env = env
			out, err := list.CombinedOutput()

			if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() == 0 {
				t.Fatalf("portal list %d = %v; want it to fail\n%s", i, err, out)
			}
			if !strings.Contains(string(out), olderZmxFatal) {
				t.Errorf("portal list %d's output = %q, want %q", i, out, olderZmxFatal)
			}
			if n := strings.Count(w.log(), "Portal ended the saver running on it"); n != i+1 {
				t.Errorf("savers ended = %d after command %d, want %d\n--- portal.log ---\n%s", n, i, i+1, w.log())
			}
			w.awaitNoSessions(t)
			w.assertLatchGone(t)
			w.assertRestoring(t, false)
		}

		if got, want := w.portal(t, "list", "--short"), "proj-saved\n"; got != want {
			t.Errorf("portal list on the supported zmx = %q, want %q", got, want)
		}
		w.saverPID(t)
		w.awaitNoFlags(t)
	})

	t.Run("a saver on zmx 0.8.1 left behind is ended, and the next command on the supported zmx replaces it", func(t *testing.T) {
		w := newBootWorld(t)
		w.save(t, "proj-saved")
		w.olderSaver(t, older)

		if got, want := w.portal(t, "list", "--short"), "proj-saved\n"; got != want {
			t.Errorf("portal list = %q, want %q", got, want)
		}
		if !strings.Contains(w.log(), "bootstrap: saver ended ") {
			t.Errorf("portal.log records no saver ended\n--- portal.log ---\n%s", w.log())
		}
		w.saverPID(t)
		if n := w.bootstraps(); n != 1 {
			t.Errorf("bootstraps = %d, want one\n--- portal.log ---\n%s", n, w.log())
		}
		w.awaitNoFlags(t)
	})
}

// olderZmxFatal is the bootstrap's fatal for a saver on zmx 0.8.1.
const olderZmxFatal = "Portal can't use this zmx: it speaks zmx 0.8.1's protocol, which Portal doesn't support. Install a zmx build newer than 0.8.1."

// awaitNoSessions waits for the zmx directory to hold no session.
func (w *bootWorld) awaitNoSessions(t *testing.T) {
	t.Helper()
	if !harnesstest.PollUntil(t, bootSettle, 20*time.Millisecond, func() bool {
		sessions, err := w.dir.Backend.List(t.Context())
		return err == nil && len(sessions) == 0
	}) {
		sessions, err := w.dir.Backend.List(t.Context())
		t.Errorf("sessions = %+v, %v; want none: the saver ended and nothing restored", sessions, err)
	}
}
