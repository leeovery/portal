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
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	_ = withImmediateRun(t)

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
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

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
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

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
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	prev := version
	version = "vX.Y.Z"
	t.Cleanup(func() { version = prev })

	_ = withImmediateRun(t)

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

	sentinel := errors.New("boom")
	prev := daemonRunFunc
	daemonRunFunc = func(_ context.Context, _ *daemonDeps) error { return sentinel }
	t.Cleanup(func() { daemonRunFunc = prev })

	_, _, err := runStateDaemon(t)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}
