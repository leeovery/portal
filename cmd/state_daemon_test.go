package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// Swaps the tick-loop sub-seam rather than the top-level daemonRunFunc, so
// the production lock-acquire + WritePIDFile head still runs and its side
// effects stay observable.
func withImmediateRun(t *testing.T) **daemonDeps {
	t.Helper()
	holder := new(*daemonDeps)
	withFuncSeam(t, &daemonTickLoopFunc, func(_ context.Context, deps *daemonDeps) error {
		*holder = deps
		return nil
	})
	return holder
}

func TestStateDaemon_WritesPIDFileOnStartup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pid, err := state.ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("daemon.pid = %d; want %d", pid, os.Getpid())
	}
}

func TestStateDaemon_WritesVersionFileOnStartup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	prev := version
	version = "test-1.2.3"
	t.Cleanup(func() { version = prev })

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := state.ReadVersionFile(dir)
	if err != nil {
		t.Fatalf("ReadVersionFile: %v", err)
	}
	if got != "test-1.2.3" {
		t.Errorf("daemon.version = %q; want %q", got, "test-1.2.3")
	}
}

func TestStateDaemon_ClearsStaleSaveRequestedOnStartup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	stalePath := filepath.Join(dir, "save.requested")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("save.requested should be removed on daemon startup; stat err = %v", err)
	}
}

func TestStateDaemon_OverwritesPIDAndVersionAcrossInvocations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	if err := state.WritePIDFile(dir, 42); err != nil {
		t.Fatalf("seed pid: %v", err)
	}
	if err := state.WriteVersionFile(dir, "stale", nil); err != nil {
		t.Fatalf("seed version: %v", err)
	}

	prev := version
	version = "fresh"
	t.Cleanup(func() { version = prev })

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pid, err := state.ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("pid = %d; want %d (must overwrite stale)", pid, os.Getpid())
	}

	got, err := state.ReadVersionFile(dir)
	if err != nil {
		t.Fatalf("ReadVersionFile: %v", err)
	}
	if got != "fresh" {
		t.Errorf("version = %q; want %q (must overwrite stale)", got, "fresh")
	}
}

func TestStateDaemon_CreatesStateDirectoryIfMissing(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "state-not-yet-created")
	t.Setenv("PORTAL_STATE_DIR", dir)

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("state dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("state path is not a directory")
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("state dir mode = %o; want 0700", perm)
	}
}

func TestStateDaemon_OpensLogFileInStateDir(t *testing.T) {
	// Both lines are INFO, which the default WARN threshold filters out.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logPath := filepath.Join(dir, "portal.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("portal.log not created: %v", err)
	}
	if !strings.Contains(string(data), "daemon: lock acquired") {
		t.Errorf("startup log line missing; got:\n%s", data)
	}
}

func TestStateDaemon_DoesNotEmitStartingINFO(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("portal.log not created: %v", err)
	}
	if strings.Contains(string(data), "daemon: starting") {
		t.Errorf("daemon must not emit an uncataloged 'starting' INFO; got:\n%s", data)
	}
	if !strings.Contains(string(data), "daemon: lock acquired") {
		t.Errorf("expected 'daemon: lock acquired' to preserve startup observability; got:\n%s", data)
	}
}

func TestStateDaemon_PassesPreparedDepsToRunFunc(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	holder := withImmediateRun(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	deps := *holder
	if deps == nil {
		t.Fatal("daemonRunFunc not invoked")
	}
	if deps.Dir != dir {
		t.Errorf("deps.Dir = %q; want %q", deps.Dir, dir)
	}
	if deps.Logger == nil {
		t.Error("deps.Logger is nil")
	}
	if deps.Client == nil {
		t.Error("deps.Client is nil")
	}
}

func TestStateDaemon_ShutdownFlushSkippedWhenRestoringSet(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")
	withDaemonLockFileReset(t)

	fc := commandertest.New(t, commandertest.Returns("1", "show-option")).AllowingUnmatched("", nil)
	client := tmux.NewClient(fc)

	withFuncSeam(t, &daemonRunFunc, func(_ context.Context, deps *daemonDeps) error {
		deps.Client = client
		return defaultShutdownFlush(deps)
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logData, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(logData), "flush_completed=false") {
		t.Errorf("expected shutdown flush_completed=false when @portal-restoring set; got:\n%s", logData)
	}
	if strings.Contains(string(logData), "flush_completed=true") {
		t.Errorf("flush should be skipped (flush_completed=false) when @portal-restoring set; got:\n%s", logData)
	}
}

func TestStateDaemon_ShutdownFlushRunsWhenRestoringUnset(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")
	withDaemonLockFileReset(t)

	fc := commandertest.New(t, commandertest.Fails(tmux.ErrOptionNotFound, "show-option")).AllowingUnmatched("", nil)
	client := tmux.NewClient(fc)

	withFuncSeam(t, &daemonRunFunc, func(_ context.Context, deps *daemonDeps) error {
		deps.Client = client
		return defaultShutdownFlush(deps)
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logData, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(logData), "flush_completed=false") {
		t.Errorf("flush should run (flush_completed=true) when @portal-restoring is unset; got:\n%s", logData)
	}
	if !strings.Contains(string(logData), "flush_completed=true") {
		t.Errorf("expected shutdown flush_completed=true when @portal-restoring unset; got:\n%s", logData)
	}
}

func TestStateDaemon_DefaultRunReturnsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	// Stubbed to keep this exercising the run loop's ctx.Done() response rather
	// than flush behaviour.
	withFuncSeam(t, &daemonShutdownFunc, func(_ *daemonDeps) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logger, _ := newCaptureLoggerForComponent(t, "daemon")
	deps := &daemonDeps{Dir: dir, TickerPeriod: time.Hour, Logger: logger}
	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Errorf("defaultDaemonRun returned error after pre-cancelled context: %v", err)
	}
}

func TestStateDaemon_ReturnsErrorWhenStateDirNotWritable(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "state")

	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	t.Setenv("PORTAL_STATE_DIR", dir)
	_ = withImmediateRun(t)

	_, _, err := runRootCmd(t, "state", "daemon")
	if err == nil {
		t.Fatal("expected error when state dir cannot be created, got nil")
	}
}

func TestStateDaemon_StartupLogIncludesVersionAndPID(t *testing.T) {
	// The startup line is INFO, which the default WARN threshold filters out.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "vX.Y.Z")

	prev := version
	version = "vX.Y.Z"
	t.Cleanup(func() { version = prev })

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "daemon: lock acquired") {
		t.Errorf("startup log missing cataloged 'lock acquired' line; got:\n%s", got)
	}
	if !strings.Contains(got, "vX.Y.Z") {
		t.Errorf("startup log missing version; got:\n%s", got)
	}
	if !strings.Contains(got, fmt.Sprintf("pid=%d", os.Getpid())) {
		t.Errorf("startup log missing pid=%d; got:\n%s", os.Getpid(), got)
	}
}

func TestStateDaemon_RunFuncErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	sentinel := errors.New("boom")
	withFuncSeam(t, &daemonRunFunc, func(_ context.Context, _ *daemonDeps) error { return sentinel })

	_, _, err := runRootCmd(t, "state", "daemon")
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func withAcquireDaemonLockFake(t *testing.T, fake func(string) (*os.File, error)) {
	t.Helper()
	withFuncSeam(t, &acquireDaemonLock, fake)
}

// Cleared around the test so a prior successful acquisition does not bleed
// into the post-condition assertions.
func withDaemonLockFileReset(t *testing.T) {
	t.Helper()
	prev := daemonLockFile
	daemonLockFile = nil
	t.Cleanup(func() { daemonLockFile = prev })
}

func TestStateDaemon_AcquiresLockBeforeWritePIDFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	// daemon.pid must not exist while the lock is being acquired, so the fake
	// records whether it did at that instant.
	var pidFileExistsAtLockAcquire bool
	withAcquireDaemonLockFake(t, func(d string) (*os.File, error) {
		if _, err := os.Stat(filepath.Join(d, "daemon.pid")); err == nil {
			pidFileExistsAtLockAcquire = true
		}
		return state.AcquireDaemonLock(d)
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pidFileExistsAtLockAcquire {
		t.Error("daemon.pid existed before acquireDaemonLock was called; lock must precede WritePIDFile")
	}
	if _, err := state.ReadPIDFile(dir); err != nil {
		t.Errorf("ReadPIDFile after success: %v", err)
	}
}

func TestStateDaemon_AcquireLockCalledAfterEnsureDir(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "fresh-state")
	t.Setenv("PORTAL_STATE_DIR", dir)
	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	var dirExistedAtAcquire bool
	withAcquireDaemonLockFake(t, func(d string) (*os.File, error) {
		if info, err := os.Stat(d); err == nil && info.IsDir() {
			dirExistedAtAcquire = true
		}
		return state.AcquireDaemonLock(d)
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !dirExistedAtAcquire {
		t.Error("state directory did not exist at lock-acquire time; EnsureDir must precede AcquireDaemonLock")
	}
}

func TestStateDaemon_ExitsCleanlyWhenLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	called := false
	withFuncSeam(t, &daemonTickLoopFunc, func(_ context.Context, _ *daemonDeps) error {
		called = true
		return nil
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("expected nil error on lock-held; got: %v", err)
	}
	if called {
		t.Error("daemonTickLoopFunc must not be called when lock is held")
	}
}

func TestStateDaemon_DoesNotWritePIDFileWhenLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	withFuncSeam(t, &daemonTickLoopFunc, func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonTickLoopFunc must not be reached on lock-held path")
		return nil
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "daemon.pid")); !os.IsNotExist(err) {
		t.Errorf("daemon.pid must not exist when lock is held; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "daemon.version")); !os.IsNotExist(err) {
		t.Errorf("daemon.version must not exist when lock is held; stat err = %v", err)
	}
}

func TestStateDaemon_DoesNotOverwritePIDFileWhenLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if err := state.WritePIDFile(dir, 9999); err != nil {
		t.Fatalf("seed pid: %v", err)
	}

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	withFuncSeam(t, &daemonTickLoopFunc, func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonTickLoopFunc must not be reached on lock-held path")
		return nil
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := state.ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("ReadPIDFile: %v", err)
	}
	if got != 9999 {
		t.Errorf("daemon.pid = %d; want 9999 (must not be overwritten by loser)", got)
	}
}

func TestStateDaemon_ReturnsErrorAndLogsWarnOnNonContentionLockFailure(t *testing.T) {
	// Set explicitly rather than depending on the env default.
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")
	withDaemonLockFileReset(t)

	sentinel := errors.New("flock: permission denied")
	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, sentinel
	})

	withFuncSeam(t, &daemonTickLoopFunc, func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonTickLoopFunc must not be reached when lock acquire fails")
		return nil
	})

	_, _, err := runRootCmd(t, "state", "daemon")
	if err == nil {
		t.Fatal("expected error on non-EWOULDBLOCK lock failure, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel error; got %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "daemon.pid")); !os.IsNotExist(err) {
		t.Errorf("daemon.pid must not exist on lock-error path; stat err = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "WARN") {
		t.Errorf("expected a WARN log line; got:\n%s", got)
	}
	if !strings.Contains(got, "acquire daemon lock") {
		t.Errorf("expected lock-acquire error log content; got:\n%s", got)
	}
	var matches int
	for line := range strings.SplitSeq(got, "\n") {
		if strings.Contains(line, "WARN") && strings.Contains(line, "acquire daemon lock") {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("expected exactly one WARN line containing %q; got %d in:\n%s",
			"acquire daemon lock", matches, got)
	}
}

func TestStateDaemon_RetainsLockFdAcrossDaemonLifetime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if daemonLockFile == nil {
		t.Fatal("daemonLockFile package-level var must be non-nil after a successful daemon RunE")
	}
	// The retained fd must still be open — closing it would release the flock.
	if _, err := daemonLockFile.Stat(); err != nil {
		t.Errorf("retained lock fd appears closed: %v", err)
	}
}

func TestStateDaemon_EmitsWarnOnLockContention(t *testing.T) {
	// Set explicitly rather than depending on the env default.
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")
	withDaemonLockFileReset(t)

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	withFuncSeam(t, &daemonTickLoopFunc, func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonTickLoopFunc must not be reached on lock-held path")
		return nil
	})

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "WARN") {
		t.Errorf("expected a WARN log line; got:\n%s", got)
	}
	if !strings.Contains(got, "another daemon holds the lock") {
		t.Errorf("expected contention log content; got:\n%s", got)
	}
	if n := strings.Count(got, "another daemon holds the lock"); n != 1 {
		t.Errorf("expected exactly one contention WARN line; got %d in:\n%s", n, got)
	}
}

func TestSelfSupervisionHysteresisTicks_ClampInvariant(t *testing.T) {
	if selfSupervisionHysteresisTicks < 3 {
		t.Errorf("selfSupervisionHysteresisTicks=%d below clamp floor of 3 "+
			"(N=1 would risk a single-tmux-hiccup false-positive self-eject)",
			selfSupervisionHysteresisTicks)
	}
	if selfSupervisionHysteresisTicks > 9 {
		t.Errorf("selfSupervisionHysteresisTicks=%d above clamp ceiling of 9 "+
			"(a measured max × 2 above 9 indicates an upstream defect, "+
			"not a tuning-knob increase)",
			selfSupervisionHysteresisTicks)
	}
}

func TestDefaultSaverMembershipProbe(t *testing.T) {
	t.Run("it returns false when HasSession is false", func(t *testing.T) {
		fc := commandertest.New(t,
			commandertest.Fails(fmt.Errorf("exit status 1"), "has-session"),
		).AllowingUnmatched("", nil)
		client := tmux.NewClient(fc)

		if defaultSaverMembershipProbe(client, os.Getpid()) {
			t.Errorf("probe = true, want false when HasSession returns false")
		}
		for _, call := range fc.Calls() {
			if len(call) >= 1 && call[0] == "list-panes" {
				t.Errorf("list-panes invoked despite HasSession false; calls = %v", fc.Calls())
			}
		}
	})

	t.Run("it returns false when SaverPanePID errors", func(t *testing.T) {
		fc := commandertest.New(t,
			commandertest.Returns("", "has-session"),
			commandertest.Fails(&tmux.CommandError{
				Stderr: "no such session: _portal-saver",
				Err:    fmt.Errorf("exit status 1"),
			}, "list-panes"),
		).AllowingUnmatched("", nil)
		client := tmux.NewClient(fc)

		if defaultSaverMembershipProbe(client, os.Getpid()) {
			t.Errorf("probe = true, want false when SaverPanePID errors")
		}
	})

	t.Run("it returns true when the pid matches selfPID", func(t *testing.T) {
		const selfPID = 4242
		fc := commandertest.New(t,
			commandertest.Returns("", "has-session"),
			commandertest.Returns(fmt.Sprintf("%d\n", selfPID), "list-panes"),
		).AllowingUnmatched("", nil)
		client := tmux.NewClient(fc)

		if !defaultSaverMembershipProbe(client, selfPID) {
			t.Errorf("probe = false, want true when pid matches selfPID")
		}
	})

	t.Run("it returns false when the pid does not match selfPID", func(t *testing.T) {
		fc := commandertest.New(t,
			commandertest.Returns("", "has-session"),
			commandertest.Returns("9999\n", "list-panes"),
		).AllowingUnmatched("", nil)
		client := tmux.NewClient(fc)

		if defaultSaverMembershipProbe(client, 4242) {
			t.Errorf("probe = true, want false when pid != selfPID (orphan daemon)")
		}
	})
}

func TestStateDaemon_HooksCleanupWiring(t *testing.T) {
	t.Run("it builds the hook store from loadHookStore at startup", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		hooksFileInTempDir(t, nil)

		holder := withImmediateRun(t)
		withDaemonLockFileReset(t)

		if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		deps := *holder
		if deps == nil {
			t.Fatal("daemon deps not captured")
		}
		if deps.HookStore == nil {
			t.Fatal("deps.HookStore is nil; want a non-nil store built from loadHookStore()")
		}
	})

	t.Run("it initialises lastCleanup to a non-zero start time", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		hooksFileInTempDir(t, nil)

		holder := withImmediateRun(t)
		withDaemonLockFileReset(t)

		if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		now := time.Now()

		deps := *holder
		if deps == nil {
			t.Fatal("daemon deps not captured")
		}
		if deps.lastCleanup.IsZero() {
			t.Fatal("deps.lastCleanup is the zero time.Time; want the daemon-start instant so the first cleanup fires one interval after start, not on the first idle tick")
		}
		// Loosely bounded to absorb scheduling jitter: lastCleanup is set a hair
		// before this capture.
		if delta := now.Sub(deps.lastCleanup); delta < 0 || delta > 2*time.Second {
			t.Errorf("deps.lastCleanup = %v; want within 2s of %v (delta %v)", deps.lastCleanup, now, delta)
		}
	})

	t.Run("it resolves the same hooks.json path foreground commands use", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		hooksPath := filepath.Join(t.TempDir(), "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

		// Seeded through the same path a foreground `portal hook set` resolves, so a
		// daemon store pointed at a different file would fail to read it back.
		const key = "proj-AbC123:0.0"
		if err := hooks.NewStore(hooksPath).Set(key, "on-resume", "echo hi", hooks.ViaCLI); err != nil {
			t.Fatalf("seed hooks.json: %v", err)
		}

		holder := withImmediateRun(t)
		withDaemonLockFileReset(t)

		if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		deps := *holder
		if deps == nil {
			t.Fatal("daemon deps not captured")
		}
		loaded, err := deps.HookStore.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("deps.HookStore.Load: %v", err)
		}
		events, ok := loaded[key]
		if !ok {
			t.Fatalf("daemon hook store did not resolve the foreground hooks.json; loaded=%v", loaded)
		}
		if got := events["on-resume"].Command; got != "echo hi" {
			t.Errorf("on-resume command = %q; want %q", got, "echo hi")
		}
	})

	// A loadHookStore failure must not abort the daemon: capture cannot be gated
	// on the best-effort cleanup store, so the failure only disables cleanup.
	// With PORTAL_HOOKS_FILE unset the path resolution reduces to
	// os.UserHomeDir(), which blanking $HOME fails deterministically on darwin;
	// PORTAL_STATE_DIR still drives EnsureDir, so only that branch is perturbed.
	t.Run("it disables cleanup with a WARN rather than aborting the daemon on a loadHookStore error", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		t.Setenv("PORTAL_HOOKS_FILE", "") // force fall-through to home-dir resolution
		t.Setenv("HOME", "")              // os.UserHomeDir() now errors → loadHookStore errors

		sink := logtest.Install(t)

		holder := withImmediateRun(t)
		withDaemonLockFileReset(t)

		if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
			t.Fatalf("RunE must proceed to the tick loop despite loadHookStore failure; got error: %v", err)
		}

		deps := *holder
		if deps == nil {
			t.Fatal("daemon deps not captured")
		}
		if deps.HookStore != nil {
			t.Errorf("deps.HookStore = %v; want nil (cleanup disabled on loadHookStore failure)", deps.HookStore)
		}

		body := sink.Body()
		const warnMsg = "load hook store failed; hooks stale-cleanup disabled"
		if !strings.Contains(body, warnMsg) {
			t.Errorf("expected disabled-cleanup WARN %q; got:\n%s", warnMsg, body)
		}
		if !strings.Contains(body, "component=daemon") {
			t.Errorf("expected the disabled-cleanup WARN under the daemon component; got:\n%s", body)
		}
		if n := strings.Count(body, warnMsg); n != 1 {
			t.Errorf("expected exactly one disabled-cleanup WARN; got %d in:\n%s", n, body)
		}
	})
}

func TestSaverMembershipProbeSeam_DefaultsToProduction(t *testing.T) {
	// Function values are not comparable in Go, so the seam is exercised
	// behaviourally: a mis-wired one would short HasSession or return true on a
	// pid mismatch.
	const selfPID = 4242
	fc := commandertest.New(t,
		commandertest.Returns("", "has-session"),
		commandertest.Returns(fmt.Sprintf("%d\n", selfPID), "list-panes"),
	).AllowingUnmatched("", nil)
	client := tmux.NewClient(fc)

	if !saverMembershipProbe(client, selfPID) {
		t.Errorf("saverMembershipProbe seam returned false; want true (default probe should pass)")
	}
}
