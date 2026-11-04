package state

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func withPgrepCommandFake(t *testing.T, fake func() *exec.Cmd) {
	t.Helper()
	prev := pgrepCommand
	pgrepCommand = fake
	t.Cleanup(func() { pgrepCommand = prev })
}

func TestPgrepPortalDaemons_NoMatchesReturnsNilNil(t *testing.T) {
	// `false` reproduces pgrep's no-match shape: status 1, no output.
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

	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Errorf("errors.AsType[*exec.ExitError](err) = false; *exec.ExitError not recovered through the wrap: %v", err)
	}
}

// The real pgrep runs with a pattern no process can match: with the daemon's
// own it would enumerate the developer's live daemon, which no unit test may
// reach.
func TestPgrepPortalDaemons_RealNoMatchExitsCleanly(t *testing.T) {
	withPgrepCommandFake(t, func() *exec.Cmd {
		return exec.Command("pgrep", "-fx", `^portal-test-no-such-process-[0-9]{40} state daemon$`)
	})

	pids, err := PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("expected no error from real pgrep no-match, got: %v", err)
	}
	if pids != nil {
		t.Errorf("expected nil pids from a pattern nothing matches, got %v", pids)
	}
}
