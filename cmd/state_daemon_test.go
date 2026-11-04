package cmd

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// fakeSaver is the saver's cycle as scripted: each poll and flush counted,
// answering with what the test set.
type fakeSaver struct {
	mu                      sync.Mutex
	polls, flushes          int
	pollResult, flushResult saver.Result
	pollErr, flushErr       error
	// onFlush, when set, answers each flush in place of the script.
	onFlush func(ctx context.Context) (saver.Result, error)
}

func (f *fakeSaver) Poll(context.Context) (saver.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.polls++
	return f.pollResult, f.pollErr
}

func (f *fakeSaver) Flush(ctx context.Context) (saver.Result, error) {
	f.mu.Lock()
	f.flushes++
	onFlush := f.onFlush
	f.mu.Unlock()
	if onFlush != nil {
		return onFlush(ctx)
	}
	return f.flushResult, f.flushErr
}

func (f *fakeSaver) counts() (polls, flushes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls, f.flushes
}

// newTestDaemon is a daemon over a state directory of the test's own and the
// Fake, which holds the saver session running this process, its polls and
// flushes going to s.
func newTestDaemon(t *testing.T, s daemonSaver) (*daemonDeps, *backendtest.Fake) {
	t.Helper()
	fake := backendtest.New(backend.Session{Name: saver.SessionName, PID: os.Getpid()})
	return &daemonDeps{
		Dir:         t.TempDir(),
		Version:     "test",
		Backend:     fake,
		Saver:       s,
		FlushAsking: time.Second,
		FlushBound:  2 * time.Second,
	}, fake
}

// realSaver makes deps's saver the production one, over its Fake and state
// directory.
func realSaver(deps *daemonDeps) {
	deps.Saver = saver.New(saver.Config{Backend: deps.Backend, StateDir: deps.Dir})
}

// userSession is an answering session a minute old, as a poll would claim it.
func userSession(name string, pid int) backend.Session {
	return backend.Session{Name: name, Dir: "/srv/" + name, PID: pid, Created: time.Now().Add(-time.Minute).Truncate(time.Second)}
}

var epoch = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)

func second(n int) time.Time { return epoch.Add(time.Duration(n) * time.Second) }

func TestStateDaemon_StartsHoldingItsLockThenWritingItsPIDAndVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fake := backendtest.New(backend.Session{Name: saver.SessionName, PID: os.Getpid()})
	fake.SetCurrent(saver.SessionName)
	withBackendDeps(t, BackendDeps{Backend: fake})
	sink := logtest.Install(t)
	if err := state.WritePIDFile(dir, 42); err != nil {
		t.Fatal(err)
	}

	var ran *daemonDeps
	var heldWhileRunning bool
	withFuncSeam(t, &daemonTickLoopFunc, func(_ context.Context, deps *daemonDeps) error {
		ran = deps
		_, err := state.AcquireDaemonLock(dir)
		heldWhileRunning = errors.Is(err, state.ErrDaemonLockHeld)
		return nil
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("state daemon: %v", err)
	}

	if ran == nil {
		t.Fatal("the tick loop never ran")
	}
	if !heldWhileRunning {
		t.Error("the tick loop ran without the daemon holding daemon.lock")
	}
	if pid, err := state.ReadPIDFile(dir); err != nil || pid != os.Getpid() {
		t.Errorf("daemon.pid = %d, %v; want this process's %d, overwriting the stale one", pid, err, os.Getpid())
	}
	if got, err := state.ReadVersionFile(dir); err != nil || got != Version() {
		t.Errorf("daemon.version = %q, %v; want %q", got, err, Version())
	}
	rec := sink.Records().Matching("daemon", "lock acquired").AtExactLevel(slog.LevelInfo).Only(t, "lock acquired")
	if got := rec.AttrString(t, "session"); got != saver.SessionName {
		t.Errorf("lock acquired session = %q, want the session the daemon runs in", got)
	}
	if ran.Saver == nil || ran.Ticks == nil || ran.FlushAsking != flushAsking || ran.FlushBound != flushBound {
		t.Errorf("the loop ran with %+v, want a saver, a ticker and the production flush bounds", ran)
	}
	lock, err := state.AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("daemon.lock once the daemon returned: %v, want it released", err)
	}
	_ = lock.Close()
}

func TestStateDaemon_RefusesAnIndexItMustNotTouch(t *testing.T) {
	for name, tc := range map[string]struct {
		index string
		is    error
	}{
		"version 1":      {`{"version": 1, "sessions": []}`, sessionindex.ErrLegacyIndex},
		"corrupt":        {"not an index", nil},
		"a newer schema": {`{"version": 3, "sessions": []}`, nil},
	} {
		t.Run(name+", holding nothing", func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PORTAL_STATE_DIR", dir)
			if err := os.WriteFile(sessionindex.Path(dir), []byte(tc.index), 0o600); err != nil {
				t.Fatal(err)
			}
			withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
			withFuncSeam(t, &daemonTickLoopFunc, func(context.Context, *daemonDeps) error {
				t.Error("the tick loop ran")
				return nil
			})
			sink := logtest.Install(t)

			_, _, err := runRootCmd(t, "state", "daemon")

			if err == nil || tc.is != nil && !errors.Is(err, tc.is) {
				t.Errorf("state daemon = %v, want a refusal", err)
			}
			for _, path := range []string{state.DaemonLock(dir), state.DaemonPID(dir), state.DaemonVersion(dir)} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("%s exists (%v), want the refused daemon holding and writing nothing", filepath.Base(path), err)
				}
			}
			rec := sink.Records().Matching("daemon", "refused to start").AtExactLevel(slog.LevelWarn).Only(t, "the refusal")
			if got := rec.AttrString(t, "path"); got != sessionindex.Path(dir) {
				t.Errorf("refusal path = %q, want sessions.json's", got)
			}
		})
	}
}

func TestDefaultDaemonRun_ASecondDaemonExitsHoldingNothing(t *testing.T) {
	deps, _ := newTestDaemon(t, &fakeSaver{})
	first, err := state.AcquireDaemonLock(deps.Dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	withFuncSeam(t, &daemonTickLoopFunc, func(context.Context, *daemonDeps) error {
		t.Error("the second daemon's tick loop ran")
		return nil
	})
	sink := logtest.Install(t)

	if err := defaultDaemonRun(t.Context(), deps); err != nil {
		t.Errorf("defaultDaemonRun = %v, want a clean exit", err)
	}

	if _, err := os.Stat(state.DaemonPID(deps.Dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("daemon.pid exists (%v), want the first daemon's left alone", err)
	}
	sink.Records().Matching("daemon", "another daemon holds the lock; exiting").AtExactLevel(slog.LevelWarn).Only(t, "the contention WARN")
	if got := sink.Records().Matching("daemon", "lock acquired"); len(got) != 0 {
		t.Errorf("logged %+v, want no lock acquired", got)
	}
}

func TestDefaultDaemonRun_ALockItCannotTakeIsAnError(t *testing.T) {
	deps, _ := newTestDaemon(t, &fakeSaver{})
	injected := errors.New("injected lock failure")
	withFuncSeam(t, &acquireDaemonLock, func(string) (*os.File, error) { return nil, injected })
	withFuncSeam(t, &daemonTickLoopFunc, func(context.Context, *daemonDeps) error {
		t.Error("the tick loop ran without the lock")
		return nil
	})
	sink := logtest.Install(t)

	if err := defaultDaemonRun(t.Context(), deps); !errors.Is(err, injected) {
		t.Errorf("defaultDaemonRun = %v, want the lock's failure", err)
	}

	if _, err := os.Stat(state.DaemonPID(deps.Dir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("daemon.pid exists (%v), want none written without the lock", err)
	}
	sink.Records().Matching("daemon", "acquire daemon lock failed").AtExactLevel(slog.LevelWarn).Only(t, "the lock failure WARN")
}

func TestTick_PollsEveryFiveSecondsOfTicks(t *testing.T) {
	for name, tc := range map[string]struct {
		ticks, polled []int
	}{
		"on a tick a second, the first and every fifth": {
			ticks:  []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			polled: []int{0, 5, 10},
		},
		"ticks a slow poll dropped move the next poll on": {
			ticks:  []int{0, 6, 9, 11, 12},
			polled: []int{0, 6, 11},
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := &fakeSaver{}
			deps, _ := newTestDaemon(t, s)

			var polled []int
			for _, n := range tc.ticks {
				before, _ := s.counts()
				deps.tick(t.Context(), second(n))
				if after, _ := s.counts(); after > before {
					polled = append(polled, n)
				}
			}

			if !reflect.DeepEqual(polled, tc.polled) {
				t.Errorf("polled on ticks %v, want %v", polled, tc.polled)
			}
		})
	}
}

func TestDaemonLoop_PollsOnItsTicksAndFlushesAtShutdown(t *testing.T) {
	s := &fakeSaver{}
	deps, _ := newTestDaemon(t, s)
	ticks := make(chan time.Time)
	deps.Ticks = ticks
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- defaultDaemonTickLoop(ctx, deps) }()

	for n := range 6 {
		ticks <- second(n)
	}
	cancel()

	if err := <-done; err != nil {
		t.Errorf("the loop = %v, want a clean shutdown", err)
	}
	if polls, flushes := s.counts(); polls != 2 || flushes != 1 {
		t.Errorf("polls %d, flushes %d; want polls at 0 s and 5 s, then the final flush", polls, flushes)
	}
}

func TestDaemonPoll_Logs(t *testing.T) {
	t.Run("a poll that did something logs one INFO summary of it, and a quiet one none", func(t *testing.T) {
		deps, fake := newTestDaemon(t, nil)
		realSaver(deps)
		fake.Add(userSession("work", 4101))
		fake.SetHistory("work", []byte("the screen"))
		sink := logtest.Install(t)

		deps.tick(t.Context(), second(0))
		deps.tick(t.Context(), second(5))

		rec := sink.Records().Matching("capture", "poll complete").AtExactLevel(slog.LevelInfo).Only(t, "one poll summary")
		for key, want := range map[string]int64{"entries": 1, "natural_churn": 0, "sessions": 1, "reaped": 0} {
			if got := rec.IntAttr(t, key); got != want {
				t.Errorf("poll complete %s = %d, want %d", key, got, want)
			}
		}
		rec.RequireDuration(t, "took")
	})

	t.Run("a poll standing down for a restore logs nothing above DEBUG", func(t *testing.T) {
		deps, fake := newTestDaemon(t, nil)
		realSaver(deps)
		fake.Add(userSession("work", 4101))
		if err := state.Restoring.Set(deps.Dir); err != nil {
			t.Fatal(err)
		}
		sink := logtest.Install(t)

		deps.tick(t.Context(), second(0))

		if got := sink.Records().AtOrAboveLevel(slog.LevelInfo); len(got) != 0 {
			t.Errorf("logged %+v, want nothing above DEBUG", got)
		}
	})

	t.Run("a failed poll is a WARN naming the stage, and the next poll carries on", func(t *testing.T) {
		deps, fake := newTestDaemon(t, nil)
		realSaver(deps)
		fake.Add(userSession("work", 4101))
		boom := errors.New("zmx dir unreadable")
		fake.Fail(backendtest.OpList, boom)
		sink := logtest.Install(t)

		deps.tick(t.Context(), second(0))
		fake.Fail(backendtest.OpList, nil)
		deps.tick(t.Context(), second(5))

		rec := sink.Records().Matching("daemon", "poll failed").AtExactLevel(slog.LevelWarn).Only(t, "the failed poll's WARN")
		if err := rec.ErrorAttr(t, "error"); !errors.Is(err, boom) {
			t.Errorf("poll failed error = %v, want the listing's", err)
		}
		sink.Records().Matching("capture", "poll complete").Only(t, "the next poll's summary")
	})

	t.Run("a poll the shutdown cut short logs nothing", func(t *testing.T) {
		deps, _ := newTestDaemon(t, &fakeSaver{pollErr: context.Canceled})
		sink := logtest.Install(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		deps.poll(ctx)

		if got := sink.Records().AtOrAboveLevel(slog.LevelInfo); len(got) != 0 {
			t.Errorf("logged %+v, want nothing for a poll the shutdown cut", got)
		}
	})
}

func TestDaemonShutdown(t *testing.T) {
	t.Run("it flushes, then logs why it ended and that the flush completed", func(t *testing.T) {
		for reason, sig := range map[string]os.Signal{"signal": syscall.SIGTERM, "sighup": syscall.SIGHUP, "exit": nil} {
			t.Run(reason, func(t *testing.T) {
				s := &fakeSaver{}
				deps, _ := newTestDaemon(t, s)
				if sig != nil {
					deps.recordShutdownSignal(sig)
				}
				sink := logtest.Install(t)

				if err := deps.shutdown(); err != nil {
					t.Errorf("shutdown = %v, want nil", err)
				}

				if _, flushes := s.counts(); flushes != 1 {
					t.Errorf("flushes = %d, want 1", flushes)
				}
				rec := sink.Records().Matching("daemon", "shutdown").AtExactLevel(slog.LevelInfo).Only(t, "the shutdown line")
				if got := rec.AttrString(t, "reason"); got != reason {
					t.Errorf("shutdown reason = %q, want %q", got, reason)
				}
				if got := rec.AttrString(t, "flush_completed"); got != "true" {
					t.Errorf("flush_completed = %s, want true", got)
				}
				if got := sink.Records().Matching("capture", "flush complete"); len(got) != 0 {
					t.Errorf("logged %+v, want no summary of a flush that saved nothing", got)
				}
			})
		}
	})

	t.Run("a flush that saved something logs one INFO summary", func(t *testing.T) {
		deps, _ := newTestDaemon(t, &fakeSaver{flushResult: saver.Result{Captured: 2}})
		sink := logtest.Install(t)

		_ = deps.shutdown()

		rec := sink.Records().Matching("capture", "flush complete").AtExactLevel(slog.LevelInfo).Only(t, "the flush summary")
		if got := rec.IntAttr(t, "sessions"); got != 2 {
			t.Errorf("flush complete sessions = %d, want 2", got)
		}
		rec.RequireDuration(t, "took")
	})

	t.Run("a flush that failed is a WARN, and the shutdown says it did not complete", func(t *testing.T) {
		deps, _ := newTestDaemon(t, &fakeSaver{flushErr: errors.New("commit lock held")})
		sink := logtest.Install(t)

		_ = deps.shutdown()

		sink.Records().Matching("daemon", "final flush failed").AtExactLevel(slog.LevelWarn).Only(t, "the failed flush's WARN")
		assertFlushCompleted(t, sink, "false")
	})

	t.Run("a restore in progress skips the flush quietly", func(t *testing.T) {
		deps, _ := newTestDaemon(t, &fakeSaver{flushErr: saver.ErrRestoring})
		sink := logtest.Install(t)

		_ = deps.shutdown()

		if got := sink.Records().AtExactLevel(slog.LevelWarn); len(got) != 0 {
			t.Errorf("logged %+v, want no WARN for a restore", got)
		}
		assertFlushCompleted(t, sink, "false")
	})

	t.Run("it asks for histories only within FlushAsking, and waits for the flush no longer than FlushBound", func(t *testing.T) {
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		asking := make(chan time.Duration, 1)
		s := &fakeSaver{onFlush: func(ctx context.Context) (saver.Result, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				deadline = time.Now().Add(time.Hour)
			}
			asking <- time.Until(deadline)
			<-release
			return saver.Result{}, nil
		}}
		deps, _ := newTestDaemon(t, s)
		deps.FlushAsking, deps.FlushBound = 20*time.Millisecond, 50*time.Millisecond
		sink := logtest.Install(t)

		start := time.Now()
		_ = deps.shutdown()

		if took := time.Since(start); took > time.Second {
			t.Errorf("shutdown took %s, want it back once the 50ms bound ran out", took)
		}
		if left := <-asking; left > deps.FlushAsking {
			t.Errorf("the flush could ask for %s, want no longer than FlushAsking", left)
		}
		rec := sink.Records().Matching("daemon", "final flush failed").AtExactLevel(slog.LevelWarn).Only(t, "the overrun's WARN")
		if got := rec.AttrString(t, "error"); !strings.Contains(got, "bound") {
			t.Errorf("final flush failed error = %q, want it to name the bound", got)
		}
		assertFlushCompleted(t, sink, "false")
	})

	t.Run("the saver's flush writes and names a changed session's scrollback", func(t *testing.T) {
		deps, fake := newTestDaemon(t, nil)
		realSaver(deps)
		work := userSession("work", 4101)
		fake.Add(work)
		fake.SetHistory("work", []byte("the first screen"))
		deps.tick(t.Context(), second(0))
		fake.SetHistory("work", []byte("the last screen"))
		sink := logtest.Install(t)

		_ = deps.shutdown()

		file, err := state.SessionScrollbackFile(work.Name, work.Created, work.PID)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(filepath.Join(deps.Dir, filepath.FromSlash(file))); err != nil || string(got) != "the last screen" {
			t.Errorf("work's scrollback = %q, %v; want the last screen", got, err)
		}
		sink.Records().Matching("capture", "flush complete").Only(t, "the flush summary")
		assertFlushCompleted(t, sink, "true")
	})
}

func assertFlushCompleted(t *testing.T, sink *logtest.Sink, want string) {
	t.Helper()
	rec := sink.Records().Matching("daemon", "shutdown").Only(t, "the shutdown line")
	if got := rec.AttrString(t, "flush_completed"); got != want {
		t.Errorf("flush_completed = %s, want %s", got, want)
	}
}

func TestShutdownReason_MapsSignalsAndDefaultsToExit(t *testing.T) {
	for want, sig := range map[string]os.Signal{"sighup": syscall.SIGHUP, "signal": syscall.SIGTERM, "exit": syscall.SIGINT} {
		deps := &daemonDeps{}
		deps.recordShutdownSignal(sig)
		if got := deps.shutdownReason(); got != want {
			t.Errorf("shutdownReason after %v = %q; want %q", sig, got, want)
		}
	}
	if got := (&daemonDeps{}).shutdownReason(); got != "exit" {
		t.Errorf("shutdownReason with no signal = %q; want exit", got)
	}
}

func TestMaybeCleanProjects(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")

	t.Run("it prunes a gone project an interval after the last prune, and not before", func(t *testing.T) {
		store, _ := seedProjectsJSON(t, gone)
		deps := &daemonDeps{Projects: store, lastProjectCleanup: epoch}

		deps.maybeCleanProjects(epoch.Add(projectCleanupInterval - time.Second))
		if got := projectPaths(t, store); len(got) != 1 {
			t.Errorf("projects = %v before the interval, want the gone one kept", got)
		}

		deps.maybeCleanProjects(epoch.Add(projectCleanupInterval))
		if got := projectPaths(t, store); len(got) != 0 {
			t.Errorf("projects = %v once the interval passed, want the gone one pruned", got)
		}
		if want := epoch.Add(projectCleanupInterval); !deps.lastProjectCleanup.Equal(want) {
			t.Errorf("last prune = %v, want %v", deps.lastProjectCleanup, want)
		}
	})

	t.Run("a prune that fails is a WARN, and waits an interval like any other", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "projects.json")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		deps := &daemonDeps{Projects: project.NewStore(path), lastProjectCleanup: epoch}
		sink := logtest.Install(t)

		at := epoch.Add(projectCleanupInterval)
		deps.maybeCleanProjects(at)

		sink.Records().Matching("daemon", "projects stale-cleanup failed").AtExactLevel(slog.LevelWarn).Only(t, "the failed prune's WARN")
		if !deps.lastProjectCleanup.Equal(at) {
			t.Errorf("last prune = %v, want %v", deps.lastProjectCleanup, at)
		}
	})

	t.Run("it runs on an idle tick, never on one that polls", func(t *testing.T) {
		store, _ := seedProjectsJSON(t, gone)
		deps, _ := newTestDaemon(t, &fakeSaver{})
		deps.Projects, deps.lastProjectCleanup = store, epoch

		deps.tick(t.Context(), epoch.Add(projectCleanupInterval))
		if got := projectPaths(t, store); len(got) != 1 {
			t.Errorf("projects = %v after a polling tick, want the prune left to an idle one", got)
		}

		deps.tick(t.Context(), epoch.Add(projectCleanupInterval+time.Second))
		if got := projectPaths(t, store); len(got) != 0 {
			t.Errorf("projects = %v after an idle tick, want the gone one pruned", got)
		}
	})
}

func projectPaths(t *testing.T, store *project.Store) []string {
	t.Helper()
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load projects: %v", err)
	}
	var paths []string
	for _, p := range loaded {
		paths = append(paths, p.Path)
	}
	return paths
}

func TestOsExitIsPoisonedPackageWide(t *testing.T) {
	if reflect.ValueOf(osExit).Pointer() == reflect.ValueOf(os.Exit).Pointer() {
		t.Error("osExit is os.Exit: a test reaching it unfaked would end the test binary")
	}
}
