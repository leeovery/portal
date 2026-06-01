package restoretest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/restoretest"
)

// TestOpenTestLogger_WritesToPortalLog asserts the helper returns a non-nil
// *slog.Logger whose output lands in <stateDir>/portal.log. Integration tests
// read portal.log file content (e.g. via portaltest.ReadPortalLogSafe) to
// assert on the daemon/bootstrap audit trail, so the helper must produce real
// on-disk content rather than discarding writes.
func TestOpenTestLogger_WritesToPortalLog(t *testing.T) {
	stateDir := t.TempDir()

	logger := restoretest.OpenTestLogger(t, stateDir)
	if logger == nil {
		t.Fatal("OpenTestLogger returned nil; want non-nil *slog.Logger")
	}

	logger.Info("smoke-marker", "key", "value")

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
