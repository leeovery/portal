package state

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// withPgrepCommandFake swaps the pgrepCommand seam for the duration of the test
// and restores it via t.Cleanup. Tests must not use t.Parallel — pgrepCommand
// is package-level mutable state shared across the test binary.
func withPgrepCommandFake(t *testing.T, fake func() *exec.Cmd) {
	t.Helper()
	prev := pgrepCommand
	pgrepCommand = fake
	t.Cleanup(func() { pgrepCommand = prev })
}

// TestPgrepPortalDaemons_NoMatchesReturnsNilNil verifies the load-bearing
// status-1 + empty-stdout branch still yields (nil, nil) — pgrep's documented
// "nothing found" signal — when wired through the boundary helper.
func TestPgrepPortalDaemons_NoMatchesReturnsNilNil(t *testing.T) {
	// `false` exits with status 1 and writes nothing to stdout/stderr — the
	// exact shape pgrep produces on no matches.
	withPgrepCommandFake(t, func() *exec.Cmd {
		return exec.Command("false")
	})

	pids, err := PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("expected (nil, nil) on status-1 no-matches, got err: %v", err)
	}
	if pids != nil {
		t.Errorf("expected nil pids, got %v", pids)
	}
}

// TestPgrepPortalDaemons_OSLayerFailureWrapsWithStderr verifies a non-status-1
// failure (here status 2 with stderr) returns the stderr-enriched wrapped
// error and remains errors.As-recoverable to *exec.ExitError.
func TestPgrepPortalDaemons_OSLayerFailureWrapsWithStderr(t *testing.T) {
	withPgrepCommandFake(t, func() *exec.Cmd {
		return exec.Command("sh", "-c", "echo 'pgrep boom' >&2; exit 2")
	})

	pids, err := PgrepPortalDaemons()
	if err == nil {
		t.Fatalf("expected error on status-2 failure, got nil (pids=%v)", pids)
	}
	if pids != nil {
		t.Errorf("expected nil pids on failure, got %v", pids)
	}
	if !strings.Contains(err.Error(), "pgrep boom") {
		t.Errorf("error %q does not contain trimmed stderr", err.Error())
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Errorf("errors.As did not recover *exec.ExitError through the wrap: %v", err)
	}
}

// TestPgrepPortalDaemons_RealNoMatchExitsCleanly verifies the production path
// (real pgrep against the canonical pattern) returns (nil, nil) on a host with
// no live portal daemons — the steady-state clean-install shape.
func TestPgrepPortalDaemons_RealNoMatchExitsCleanly(t *testing.T) {
	pids, err := PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("expected no error from real pgrep no-match, got: %v", err)
	}
	_ = pids // may legitimately be non-nil if a daemon is running; only err matters here
}
