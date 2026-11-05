package migrate_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/state"
)

// daemonPID is the pid the fixtures' daemon.pid names.
const daemonPID = 4242

// fakeClock is a clock a sleep moves on.
type fakeClock struct {
	now   time.Time
	start time.Time
	naps  int
}

func newFakeClock() *fakeClock {
	start := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	return &fakeClock{now: start, start: start}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.now = c.now.Add(d)
	c.naps++
	return nil
}

func (c *fakeClock) elapsed() time.Duration { return c.now.Sub(c.start) }

// reading is one answer Identify gives.
type reading struct {
	result state.IdentifyResult
	err    error
}

var (
	readRunning = reading{result: state.IdentifyIsPortalDaemon}
	readDead    = reading{result: state.IdentifyDead}
	readOther   = reading{result: state.IdentifyNotPortalDaemon}
	readFailing = reading{err: errors.New("ps: exit status 1")}
)

// daemonWorld is the tmux version's state dir, its daemon read by the
// answers of identify, on a fake clock.
type daemonWorld struct {
	dir      string
	clock    *fakeClock
	identify func(elapsed time.Duration) reading
	asked    []int
	told     []int
}

func newDaemonWorld(t *testing.T, pidFile string, identify func(elapsed time.Duration) reading) *daemonWorld {
	t.Helper()
	w := &daemonWorld{dir: t.TempDir(), clock: newFakeClock(), identify: identify}
	if pidFile != "" {
		writeFile(t, state.DaemonPID(w.dir), pidFile)
	}
	return w
}

func (w *daemonWorld) daemon() migrate.Daemon {
	return migrate.Daemon{
		Identify: func(pid int) (state.IdentifyResult, error) {
			w.asked = append(w.asked, pid)
			r := w.identify(w.clock.elapsed())
			return r.result, r.err
		},
		Sleep: w.clock.Sleep,
		Now:   w.clock.Now,
	}
}

func (w *daemonWorld) await(ctx context.Context) (int, error) {
	return w.daemon().AwaitExit(ctx, w.dir, func(pid int) { w.told = append(w.told, pid) })
}

// always answers r on every read.
func always(r reading) func(time.Duration) reading {
	return func(time.Duration) reading { return r }
}

// until answers before until elapsed reaches at, then after.
func until(at time.Duration, before, after reading) func(time.Duration) reading {
	return func(elapsed time.Duration) reading {
		if elapsed < at {
			return before
		}
		return after
	}
}

var pidFile = strconv.Itoa(daemonPID) + "\n"

func TestAwaitExit(t *testing.T) {
	t.Run("it waits for nothing when daemon.pid is absent", func(t *testing.T) {
		w := newDaemonWorld(t, "", always(readRunning))

		pid, err := w.await(context.Background())

		if pid != 0 || err != nil || len(w.asked) != 0 {
			t.Errorf("AwaitExit = %d, %v after asking about %v; want 0, nil, no process read", pid, err, w.asked)
		}
	})

	t.Run("it waits for nothing when daemon.pid isn't a number", func(t *testing.T) {
		w := newDaemonWorld(t, "not a pid\n", always(readRunning))

		pid, err := w.await(context.Background())

		if pid != 0 || err != nil || len(w.asked) != 0 {
			t.Errorf("AwaitExit = %d, %v after asking about %v; want 0, nil, no process read", pid, err, w.asked)
		}
	})

	t.Run("it fails on a daemon.pid it can't read", func(t *testing.T) {
		w := newDaemonWorld(t, "", always(readRunning))
		mkdir(t, state.DaemonPID(w.dir))

		pid, err := w.await(context.Background())

		if err == nil || errors.Is(err, migrate.ErrDaemonRunning) || len(w.asked) != 0 {
			t.Errorf("AwaitExit = %d, %v after asking about %v; want a read error, no process read", pid, err, w.asked)
		}
	})

	for name, r := range map[string]reading{
		"names no process":      readDead,
		"isn't a portal daemon": readOther,
	} {
		t.Run("it counts a pid that "+name+" as exited, at once", func(t *testing.T) {
			w := newDaemonWorld(t, pidFile, always(r))

			pid, err := w.await(context.Background())

			if pid != daemonPID || err != nil {
				t.Fatalf("AwaitExit = %d, %v; want %d, nil", pid, err, daemonPID)
			}
			if w.clock.naps != 0 || len(w.told) != 0 {
				t.Errorf("slept %d times and told %v; want no wait at all", w.clock.naps, w.told)
			}
		})
	}

	t.Run("it waits for a running daemon to exit, telling of the wait once", func(t *testing.T) {
		w := newDaemonWorld(t, pidFile, until(2*time.Second, readRunning, readDead))

		pid, err := w.await(context.Background())

		if pid != daemonPID || err != nil {
			t.Fatalf("AwaitExit = %d, %v; want %d, nil", pid, err, daemonPID)
		}
		if w.clock.elapsed() != 2*time.Second {
			t.Errorf("returned after %v; want the 2s the daemon took", w.clock.elapsed())
		}
		if len(w.told) != 1 || w.told[0] != daemonPID {
			t.Errorf("told %v; want the pid once", w.told)
		}
		for _, asked := range w.asked {
			if asked != daemonPID {
				t.Errorf("asked about pid %d; want only %d", asked, daemonPID)
			}
		}
	})

	t.Run("it counts a pid reused by another process as exited", func(t *testing.T) {
		w := newDaemonWorld(t, pidFile, until(time.Second, readRunning, readOther))

		pid, err := w.await(context.Background())

		if pid != daemonPID || err != nil || w.clock.elapsed() != time.Second {
			t.Errorf("AwaitExit = %d, %v after %v; want %d, nil after 1s", pid, err, w.clock.elapsed(), daemonPID)
		}
	})

	t.Run("it still waits just short of the bound", func(t *testing.T) {
		w := newDaemonWorld(t, pidFile, until(migrate.DaemonBound-50*time.Millisecond, readRunning, readDead))

		pid, err := w.await(context.Background())

		if pid != daemonPID || err != nil {
			t.Errorf("AwaitExit = %d, %v; want %d, nil: the daemon exited inside the bound", pid, err, daemonPID)
		}
	})

	t.Run("it refuses a daemon still running at the bound", func(t *testing.T) {
		w := newDaemonWorld(t, pidFile, always(readRunning))

		pid, err := w.await(context.Background())

		if !errors.Is(err, migrate.ErrDaemonRunning) {
			t.Fatalf("AwaitExit = %d, %v; want ErrDaemonRunning", pid, err)
		}
		if w.clock.elapsed() != migrate.DaemonBound {
			t.Errorf("refused after %v; want at the bound, %v", w.clock.elapsed(), migrate.DaemonBound)
		}
		want := "The tmux version's save daemon (pid 4242) is still running 30s after its saver was stopped, so nothing was migrated. Run portal migrate again once it has exited."
		if err.Error() != want {
			t.Errorf("refusal = %q, want %q", err, want)
		}
		if len(w.told) != 1 {
			t.Errorf("told %v; want the pid once", w.told)
		}
	})

	t.Run("it refuses a daemon it can't read at the bound, saying why", func(t *testing.T) {
		w := newDaemonWorld(t, pidFile, always(readFailing))

		_, err := w.await(context.Background())

		if !errors.Is(err, migrate.ErrDaemonRunning) || w.clock.elapsed() != migrate.DaemonBound {
			t.Fatalf("AwaitExit = %v after %v; want ErrDaemonRunning at the bound", err, w.clock.elapsed())
		}
		if !strings.Contains(err.Error(), ", or Portal can't tell it has exited (ps: exit status 1)") {
			t.Errorf("refusal = %q; want it saying the process couldn't be read, and why", err)
		}
		if len(w.told) != 0 {
			t.Errorf("told %v; want nothing told of a daemon never read running", w.told)
		}
	})

	t.Run("it reads again after a failed read", func(t *testing.T) {
		w := newDaemonWorld(t, pidFile, until(time.Second, readFailing, readDead))

		pid, err := w.await(context.Background())

		if pid != daemonPID || err != nil {
			t.Errorf("AwaitExit = %d, %v; want %d, nil", pid, err, daemonPID)
		}
	})

	t.Run("it stops waiting when its context ends", func(t *testing.T) {
		w := newDaemonWorld(t, pidFile, always(readRunning))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		daemon := w.daemon()
		daemon.Sleep = nil

		_, err := daemon.AwaitExit(ctx, w.dir, func(int) {})

		if !errors.Is(err, context.Canceled) || errors.Is(err, migrate.ErrDaemonRunning) {
			t.Errorf("AwaitExit = %v; want the context's end", err)
		}
	})

	t.Run("it proves the pid by the process table by default", func(t *testing.T) {
		w := newDaemonWorld(t, strconv.Itoa(os.Getpid()), nil)

		pid, err := migrate.Daemon{}.AwaitExit(context.Background(), w.dir, func(int) {})

		if pid != os.Getpid() || err != nil {
			t.Errorf("AwaitExit = %d, %v; want the test binary, no daemon, read as exited", pid, err)
		}
	})
}
