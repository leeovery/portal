package state_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// openDebugLogger opens a real *state.Logger at logPath with DEBUG level so
// breadcrumb entries are retained, and registers cleanup to close it.
func openDebugLogger(t *testing.T, logPath string) *state.Logger {
	t.Helper()
	t.Setenv("PORTAL_LOG_LEVEL", "debug")
	lg, err := state.OpenLogger(logPath, false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = lg.Close() })
	return lg
}

func readLog(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return string(data)
}

func TestWriteVersionFile_EmitsBreadcrumb(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "portal.log")
	lg := openDebugLogger(t, logPath)

	if err := state.WriteVersionFile(dir, "1.2.3", lg); err != nil {
		t.Fatalf("WriteVersionFile: %v", err)
	}

	log := readLog(t, logPath)
	if !strings.Contains(log, "daemon.version write:") {
		t.Fatalf("log missing prefix 'daemon.version write:':\n%s", log)
	}
	if !strings.Contains(log, "| DEBUG |") {
		t.Errorf("breadcrumb is not DEBUG level:\n%s", log)
	}
	if !strings.Contains(log, "| "+state.ComponentDaemon+" |") {
		t.Errorf("breadcrumb component != %q:\n%s", state.ComponentDaemon, log)
	}
	if !strings.Contains(log, "version=1.2.3") {
		t.Errorf("breadcrumb missing version token:\n%s", log)
	}
	wantPID := fmt.Sprintf("pid=%d", os.Getpid())
	if !strings.Contains(log, wantPID) {
		t.Errorf("breadcrumb missing %q:\n%s", wantPID, log)
	}
	wantPath := "path=" + filepath.Join(dir, "daemon.version")
	if !strings.Contains(log, wantPath) {
		t.Errorf("breadcrumb missing %q:\n%s", wantPath, log)
	}
}

func TestWriteVersionFile_EmitsBreadcrumbEvenWhenWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based write-failure test relies on POSIX semantics")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; read-only directory does not block writes")
	}

	parent := t.TempDir()
	logPath := filepath.Join(parent, "portal.log")
	lg := openDebugLogger(t, logPath)

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

	log := readLog(t, logPath)
	if !strings.Contains(log, "daemon.version write:") {
		t.Fatalf("breadcrumb missing after failed write:\n%s", log)
	}
	if !strings.Contains(log, "version=9.9.9") {
		t.Errorf("breadcrumb missing version token after failed write:\n%s", log)
	}
}

func TestWriteVersionFile_EmitsExactlyOneBreadcrumbPerCall(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "portal.log")
	lg := openDebugLogger(t, logPath)

	if err := state.WriteVersionFile(dir, "v-once", lg); err != nil {
		t.Fatalf("WriteVersionFile: %v", err)
	}

	log := readLog(t, logPath)
	count := strings.Count(log, "daemon.version write:")
	if count != 1 {
		t.Fatalf("expected exactly 1 breadcrumb, got %d. log:\n%s", count, log)
	}
}
