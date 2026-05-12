// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// runStateDaemon executes "portal state daemon" with stdout/stderr captured.
func runStateDaemon(t *testing.T) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	resetStateCmdFlags()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"state", "daemon"})
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

// withImmediateRun installs a daemonRunFunc that returns nil immediately and
// captures the deps it received, so tests can assert on startup side effects.
func withImmediateRun(t *testing.T) **daemonDeps {
	t.Helper()
	holder := new(*daemonDeps)
	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, deps *daemonDeps) error {
		*holder = deps
		return nil
	}
	t.Cleanup(func() { daemonRunFunc = prev })
	return holder
}

func TestStateDaemon_WritesPIDFileOnStartup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
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

	if _, _, err := runStateDaemon(t); err != nil {
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

	// Pre-create save.requested as if a prior daemon left it behind.
	stalePath := filepath.Join(dir, "save.requested")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Errorf("save.requested should be removed on daemon startup; stat err = %v", err)
	}
}

func TestStateDaemon_OverwritesPIDAndVersionAcrossInvocations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Seed pre-existing PID and version files with stale values.
	if err := state.WritePIDFile(dir, 42); err != nil {
		t.Fatalf("seed pid: %v", err)
	}
	if err := state.WriteVersionFile(dir, "stale"); err != nil {
		t.Fatalf("seed version: %v", err)
	}

	prev := version
	version = "fresh"
	t.Cleanup(func() { version = prev })

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
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

	if _, _, err := runStateDaemon(t); err != nil {
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
	// "starting, version=..." is INFO; default WARN threshold filters it.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logPath := filepath.Join(dir, "portal.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("portal.log not created: %v", err)
	}
	if !strings.Contains(string(data), "starting") {
		t.Errorf("startup log line missing; got:\n%s", data)
	}
}

func TestStateDaemon_PassesPreparedDepsToRunFunc(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	holder := new(*daemonDeps)
	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, deps *daemonDeps) error {
		*holder = deps
		return nil
	}
	t.Cleanup(func() { daemonRunFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
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

// fakeCommander records tmux invocations and returns scripted responses.
type fakeCommander struct {
	calls    [][]string
	getValue string
	getErr   error
}

func (f *fakeCommander) Run(args ...string) (string, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if len(args) >= 2 && args[0] == "show-option" {
		if f.getErr != nil {
			return "", f.getErr
		}
		return f.getValue, nil
	}
	return "", nil
}

// RunRaw mirrors Run for this fake — daemon shutdown tests don't need a
// distinction between trimmed and raw output.
func (f *fakeCommander) RunRaw(args ...string) (string, error) {
	return f.Run(args...)
}

func TestStateDaemon_ShutdownFlushSkippedWhenRestoringSet(t *testing.T) {
	// "skipping final flush" is INFO; default WARN threshold filters it.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	fc := &fakeCommander{getValue: "1"}
	client := tmux.NewClient(fc)

	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, deps *daemonDeps) error {
		// Override the production client with our fake before shutdown runs.
		deps.Client = client
		return defaultShutdownFlush(deps)
	}
	t.Cleanup(func() { daemonRunFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logData, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(logData), "skipping final flush") {
		t.Errorf("expected skip-flush log entry; got:\n%s", logData)
	}
	if strings.Contains(string(logData), "final flush") &&
		!strings.Contains(string(logData), "skipping final flush") {
		t.Errorf("flush should be skipped when @portal-restoring set; got:\n%s", logData)
	}
}

func TestStateDaemon_ShutdownFlushRunsWhenRestoringUnset(t *testing.T) {
	// "final flush" is INFO; default WARN threshold filters it.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	fc := &fakeCommander{getErr: tmux.ErrOptionNotFound}
	client := tmux.NewClient(fc)

	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, deps *daemonDeps) error {
		deps.Client = client
		return defaultShutdownFlush(deps)
	}
	t.Cleanup(func() { daemonRunFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	logData, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(string(logData), "skipping final flush") {
		t.Errorf("flush should run when @portal-restoring is unset; got:\n%s", logData)
	}
	if !strings.Contains(string(logData), "final flush") {
		t.Errorf("expected final-flush log entry; got:\n%s", logData)
	}
}

func TestStateDaemon_DefaultRunReturnsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Replace defaultShutdownFlush with a no-op to keep the test
	// hermetic — we are exercising the run loop's response to ctx.Done(),
	// not the flush behavior (covered separately).
	prevFlush := daemonShutdownFunc
	daemonShutdownFunc = func(_ *daemonDeps) error { return nil }
	t.Cleanup(func() { daemonShutdownFunc = prevFlush })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deps := &daemonDeps{Dir: dir, TickerPeriod: time.Hour}
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

	_, _, err := runStateDaemon(t)
	if err == nil {
		t.Fatal("expected error when state dir cannot be created, got nil")
	}
}

func TestStateDaemon_StartupLogIncludesVersionAndPID(t *testing.T) {
	// "starting, version=..., pid=..." is INFO; default WARN threshold filters it.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	prev := version
	version = "vX.Y.Z"
	t.Cleanup(func() { version = prev })

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
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
	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, _ *daemonDeps) error { return sentinel }
	t.Cleanup(func() { daemonRunFunc = prev })

	_, _, err := runStateDaemon(t)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// withAcquireDaemonLockFake swaps the package-level acquireDaemonLock seam for
// the duration of the test and restores it via t.Cleanup. Tests must not use
// t.Parallel — the seam is package-level mutable state shared across tests.
func withAcquireDaemonLockFake(t *testing.T, fake func(string) (*os.File, error)) {
	t.Helper()
	prev := acquireDaemonLock
	acquireDaemonLock = fake
	t.Cleanup(func() { acquireDaemonLock = prev })
}

// withDaemonLockFileReset clears the package-level daemonLockFile var around
// the test so a prior test's successful acquisition does not bleed into the
// post-condition assertions of the test under run.
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

	// Order is asserted by recording whether daemon.pid existed at the moment
	// acquireDaemonLock was invoked. The contract is: pidfile MUST NOT exist
	// when the lock is being acquired, because WritePIDFile runs only after a
	// successful lock acquisition.
	var pidFileExistsAtLockAcquire bool
	withAcquireDaemonLockFake(t, func(d string) (*os.File, error) {
		if _, err := os.Stat(filepath.Join(d, "daemon.pid")); err == nil {
			pidFileExistsAtLockAcquire = true
		}
		// Return the real lock-file fd so the rest of startup proceeds normally.
		return state.AcquireDaemonLock(d)
	})

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pidFileExistsAtLockAcquire {
		t.Error("daemon.pid existed before acquireDaemonLock was called; lock must precede WritePIDFile")
	}
	// Sanity: pidfile is written on the success path.
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

	if _, _, err := runStateDaemon(t); err != nil {
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

	// Seam returns ErrDaemonLockHeld → daemon must return nil (exit 0).
	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	// daemonRunFunc must NOT be invoked on contention.
	called := false
	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, _ *daemonDeps) error {
		called = true
		return nil
	}
	t.Cleanup(func() { daemonRunFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("expected nil error on lock-held; got: %v", err)
	}
	if called {
		t.Error("daemonRunFunc must not be called when lock is held")
	}
}

func TestStateDaemon_DoesNotWritePIDFileWhenLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	// No prior runFunc seam set: assert RunE returns before reaching it.
	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonRunFunc must not be reached on lock-held path")
		return nil
	}
	t.Cleanup(func() { daemonRunFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// daemon.pid must NOT have been written.
	if _, err := os.Stat(filepath.Join(dir, "daemon.pid")); !os.IsNotExist(err) {
		t.Errorf("daemon.pid must not exist when lock is held; stat err = %v", err)
	}
	// daemon.version must NOT have been written.
	if _, err := os.Stat(filepath.Join(dir, "daemon.version")); !os.IsNotExist(err) {
		t.Errorf("daemon.version must not exist when lock is held; stat err = %v", err)
	}
}

func TestStateDaemon_DoesNotOverwritePIDFileWhenLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	// Pre-seed daemon.pid with a known stale value the loser must NOT overwrite.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if err := state.WritePIDFile(dir, 9999); err != nil {
		t.Fatalf("seed pid: %v", err)
	}

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonRunFunc must not be reached on lock-held path")
		return nil
	}
	t.Cleanup(func() { daemonRunFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
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

func TestStateDaemon_ReturnsErrorOnNonContentionLockFailure(t *testing.T) {
	// ERROR is above the default WARN threshold, but we set the level
	// explicitly so we are not depending on the env default.
	t.Setenv("PORTAL_LOG_LEVEL", "error")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	sentinel := errors.New("flock: permission denied")
	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, sentinel
	})

	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonRunFunc must not be reached when lock acquire fails")
		return nil
	}
	t.Cleanup(func() { daemonRunFunc = prev })

	_, _, err := runStateDaemon(t)
	if err == nil {
		t.Fatal("expected error on non-EWOULDBLOCK lock failure, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel error; got %v", err)
	}

	// State files must not be written on the fatal error path.
	if _, err := os.Stat(filepath.Join(dir, "daemon.pid")); !os.IsNotExist(err) {
		t.Errorf("daemon.pid must not exist on lock-error path; stat err = %v", err)
	}

	// Spec § Fix Part 1 → Lock-file create/open semantics requires
	// non-EWOULDBLOCK open(2)/flock failures to emit an ERROR-level log
	// line. Mirror the WARN-on-contention sibling test: assert exactly one
	// such line is present so this fatal path is not silent and not noisy.
	data, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "ERROR") {
		t.Errorf("expected an ERROR log line; got:\n%s", got)
	}
	if !strings.Contains(got, "acquire daemon lock") {
		t.Errorf("expected lock-acquire error log content; got:\n%s", got)
	}
	// Exactly one matching line — the fatal path must not be noisy.
	var matches int
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "ERROR") && strings.Contains(line, "acquire daemon lock") {
			matches++
		}
	}
	if matches != 1 {
		t.Errorf("expected exactly one ERROR line containing %q; got %d in:\n%s",
			"acquire daemon lock", matches, got)
	}
}

func TestStateDaemon_RetainsLockFdAcrossDaemonLifetime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if daemonLockFile == nil {
		t.Fatal("daemonLockFile package-level var must be non-nil after a successful daemon RunE")
	}
	// The retained fd must still be open — closing it would release the
	// flock. Probe Fd() returns a positive integer; calling Stat on the
	// underlying file should not error on an open fd.
	if _, err := daemonLockFile.Stat(); err != nil {
		t.Errorf("retained lock fd appears closed: %v", err)
	}
}

func TestStateDaemon_EmitsWarnOnLockContention(t *testing.T) {
	// WARN is above the default INFO threshold, but we set the level
	// explicitly so we are not depending on the env default.
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonRunFunc must not be reached on lock-held path")
		return nil
	}
	t.Cleanup(func() { daemonRunFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
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
	// Exactly one such line — the loser path must not be noisy.
	if n := strings.Count(got, "another daemon holds the lock"); n != 1 {
		t.Errorf("expected exactly one contention WARN line; got %d in:\n%s", n, got)
	}
}
