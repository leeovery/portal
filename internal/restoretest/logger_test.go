package restoretest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/restoretest"
)

// TestOpenTestLogger_WritesToPortalLog asserts the helper returns a non-nil
// *slog.Logger whose output lands in the day file the portal.log symlink points
// at. Integration tests read portal.log content (e.g. via
// portaltest.ReadPortalLogSafe) to assert on the daemon/bootstrap audit trail,
// so the helper must produce real on-disk content rather than discarding writes.
func TestOpenTestLogger_WritesToPortalLog(t *testing.T) {
	stateDir := t.TempDir()

	logger := restoretest.OpenTestLogger(t, stateDir)
	if logger == nil {
		t.Fatal("OpenTestLogger returned nil; want non-nil *slog.Logger")
	}

	logger.Info("smoke-marker", "key", "value")

	// Reading through the portal.log symlink follows it to the day file.
	path := filepath.Join(stateDir, "portal.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "smoke-marker") {
		t.Errorf("portal.log missing logged message; got:\n%s", got)
	}
	if !strings.Contains(got, "key=value") {
		t.Errorf("portal.log missing logged attr; got:\n%s", got)
	}
	if !strings.Contains(got, "INFO") {
		t.Errorf("portal.log missing slog text level label; got:\n%s", got)
	}
}

// TestOpenTestLogger_ProducesProductionSinkShape pins the on-disk contract:
// OpenTestLogger must mirror the production rotatingSink's file layout — a dated
// portal.log.<date> regular file plus a portal.log SYMLINK pointing at it — NOT
// a bare regular-file portal.log. A regular-file portal.log is what the
// production sink's migrationGuard os.Remove()s on its first write, so a test
// that opens this logger against a stateDir and also spawns the real binary
// would otherwise have two writers contending over a path one of them deletes.
func TestOpenTestLogger_ProducesProductionSinkShape(t *testing.T) {
	stateDir := t.TempDir()

	_ = restoretest.OpenTestLogger(t, stateDir)

	// portal.log must be a SYMLINK, not a regular file (Lstat does not follow).
	link := filepath.Join(stateDir, "portal.log")
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat portal.log: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("portal.log is not a symlink (mode %v); the production migration guard would unlink a regular file", info.Mode())
	}

	// The symlink target is the bare relative basename portal.log.<date>,
	// matching the production sink's swingSymlink contract.
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("readlink portal.log: %v", err)
	}
	wantTarget := "portal.log." + time.Now().Format("2006-01-02")
	if target != wantTarget {
		t.Fatalf("portal.log symlink target = %q; want %q (bare relative dated basename)", target, wantTarget)
	}

	// The dated day file must exist as a regular file in the stateDir.
	dayPath := filepath.Join(stateDir, wantTarget)
	dayInfo, err := os.Stat(dayPath)
	if err != nil {
		t.Fatalf("stat %s: %v", wantTarget, err)
	}
	if !dayInfo.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file (mode %v)", wantTarget, dayInfo.Mode())
	}
}
