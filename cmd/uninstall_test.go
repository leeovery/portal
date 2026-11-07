package cmd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
	"golang.org/x/sys/unix"
)

// removal is uninstall's second line, whatever it did.
const removal = "To remove Portal completely, uninstall the binary and delete that directory.\n"

// stopperFake is a stop that finds a saver when stopped, failing with err,
// which it hands to orElse, and counts the stops asked of it. One whose saver
// is theirs, another state directory's, fails with it, orElse unasked, as
// saverhost's does.
type stopperFake struct {
	stopped bool
	err     error
	theirs  *saverhost.OtherStateDirError
	stops   int
}

func (s *stopperFake) StopOr(ctx context.Context, orElse func(context.Context, error) (bool, error)) (bool, error) {
	s.stops++
	switch {
	case s.theirs != nil:
		return s.stopped, fmt.Errorf("pid %d isn't confirmed as the save daemon, so nothing was sent to it: %w", s.theirs.PID, s.theirs)
	case s.err != nil:
		return orElse(ctx, s.err)
	}
	return s.stopped, nil
}

// daemonFake is the saver's daemon, one Portal can't use when unusable,
// whose end fails with endErr, counting the judgements and ends asked of it.
// onAsk runs at each judgement and each end, handed which it is.
type daemonFake struct {
	isUnusable bool
	endErr     error
	asks, ends int
	onAsk      func(what string)
}

func (d *daemonFake) unusable(context.Context) bool {
	d.asks++
	d.asked("the judgement")
	return d.isUnusable
}

func (d *daemonFake) End(context.Context) error {
	d.ends++
	d.asked("the end")
	return d.endErr
}

func (d *daemonFake) asked(what string) {
	if d.onAsk != nil {
		d.onAsk(what)
	}
}

func TestUninstall(t *testing.T) {
	t.Run("it stops the saver and says where the saved sessions and config are", func(t *testing.T) {
		stop := &stopperFake{stopped: true}
		daemon := &daemonFake{isUnusable: true}
		withUninstallDeps(t, UninstallDeps{Saver: stop, Daemon: daemon})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon stopped. Saved sessions and config are untouched at ~/.config/portal/.\n" + removal; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if stop.stops != 1 {
			t.Errorf("stops = %d, want one", stop.stops)
		}
		if daemon.asks != 0 || daemon.ends != 0 {
			t.Errorf("the daemon was judged %d times and ended %d, want neither past a stop that worked", daemon.asks, daemon.ends)
		}
	})

	t.Run("with no saver running it says so, and succeeds", func(t *testing.T) {
		stop := &stopperFake{}
		withUninstallDeps(t, UninstallDeps{Saver: stop})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon isn't running. Saved sessions and config are untouched at ~/.config/portal/.\n" + removal; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("it names a config directory outside the home whole", func(t *testing.T) {
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{stopped: true}})
		t.Setenv("XDG_CONFIG_HOME", "/srv/config")

		stdout, _, err := runRootCmd(t, "uninstall")

		if want := "Portal's save daemon stopped. Saved sessions and config are untouched at /srv/config/portal/.\n" + removal; err != nil || stdout.String() != want {
			t.Errorf("uninstall = %q, %v; want %q", stdout, err, want)
		}
	})

	refused := errors.New("pid 4242 isn't confirmed as the save daemon, so nothing was sent to it")
	for name, fake := range map[string]*stopperFake{
		"a stop that fails":                  {stopped: true, err: refused},
		"a saver whose daemon can't be read": {err: refused},
	} {
		t.Run(name+" fails the command with its reason, printing nothing", func(t *testing.T) {
			daemon := &daemonFake{}
			withUninstallDeps(t, UninstallDeps{Saver: fake, Daemon: daemon})

			stdout, _, err := runRootCmd(t, "uninstall")

			if !errors.Is(err, refused) {
				t.Fatalf("uninstall = %v, want the failure", err)
			}
			if want := "couldn't stop Portal's save daemon: " + refused.Error(); err.Error() != want {
				t.Errorf("error = %q, want %q", err, want)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing", stdout)
			}
			if daemon.ends != 0 {
				t.Errorf("ends = %d, want none of a daemon Portal can use", daemon.ends)
			}
		})
	}

	// Decision 68: a saver whose daemon speaks a zmx protocol Portal can't
	// use can't be read to stop, and is ended as the bootstrap ends one.
	t.Run("a saver it can't read for its daemon's protocol is ended, and reported stopped", func(t *testing.T) {
		daemon := &daemonFake{isUnusable: true}
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{err: refused}, Daemon: daemon})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if want := "Portal's save daemon stopped. Saved sessions and config are untouched at ~/.config/portal/.\n" + removal; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		if daemon.ends != 1 {
			t.Errorf("ends = %d, want one", daemon.ends)
		}
	})

	t.Run("a saver another state directory's is refused in the bootstrap's terms, nothing judged or ended, printing nothing", func(t *testing.T) {
		theirs := &saverhost.OtherStateDirError{StateDir: "/tmp/scratch/state", Other: "/srv/portal/state", PID: 4242}
		daemon := &daemonFake{isUnusable: true}
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{stopped: true, theirs: theirs}, Daemon: daemon})

		stdout, _, err := runRootCmd(t, "uninstall")

		if !errors.Is(err, theirs) {
			t.Fatalf("uninstall = %v, want the refusal", err)
		}
		if want := "couldn't stop Portal's save daemon: " + bootstrap.OtherStateDir(theirs).UserMessage; err.Error() != want {
			t.Errorf("error = %q, want %q", err, want)
		}
		if daemon.asks != 0 || daemon.ends != 0 {
			t.Errorf("the daemon was judged %d times and ended %d, want neither", daemon.asks, daemon.ends)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})

	t.Run("one it can't end fails the command with the way out of it and both reasons, printing nothing", func(t *testing.T) {
		unproved := errors.New("no one process is proved the session's zmx daemon")
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{err: refused}, Daemon: &daemonFake{isUnusable: true, endErr: unproved}})

		stdout, _, err := runRootCmd(t, "uninstall")

		if !errors.Is(err, refused) || !errors.Is(err, unproved) {
			t.Fatalf("uninstall = %v, want the stop's and the end's failures", err)
		}
		if want := "couldn't stop Portal's save daemon: " + bootstrap.SaverLeftMessage + " ("; !strings.HasPrefix(err.Error(), want) {
			t.Errorf("error = %q, want it to begin %q", err, want)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})

	// The stop can't read a daemon on a protocol Portal can't use, so only
	// the end finds it another state directory's: SaverLeftMessage would have
	// the user end that state's saver.
	t.Run("one the end finds another state directory's is refused in the bootstrap's terms, printing nothing", func(t *testing.T) {
		theirs := &saverhost.OtherStateDirError{StateDir: "/tmp/scratch/state", Other: "/srv/portal/state", PID: 4242}
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{err: refused}, Daemon: &daemonFake{isUnusable: true, endErr: theirs}})

		stdout, _, err := runRootCmd(t, "uninstall")

		if want := "couldn't stop Portal's save daemon: " + bootstrap.OtherStateDir(theirs).UserMessage; err == nil || err.Error() != want {
			t.Errorf("uninstall = %v, want %q", err, want)
		}
		if _, fatal := errors.AsType[*bootstrap.FatalError](err); !fatal {
			t.Errorf("uninstall = %v, want the bootstrap's fatal, which Execute prints alone", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})

	// Decision 68: the judgement and the end hold saver.lock as the stop
	// does, so neither can reach a saver a bootstrap makes meanwhile.
	t.Run("unstaged, it judges and ends a saver it can't stop under saver.lock", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		fake := backendtest.New(backend.Session{Name: saver.SessionName, PID: 4242})
		fake.SetReachEnv([]string{"ZMX_DIR=/tmp/zx-uninstall"})
		fake.StopAnswering(saver.SessionName)
		withBackendDeps(t, BackendDeps{Backend: fake})
		var free []string
		daemon := &daemonFake{isUnusable: true}
		daemon.onAsk = func(what string) {
			if !saverLockHeld(t, dir) {
				free = append(free, what)
			}
		}
		withUninstallDeps(t, UninstallDeps{Daemon: daemon})

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if !strings.HasPrefix(stdout.String(), "Portal's save daemon stopped.") {
			t.Errorf("stdout = %q, want the saver reported stopped", stdout)
		}
		if daemon.asks != 1 || daemon.ends != 1 {
			t.Errorf("the daemon was judged %d times and ended %d, want once each", daemon.asks, daemon.ends)
		}
		if len(free) != 0 {
			t.Errorf("%v ran with saver.lock free, want every judgement and end under it", free)
		}
	})

	t.Run("a session backend that can't be bound fails the command the same way", func(t *testing.T) {
		t.Setenv("ZMX_DIR", "relative/zmx")

		stdout, _, err := runRootCmd(t, "uninstall")

		if err == nil || !strings.HasPrefix(err.Error(), "couldn't stop Portal's save daemon: ") {
			t.Errorf("uninstall = %v, want it to begin \"couldn't stop Portal's save daemon: \"", err)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout)
		}
	})

	t.Run("a session backend whose reach names no directory fails the command, reading nothing", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: saver.SessionName, PID: 4242})
		withBackendDeps(t, BackendDeps{Backend: fake})

		_, _, err := runRootCmd(t, "uninstall")

		if err == nil || !strings.HasPrefix(err.Error(), "couldn't stop Portal's save daemon: ") {
			t.Errorf("uninstall = %v, want it to begin \"couldn't stop Portal's save daemon: \"", err)
		}
		if calls := fake.Calls(); len(calls) != 0 {
			t.Errorf("backend calls = %+v, want none", calls)
		}
	})

	t.Run("it keeps the latch, and writes nothing to the state directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		if err := state.WriteLatch(dir, state.Latch{BootID: "4DAA6ADD-0334-49EC-9FC6-8323D691249E", Version: "0.13.0"}); err != nil {
			t.Fatal(err)
		}
		before := dirContents(t, dir)
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{stopped: true}})

		if _, _, err := runRootCmd(t, "uninstall"); err != nil {
			t.Fatalf("uninstall: %v", err)
		}

		if after := dirContents(t, dir); !maps.Equal(before, after) {
			t.Errorf("state directory = %v, want it as it was, %v", after, before)
		}
	})

	t.Run("it runs no bootstrap", func(t *testing.T) {
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: panicRunner{}})
		withUninstallDeps(t, UninstallDeps{Saver: &stopperFake{}})

		if _, _, err := runRootCmd(t, "uninstall"); err != nil {
			t.Fatalf("uninstall: %v", err)
		}
	})

	t.Run("unstaged, it reads the session backend's saver under saver.lock in the state directory", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		fake := backendtest.New()
		fake.SetReachEnv([]string{"ZMX_DIR=/tmp/zx-uninstall"})
		withBackendDeps(t, BackendDeps{Backend: fake})
		const held = 150 * time.Millisecond
		release := holdSaverLock(t, dir)
		time.AfterFunc(held, release)
		start := time.Now()

		stdout, _, err := runRootCmd(t, "uninstall")

		if err != nil {
			t.Fatalf("uninstall: %v", err)
		}
		if took := time.Since(start); took < held {
			t.Errorf("uninstall took %s, want it waiting out saver.lock, held %s", took, held)
		}
		if want := "Portal's save daemon isn't running. Saved sessions and config are untouched at ~/.config/portal/.\n" + removal; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		calls := fake.Calls()
		if len(calls) != 1 || calls[0].Op != backendtest.OpSession || calls[0].Name != saver.SessionName {
			t.Errorf("backend calls = %+v, want one read of %s", calls, saver.SessionName)
		}
	})

	t.Run("unstaged with no state directory, it finds no saver and creates nothing", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "state")
		t.Setenv("PORTAL_STATE_DIR", dir)
		fake := backendtest.New()
		fake.SetReachEnv([]string{"ZMX_DIR=/tmp/zx-uninstall"})
		withBackendDeps(t, BackendDeps{Backend: fake})

		stdout, _, err := runRootCmd(t, "uninstall")

		if want := "Portal's save daemon isn't running. Saved sessions and config are untouched at ~/.config/portal/.\n" + removal; err != nil || stdout.String() != want {
			t.Errorf("uninstall = %q, %v; want %q", stdout, err, want)
		}
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat %s = %v, want it still absent", dir, err)
		}
	})
}

// saverLockHeld reports whether a descriptor holds saver.lock in dir.
func saverLockHeld(t *testing.T, dir string) bool {
	t.Helper()
	f, err := os.Open(saverhost.LockPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	return errors.Is(unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB), unix.EWOULDBLOCK)
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
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("flock saver.lock: %v", err)
	}
	return func() { _ = f.Close() }
}

func TestUninstall_IsBootstrapExempt(t *testing.T) {
	if !bootstrapExempt["uninstall"] {
		t.Fatal("uninstall must be registered in bootstrapExempt")
	}
}

// dirContents is every file under dir and its bytes.
func dirContents(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
