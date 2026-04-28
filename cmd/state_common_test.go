// Tests in this file mutate package-level state via t.Setenv and MUST NOT use t.Parallel.
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// TestOpenNoRotateLogger_OpensAppendOnlyAndDoesNotRotate verifies the helper
// returns a usable *state.Logger that appends without ever renaming the
// existing file, even when its size exceeds the 1 MiB rotation threshold.
// This protects the spec invariant that only the daemon rotates portal.log.
func TestOpenNoRotateLogger_OpensAppendOnlyAndDoesNotRotate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Pre-seed an over-threshold portal.log so that, if the helper used
	// rotate=true, we would observe a rename to portal.log.old.
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	logPath := state.PortalLog(dir)
	oversized := make([]byte, 2<<20) // 2 MiB > 1 MiB rotation threshold
	if err := os.WriteFile(logPath, oversized, 0o600); err != nil {
		t.Fatalf("seed oversized log: %v", err)
	}

	logger, err := openNoRotateLogger()
	if err != nil {
		t.Fatalf("openNoRotateLogger: %v", err)
	}
	if logger == nil {
		t.Fatal("openNoRotateLogger returned nil logger; want non-nil")
	}
	t.Cleanup(func() { _ = logger.Close() })

	// Append a known entry — the helper opens with O_APPEND so the existing
	// 2 MiB block must remain ahead of our new line.
	t.Setenv("PORTAL_LOG_LEVEL", "warn")
	logger.Warn(state.ComponentDaemon, "no-rotate-marker")
	_ = logger.Close()

	// portal.log.old must NOT exist — non-daemon writers never rotate.
	if _, err := os.Stat(filepath.Join(dir, "portal.log.old")); !os.IsNotExist(err) {
		t.Errorf("portal.log.old created by non-daemon writer; stat=%v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(data) <= len(oversized) {
		t.Errorf("log truncated/rotated: size=%d <= seed=%d", len(data), len(oversized))
	}
	if !strings.Contains(string(data[len(oversized):]), "no-rotate-marker") {
		t.Errorf("appended entry missing from log tail")
	}
}

// TestOpenNoRotateLogger_ReturnsNilLoggerOnError exercises the failure path:
// when EnsureDir cannot create the state dir (e.g. PORTAL_STATE_DIR points at
// a path whose parent is a regular file), the helper returns (nil, err) and
// callers fall back to a no-op logger.
func TestOpenNoRotateLogger_ReturnsNilLoggerOnError(t *testing.T) {
	dir := t.TempDir()
	// Make PORTAL_STATE_DIR's parent a regular file so EnsureDir fails.
	blocking := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocking, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}
	t.Setenv("PORTAL_STATE_DIR", filepath.Join(blocking, "child"))

	logger, err := openNoRotateLogger()
	if err == nil {
		t.Errorf("openNoRotateLogger expected error when state dir cannot be created; got nil")
	}
	if logger != nil {
		_ = logger.Close()
		t.Errorf("openNoRotateLogger returned non-nil logger on error path")
	}
}
