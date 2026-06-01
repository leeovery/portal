package restoretest

import (
	"io"
	"log/slog"
	"testing"
)

// OpenTestLogger returns a silent *slog.Logger (writing to io.Discard) for
// integration-test sites that need a non-nil logger to satisfy the retyped
// bootstrap / restore / FIFOSweeper adapters. These tests assert on tmux /
// state-dir side effects, not on log output, so a discard sink is sufficient.
//
// The signature keeps the *testing.T-first shape (and the unused stateDir
// parameter) so the 12+ promoted call sites continue to compile unchanged
// after the observability migration retyped every logging seam to
// *slog.Logger. Tests that DO want to capture log output use
// log.SetTestHandler instead.
func OpenTestLogger(t *testing.T, stateDir string) *slog.Logger {
	t.Helper()
	_ = stateDir
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
