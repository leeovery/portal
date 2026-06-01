package restoretest

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// portalLogName is the portal.log basename joined onto the caller-supplied
// stateDir. Mirrors internal/log's own constant — restoretest must not import
// internal/log just to read it, and keeping the literal local avoids any
// import-cycle risk.
const portalLogName = "portal.log"

// OpenTestLogger returns a *slog.Logger that writes a text-format audit trail
// to <stateDir>/portal.log. It mirrors the production Phase-1 sink (an
// append-mode portal.log under stateDir wrapped in slog.NewTextHandler) so
// integration tests that read portal.log file content (e.g. via
// portaltest.ReadPortalLogSafe) observe real on-disk records.
//
// The opened file is closed via t.Cleanup. The signature keeps the
// *testing.T-first shape and *slog.Logger return type so the promoted call
// sites continue to compile unchanged after the observability migration retyped
// every logging seam to *slog.Logger. Tests that want to capture log output
// in-process (without touching disk) use log.SetTestHandler instead.
func OpenTestLogger(t *testing.T, stateDir string) *slog.Logger {
	t.Helper()

	path := filepath.Join(stateDir, portalLogName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenTestLogger: open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })

	return slog.New(slog.NewTextHandler(f, nil))
}
