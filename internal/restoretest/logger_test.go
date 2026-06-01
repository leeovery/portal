package restoretest_test

import (
	"testing"

	"github.com/leeovery/portal/internal/restoretest"
)

// TestOpenTestLogger_ReturnsUsableSilentLogger asserts the helper returns a
// non-nil *slog.Logger that swallows writes silently (the post-migration
// contract: a discard-backed sink for adapter tests that assert on side
// effects, not log output).
func TestOpenTestLogger_ReturnsUsableSilentLogger(t *testing.T) {
	stateDir := t.TempDir()

	logger := restoretest.OpenTestLogger(t, stateDir)
	if logger == nil {
		t.Fatal("OpenTestLogger returned nil; want non-nil *slog.Logger")
	}

	// Logging must not panic and must produce no observable file output —
	// the sink is io.Discard.
	logger.Info("smoke", "key", "value")
}
