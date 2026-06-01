package state_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// TestWriteVersionFile_EmitsBreadcrumb pins the post-migration breadcrumb
// shape: a DEBUG record "daemon.version write" carrying the destination path
// attr. version and pid are now baseline attrs injected per-record by the
// configured handler (set once via main -> log.Init), so they are no longer
// emitted at this call site.
func TestWriteVersionFile_EmitsBreadcrumb(t *testing.T) {
	dir := t.TempDir()
	lg, sink := newCaptureLogger(t)

	if err := state.WriteVersionFile(dir, "1.2.3", lg); err != nil {
		t.Fatalf("WriteVersionFile: %v", err)
	}

	log := sink.body()
	if !strings.Contains(log, "daemon.version write") {
		t.Fatalf("log missing message 'daemon.version write':\n%s", log)
	}
	if !strings.Contains(log, "DEBUG") {
		t.Errorf("breadcrumb is not DEBUG level:\n%s", log)
	}
	wantPath := "path=" + filepath.Join(dir, "daemon.version")
	if !strings.Contains(log, wantPath) {
		t.Errorf("breadcrumb missing %q:\n%s", wantPath, log)
	}
}

// TestWriteVersionFile_EmitsBreadcrumbEvenWhenWriteFails pins that the
// breadcrumb is emitted BEFORE the atomic-write side effect, so a write
// failure still leaves the paper trail.
func TestWriteVersionFile_EmitsBreadcrumbEvenWhenWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based write-failure test relies on POSIX semantics")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; read-only directory does not block writes")
	}

	parent := t.TempDir()
	lg, sink := newCaptureLogger(t)

	roDir := filepath.Join(parent, "ro")
	if err := os.Mkdir(roDir, 0o500); err != nil {
		t.Fatalf("mkdir ro: %v", err)
	}
	// Restore writable mode after test so t.TempDir cleanup can remove it.
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o700) })

	err := state.WriteVersionFile(roDir, "9.9.9", lg)
	if err == nil {
		t.Fatalf("WriteVersionFile to read-only dir succeeded; want error")
	}

	log := sink.body()
	if !strings.Contains(log, "daemon.version write") {
		t.Fatalf("breadcrumb missing after failed write:\n%s", log)
	}
	wantPath := "path=" + filepath.Join(roDir, "daemon.version")
	if !strings.Contains(log, wantPath) {
		t.Errorf("breadcrumb missing path attr after failed write:\n%s", log)
	}
}

// TestWriteVersionFile_EmitsExactlyOneBreadcrumbPerCall pins one breadcrumb
// per call.
func TestWriteVersionFile_EmitsExactlyOneBreadcrumbPerCall(t *testing.T) {
	dir := t.TempDir()
	lg, sink := newCaptureLogger(t)

	if err := state.WriteVersionFile(dir, "v-once", lg); err != nil {
		t.Fatalf("WriteVersionFile: %v", err)
	}

	count := strings.Count(sink.body(), "daemon.version write")
	if count != 1 {
		t.Fatalf("expected exactly 1 breadcrumb, got %d. log:\n%s", count, sink.body())
	}
}
