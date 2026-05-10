package restoretest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/restoretest"
)

// TestOpenTestLogger_CreatesPortalLogUnderStateDir asserts the helper
// opens (and therefore creates) <stateDir>/portal.log so subsequent
// writes through the returned *state.Logger land at that path. This
// pins the path convention shared by the 12 promoted call sites.
func TestOpenTestLogger_CreatesPortalLogUnderStateDir(t *testing.T) {
	stateDir := t.TempDir()

	logger := restoretest.OpenTestLogger(t, stateDir)
	if logger == nil {
		t.Fatal("OpenTestLogger returned nil; want non-nil *state.Logger")
	}

	logPath := filepath.Join(stateDir, "portal.log")
	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("portal.log not created at %s: %v", logPath, err)
	}
}
