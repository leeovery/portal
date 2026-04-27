// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// runStateNotify executes "portal state notify" and returns stdout/stderr
// buffers and the Execute error.
func runStateNotify(t *testing.T) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	resetStateCmdFlags()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs([]string{"state", "notify"})
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

func TestStateNotify_CreatesSaveRequestedWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	if _, _, err := runStateNotify(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(dir, "save.requested")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("save.requested not created: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("save.requested size = %d, want 0", info.Size())
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("save.requested mode = %o, want 0600", perm)
	}
}

func TestStateNotify_BumpsMtimeWhenPresent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	if _, _, err := runStateNotify(t); err != nil {
		t.Fatalf("first notify: unexpected error: %v", err)
	}
	path := filepath.Join(dir, "save.requested")
	first, err := os.Stat(path)
	if err != nil {
		t.Fatalf("first stat: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	if _, _, err := runStateNotify(t); err != nil {
		t.Fatalf("second notify: unexpected error: %v", err)
	}
	second, err := os.Stat(path)
	if err != nil {
		t.Fatalf("second stat: %v", err)
	}

	if !second.ModTime().After(first.ModTime()) {
		t.Errorf("mtime did not advance: first=%v second=%v", first.ModTime(), second.ModTime())
	}
}

func TestStateNotify_CreatesStateDirWithMode0700WhenMissing(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "state-not-yet-created")
	t.Setenv("PORTAL_STATE_DIR", dir)

	if _, _, err := runStateNotify(t); err != nil {
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
		t.Errorf("state dir mode = %o, want 0700", perm)
	}
}

func TestStateNotify_TruncatesExistingContent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Pre-create save.requested with some content.
	path := filepath.Join(dir, "save.requested")
	if err := os.WriteFile(path, []byte("foo"), 0o600); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	if _, _, err := runStateNotify(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("save.requested size = %d, want 0 (truncated)", info.Size())
	}
}

func TestStateNotify_ExitsZeroOnSuccess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	_, _, err := runStateNotify(t)
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
}

func TestStateNotify_ExitsNonZeroWhenStateDirNotWritable(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "state")

	// Make parent read+execute only so MkdirAll(dir, ...) cannot create the
	// state subdirectory beneath it.
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	t.Setenv("PORTAL_STATE_DIR", dir)

	_, _, err := runStateNotify(t)
	if err == nil {
		t.Fatal("expected non-zero exit when state dir is not writable, got nil")
	}
}

func TestStateNotify_DoesNotReadOrCreateOtherStateFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	if _, _, err := runStateNotify(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// sessions.json must not exist after notify.
	if _, err := os.Stat(filepath.Join(dir, "sessions.json")); !os.IsNotExist(err) {
		t.Errorf("sessions.json must not exist after notify; stat err = %v", err)
	}

	// scrollback/ may exist (EnsureDir creates it) but must be empty.
	scrollback := filepath.Join(dir, "scrollback")
	if entries, err := os.ReadDir(scrollback); err == nil {
		if len(entries) != 0 {
			t.Errorf("scrollback/ must be empty after notify, got %d entries", len(entries))
		}
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected scrollback stat error: %v", err)
	}
}

// stateNotifyPanicBootstrapper implements ServerBootstrapper but panics on any
// call. Used to prove that PersistentPreRunE never invokes bootstrap for the
// state notify command (state is in skipTmuxCheck).
type stateNotifyPanicBootstrapper struct{}

func (stateNotifyPanicBootstrapper) EnsureServer() (bool, error) {
	panic("state notify must not invoke bootstrap (state is in skipTmuxCheck)")
}

func TestStateNotify_DoesNotInvokeBootstrap(t *testing.T) {
	bootstrapDeps = &BootstrapDeps{Bootstrapper: stateNotifyPanicBootstrapper{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PersistentPreRunE invoked bootstrap: %v", r)
		}
	}()

	if _, _, err := runStateNotify(t); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
