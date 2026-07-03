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

	"github.com/leeovery/portal/internal/hooks"
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

// withImmediateRun installs a daemonTickLoopFunc that returns nil immediately
// and captures the deps it received, so tests can assert on startup side
// effects (including the lock-acquire + WritePIDFile ceremony that now lives
// at the head of defaultDaemonRun). Swapping the tick-loop sub-seam (rather
// than the top-level daemonRunFunc seam) preserves the production acquire+pid
// path so tests observing daemon.pid / daemon.lock side effects see them.
func withImmediateRun(t *testing.T) **daemonDeps {
	t.Helper()
	holder := new(*daemonDeps)
	prev := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, deps *daemonDeps) error {
		*holder = deps
		return nil
	}
	t.Cleanup(func() { daemonTickLoopFunc = prev })
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
	if err := state.WriteVersionFile(dir, "stale", nil); err != nil {
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
	// The cataloged "daemon: lock acquired" INFO marks startup observably; the
	// redundant "daemon: starting" line was dropped per spec § Process/subsystem
	// boundary. Both are INFO, filtered by the default WARN threshold, so bump.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")

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
	if !strings.Contains(string(data), "daemon: lock acquired") {
		t.Errorf("startup log line missing; got:\n%s", data)
	}
}

// TestStateDaemon_DoesNotEmitStartingINFO pins the spec drop: the daemon RunE
// no longer emits the uncataloged "daemon: starting" INFO (spec § Saver and
// daemon lifecycle event taxonomy → Process/subsystem boundary). Startup stays
// observable via "process: start process_role=daemon" + "daemon: lock acquired".
func TestStateDaemon_DoesNotEmitStartingINFO(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")

	_ = withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "portal.log"))
	if err != nil {
		t.Fatalf("portal.log not created: %v", err)
	}
	if strings.Contains(string(data), "daemon: starting") {
		t.Errorf("daemon must not emit an uncataloged 'starting' INFO; got:\n%s", data)
	}
	// Startup observability is preserved by the cataloged lock-acquired event.
	if !strings.Contains(string(data), "daemon: lock acquired") {
		t.Errorf("expected 'daemon: lock acquired' to preserve startup observability; got:\n%s", data)
	}
}

func TestStateDaemon_PassesPreparedDepsToRunFunc(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	holder := withImmediateRun(t)

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
	// The restoring-skip path emits a "shutdown" INFO with flush_completed=false
	// (the breadcrumb "skipping final flush" is now DEBUG). At the INFO level
	// the shutdown line survives the filter and is the observable truth.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")
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
	if !strings.Contains(string(logData), "flush_completed=false") {
		t.Errorf("expected shutdown flush_completed=false when @portal-restoring set; got:\n%s", logData)
	}
	if strings.Contains(string(logData), "flush_completed=true") {
		t.Errorf("flush should be skipped (flush_completed=false) when @portal-restoring set; got:\n%s", logData)
	}
}

func TestStateDaemon_ShutdownFlushRunsWhenRestoringUnset(t *testing.T) {
	// The flush-attempted path emits a "shutdown" INFO with flush_completed=true
	// (the breadcrumb "final flush" is now DEBUG). At the INFO level the
	// shutdown line survives the filter and is the observable truth.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")
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

	// Replace defaultShutdownFlush with a no-op to keep the test
	// hermetic — we are exercising the run loop's response to ctx.Done(),
	// not the flush behavior (covered separately).
	prevFlush := daemonShutdownFunc
	daemonShutdownFunc = func(_ *daemonDeps) error { return nil }
	t.Cleanup(func() { daemonShutdownFunc = prevFlush })

	// defaultDaemonRun now performs the acquire+pid ceremony at its head
	// before entering the tick loop (spec § Component C step 4 — adjacency
	// invariant). The real flock against the tempdir succeeds and pid is
	// written; the pre-cancelled ctx then fires the loop's ctx.Done() case
	// which delegates to the stubbed daemonShutdownFunc.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// defaultDaemonRun now emits a "lock acquired" INFO post-acquire, so the
	// deps need a non-nil Logger (production always sets daemonLogger here).
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

	_, _, err := runStateDaemon(t)
	if err == nil {
		t.Fatal("expected error when state dir cannot be created, got nil")
	}
}

func TestStateDaemon_StartupLogIncludesVersionAndPID(t *testing.T) {
	// version/pid ride as baseline attrs injected by the configured handler onto
	// the cataloged "daemon: lock acquired" startup line (the redundant
	// "daemon: starting" line was dropped per spec). Both are INFO; the default
	// WARN threshold filters INFO, so bump to info.
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "vX.Y.Z")

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

	// daemonTickLoopFunc must NOT be invoked on contention — defaultDaemonRun
	// short-circuits at the acquire-lock err-guard before reaching the loop.
	called := false
	prev := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error {
		called = true
		return nil
	}
	t.Cleanup(func() { daemonTickLoopFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
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

	// daemonTickLoopFunc must NOT be reached on the lock-held path —
	// defaultDaemonRun short-circuits at the acquire-lock err-guard.
	prev := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonTickLoopFunc must not be reached on lock-held path")
		return nil
	}
	t.Cleanup(func() { daemonTickLoopFunc = prev })

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Under the post-T7-5 ordering, neither daemon.pid nor daemon.version is
	// written when the daemon exits on lock contention: defaultDaemonRun now
	// performs acquireDaemonLock → WritePIDFile → WriteVersionFile in that
	// order, so a lock-held early-return at the acquire err-guard short-
	// circuits both writes. This test spot-checks the daemon.pid invariant
	// and (below) the daemon.version invariant; the acquire-then-write
	// adjacency itself is pinned structurally by the T4-8 AST adjacency test.
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

	prev := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonTickLoopFunc must not be reached on lock-held path")
		return nil
	}
	t.Cleanup(func() { daemonTickLoopFunc = prev })

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

func TestStateDaemon_ReturnsErrorAndLogsWarnOnNonContentionLockFailure(t *testing.T) {
	// WARN is the production emission level per Component C spec —
	// non-contention failure mirrors the WARN-on-contention sibling path so
	// the fatal path is not noisier than contention. Set explicitly so we
	// are not depending on the env default.
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	initTestLogToStateDir(t, dir, "test")
	withDaemonLockFileReset(t)

	sentinel := errors.New("flock: permission denied")
	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, sentinel
	})

	prev := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonTickLoopFunc must not be reached when lock acquire fails")
		return nil
	}
	t.Cleanup(func() { daemonTickLoopFunc = prev })

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
	// non-EWOULDBLOCK open(2)/flock failures to emit a WARN-level log line
	// (per Component C, mirroring the WARN-on-contention sibling path so
	// non-contention failure is not noisier than contention). Assert
	// exactly one such line is present so this fatal path is not silent
	// and not noisy.
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
	// Exactly one matching line — the fatal path must not be noisy.
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
	initTestLogToStateDir(t, dir, "test")
	withDaemonLockFileReset(t)

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	prev := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("daemonTickLoopFunc must not be reached on lock-held path")
		return nil
	}
	t.Cleanup(func() { daemonTickLoopFunc = prev })

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

// TestSelfSupervisionHysteresisTicks_ClampInvariant pins the spec
// § Component D acceptance criteria: "A unit test asserts
// selfSupervisionHysteresisTicks >= 1 to prevent accidental zeroing".
// The full clamp envelope (3 ≤ N ≤ 9) is the explicit lower-floor /
// upper-ceiling from the task body; the in-source comment above the
// constant records the per-scenario measurements, and the integration
// harness re-verifies the safety-factor invariant whenever it runs.
//
// This test is the cheap default-lane guard against the
// constant being accidentally edited out of the safe envelope (e.g.
// a refactor that introduces a different value, or a mistaken `var`
// → `const` swap that defaults to zero). Without it, the only check
// would be the integration-tagged harness — which most developer
// runs skip.
func TestSelfSupervisionHysteresisTicks_ClampInvariant(t *testing.T) {
	if selfSupervisionHysteresisTicks < 3 {
		t.Errorf("selfSupervisionHysteresisTicks=%d below clamp floor of 3 "+
			"(spec § Component D rationale: N=1 would risk single-tmux-hiccup "+
			"false-positive self-eject; spec floor is 3)",
			selfSupervisionHysteresisTicks)
	}
	if selfSupervisionHysteresisTicks > 9 {
		t.Errorf("selfSupervisionHysteresisTicks=%d above clamp ceiling of 9 "+
			"(spec § Risk Summary: max × 2 > 9 indicates upstream defect, "+
			"not a tuning-knob increase)",
			selfSupervisionHysteresisTicks)
	}
}

// membershipFakeCommander scripts the two tmux subprocesses the
// defaultSaverMembershipProbe issues per call: `has-session` and `list-panes`.
// It records all args for diagnostic prints on failure but assertions key off
// the returned bool, so call-shape pinning lives in the SaverPanePID test
// suite, not here.
//
// hasSessionErr models the HasSession failure branch (non-zero tmux exit on
// `has-session -t =<name>`). listOutput / listErr drive SaverPanePID.
type membershipFakeCommander struct {
	calls         [][]string
	hasSessionErr error
	listPanesOut  string
	listPanesErr  error
}

func (f *membershipFakeCommander) Run(args ...string) (string, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if len(args) >= 1 && args[0] == "has-session" {
		if f.hasSessionErr != nil {
			return "", f.hasSessionErr
		}
		return "", nil
	}
	if len(args) >= 1 && args[0] == "list-panes" {
		return f.listPanesOut, f.listPanesErr
	}
	return "", nil
}

func (f *membershipFakeCommander) RunRaw(args ...string) (string, error) {
	return f.Run(args...)
}

// TestDefaultSaverMembershipProbe pins the four observable shapes of the
// production probe that Component D's tick-loop integration (Task 5-3) will
// consume:
//
//  1. HasSession false → false. SaverPanePID is never invoked.
//  2. HasSession true, SaverPanePID errors → false. The spec mandates "treat
//     any error as absent" so the daemon's hysteresis counter increments
//     uniformly across race-induced ErrNoSuchSession, ErrEmptyPaneList,
//     ErrPanePIDParse, and generic exec failures.
//  3. HasSession true, SaverPanePID returns pid == selfPID → true. The
//     legitimate daemon's happy path; counter resets in Task 5-3.
//  4. HasSession true, SaverPanePID returns pid != selfPID → false. The
//     orphan-daemon condition — bound by Component A/B sweeps at bootstrap
//     and by Component D's self-eject between bootstraps.
//
// The probe is not invoked from the tick loop in this task — Task 5-3 owns
// that integration.
func TestDefaultSaverMembershipProbe(t *testing.T) {
	t.Run("it returns false when HasSession is false", func(t *testing.T) {
		fc := &membershipFakeCommander{hasSessionErr: fmt.Errorf("exit status 1")}
		client := tmux.NewClient(fc)

		if defaultSaverMembershipProbe(client, os.Getpid()) {
			t.Errorf("probe = true, want false when HasSession returns false")
		}
		// SaverPanePID must not have been invoked — short-circuit on HasSession.
		for _, call := range fc.calls {
			if len(call) >= 1 && call[0] == "list-panes" {
				t.Errorf("list-panes invoked despite HasSession false; calls = %v", fc.calls)
			}
		}
	})

	t.Run("it returns false when SaverPanePID errors", func(t *testing.T) {
		// HasSession returns nil → present; SaverPanePID then sees a
		// "no such session" stderr (the documented race) — probe must
		// classify as absent.
		fc := &membershipFakeCommander{
			listPanesErr: &tmux.CommandError{
				Stderr: "no such session: _portal-saver",
				Err:    fmt.Errorf("exit status 1"),
			},
		}
		client := tmux.NewClient(fc)

		if defaultSaverMembershipProbe(client, os.Getpid()) {
			t.Errorf("probe = true, want false when SaverPanePID errors")
		}
	})

	t.Run("it returns true when the pid matches selfPID", func(t *testing.T) {
		const selfPID = 4242
		fc := &membershipFakeCommander{listPanesOut: fmt.Sprintf("%d\n", selfPID)}
		client := tmux.NewClient(fc)

		if !defaultSaverMembershipProbe(client, selfPID) {
			t.Errorf("probe = false, want true when pid matches selfPID")
		}
	})

	t.Run("it returns false when the pid does not match selfPID", func(t *testing.T) {
		fc := &membershipFakeCommander{listPanesOut: "9999\n"}
		client := tmux.NewClient(fc)

		if defaultSaverMembershipProbe(client, 4242) {
			t.Errorf("probe = true, want false when pid != selfPID (orphan daemon)")
		}
	})
}

// TestStateDaemon_HooksCleanupWiring pins task 3-1: the daemon RunE must carry
// a *hooks.Store built once from loadHookStore() (resolving the SAME hooks.json
// foreground commands mutate) plus a lastCleanup throttle anchor initialised to
// the daemon-start instant. Both are the inputs the throttled daemon-owned
// hooks stale-cleanup gate (tasks 3-2/3-3) will consume; this task wires the
// fields only. withImmediateRun short-circuits the tick loop, so no daemon
// subprocess is spawned (these are in-process unit tests, not the
// IsolateStateForTest daemon-spawning class).
func TestStateDaemon_HooksCleanupWiring(t *testing.T) {
	t.Run("it builds the hook store from loadHookStore at startup", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		holder := withImmediateRun(t)
		withDaemonLockFileReset(t)

		if _, _, err := runStateDaemon(t); err != nil {
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
		t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

		holder := withImmediateRun(t)
		withDaemonLockFileReset(t)

		if _, _, err := runStateDaemon(t); err != nil {
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
		// Loosely bounded to absorb CI scheduling jitter: lastCleanup is set to
		// time.Now() inside RunE, a hair before this post-run capture.
		if delta := now.Sub(deps.lastCleanup); delta < 0 || delta > 2*time.Second {
			t.Errorf("deps.lastCleanup = %v; want within 2s of %v (delta %v)", deps.lastCleanup, now, delta)
		}
	})

	t.Run("it resolves the same hooks.json path foreground commands use", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		hooksPath := filepath.Join(t.TempDir(), "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

		// Seed one entry through the very path a foreground `portal hooks set`
		// resolves, so a daemon store pointed at a DIFFERENT file would visibly
		// fail to read it back.
		const key = "proj-AbC123:0.0"
		if err := hooks.NewStore(hooksPath).Set(key, "on-resume", "echo hi", "cli"); err != nil {
			t.Fatalf("seed hooks.json: %v", err)
		}

		holder := withImmediateRun(t)
		withDaemonLockFileReset(t)

		if _, _, err := runStateDaemon(t); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		deps := *holder
		if deps == nil {
			t.Fatal("daemon deps not captured")
		}
		loaded, err := deps.HookStore.Load()
		if err != nil {
			t.Fatalf("deps.HookStore.Load(): %v", err)
		}
		events, ok := loaded[key]
		if !ok {
			t.Fatalf("daemon hook store did not resolve the foreground hooks.json; loaded=%v", loaded)
		}
		if got := events["on-resume"]; got != "echo hi" {
			t.Errorf("on-resume command = %q; want %q", got, "echo hi")
		}
	})

	// AMBIGUITY NOTE (task 3-1): loadHookStore() only errors when path
	// resolution fails. With PORTAL_HOOKS_FILE unset that reduces to
	// os.UserHomeDir() failing, which we induce deterministically on this
	// platform (darwin) by blanking $HOME. PORTAL_STATE_DIR still drives
	// EnsureDir, so only the hooks-path branch is perturbed. If a future
	// platform resolved a home dir without $HOME this branch would need the
	// source-inspection fallback the task describes; today the live error is
	// deterministic, so we assert the real RunE surface rather than grepping
	// source.
	t.Run("it surfaces a loadHookStore error rather than silently disabling cleanup", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PORTAL_STATE_DIR", dir)
		t.Setenv("PORTAL_HOOKS_FILE", "") // force fall-through to home-dir resolution
		t.Setenv("HOME", "")              // os.UserHomeDir() now errors → loadHookStore errors

		_ = withImmediateRun(t) // guard: a regression that does NOT error must not spin the real tick loop
		withDaemonLockFileReset(t)

		_, _, err := runStateDaemon(t)
		if err == nil {
			t.Fatal("expected RunE to surface the loadHookStore error; got nil (cleanup would be silently disabled)")
		}
		if !strings.Contains(err.Error(), "load hook store") {
			t.Errorf("error = %v; want it wrapped with %q", err, "load hook store")
		}
	})
}

// TestSaverMembershipProbeSeam_DefaultsToProduction guards the wiring
// invariant: production must reach defaultSaverMembershipProbe through the
// saverMembershipProbe seam. A regression here (e.g., a future refactor that
// silently overrides the default at init time) would break Task 5-3's
// integration without triggering the per-behaviour cases above.
func TestSaverMembershipProbeSeam_DefaultsToProduction(t *testing.T) {
	// Function-value equality is not defined in Go, so we exercise the seam
	// against the same fakeCommander shape the default uses and assert the
	// observable behaviour matches. A mis-wired seam would either short
	// HasSession or return true on a pid mismatch.
	const selfPID = 4242
	fc := &membershipFakeCommander{listPanesOut: fmt.Sprintf("%d\n", selfPID)}
	client := tmux.NewClient(fc)

	if !saverMembershipProbe(client, selfPID) {
		t.Errorf("saverMembershipProbe seam returned false; want true (default probe should pass)")
	}
}
