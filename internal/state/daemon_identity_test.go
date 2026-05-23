package state

import (
	"errors"
	"os"
	"testing"
)

// withIdentifyPSFake swaps the identifyPS seam for the duration of the test
// and restores it via t.Cleanup. Tests must not use t.Parallel — identifyPS is
// package-level mutable state shared across the test binary.
func withIdentifyPSFake(t *testing.T, fake func(pid int) (string, error)) {
	t.Helper()
	prev := identifyPS
	identifyPS = fake
	t.Cleanup(func() { identifyPS = prev })
}

func TestIdentifyDaemon_IsPortalDaemonWhenPSReportsPortalBinaryWithStateDaemonArgv(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal portal state daemon\n", nil
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyIsPortalDaemon {
		t.Errorf("got = %v; want IdentifyIsPortalDaemon", got)
	}
}

func TestIdentifyDaemon_IsPortalDaemonWhenArgvHasTrailingFlags(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal portal state daemon --flag\n", nil
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyIsPortalDaemon {
		t.Errorf("got = %v; want IdentifyIsPortalDaemon", got)
	}
}

func TestIdentifyDaemon_NotPortalDaemonWhenCommIsNotPortal(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "sleep sleep 30\n", nil
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyNotPortalDaemon {
		t.Errorf("got = %v; want IdentifyNotPortalDaemon", got)
	}
}

func TestIdentifyDaemon_NotPortalDaemonWhenArgvSuffixBreaksAnchoredRegex(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal portal state daemon-foo\n", nil
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyNotPortalDaemon {
		t.Errorf("got = %v; want IdentifyNotPortalDaemon", got)
	}
}

func TestIdentifyDaemon_NotPortalDaemonWhenArgvIsPortalSomethingElse(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal portal open foo\n", nil
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyNotPortalDaemon {
		t.Errorf("got = %v; want IdentifyNotPortalDaemon", got)
	}
}

func TestIdentifyDaemon_NotPortalDaemonAgainstOwnTestProcessPID(t *testing.T) {
	// Use the real ps seam (no stub). The state.test binary is comm=state.test
	// (or similar) — definitively not "portal".
	got, err := IdentifyDaemon(os.Getpid())
	if err != nil {
		t.Fatalf("IdentifyDaemon(os.Getpid()): unexpected err: %v", err)
	}
	if got != IdentifyNotPortalDaemon {
		t.Errorf("got = %v; want IdentifyNotPortalDaemon", got)
	}
}

func TestIdentifyDaemon_DeadForNonExistentPID(t *testing.T) {
	// Use the real ps seam. PID 0x7FFFFFFE is virtually guaranteed not to
	// exist on a real system; ps will exit non-zero with empty stdout.
	got, err := IdentifyDaemon(0x7FFFFFFE)
	if err != nil {
		t.Fatalf("IdentifyDaemon(nonexistent): unexpected err: %v", err)
	}
	if got != IdentifyDead {
		t.Errorf("got = %v; want IdentifyDead", got)
	}
}

func TestIdentifyDaemon_DeadWhenPSExitsNonZeroWithEmptyStdout(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "", &fakePSExitError{}
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyDead {
		t.Errorf("got = %v; want IdentifyDead", got)
	}
}

func TestIdentifyDaemon_TransientErrorWhenPSExitsNonZeroWithNonEmptyStdout(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal portal state daemon\n", &fakePSExitError{}
	})

	got, err := IdentifyDaemon(1234)
	if err == nil {
		t.Fatalf("IdentifyDaemon: expected transient error, got nil (result %v)", got)
	}
	if got != 0 {
		t.Errorf("got = %v; want 0 on transient error", got)
	}
}

func TestIdentifyDaemon_TransientErrorWhenPSOutputIsSingleToken(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal\n", nil
	})

	got, err := IdentifyDaemon(1234)
	if err == nil {
		t.Fatalf("IdentifyDaemon: expected transient error for malformed output, got nil (result %v)", got)
	}
	if got != 0 {
		t.Errorf("got = %v; want 0 on transient error", got)
	}
}

func TestIdentifyDaemon_DeadForZeroPIDWithoutInvokingPS(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		t.Fatalf("identifyPS must not be called for pid <= 0; got pid=%d", pid)
		return "", nil
	})

	got, err := IdentifyDaemon(0)
	if err != nil {
		t.Fatalf("IdentifyDaemon(0): unexpected err: %v", err)
	}
	if got != IdentifyDead {
		t.Errorf("got = %v; want IdentifyDead", got)
	}
}

func TestIdentifyDaemon_DeadForNegativePIDWithoutInvokingPS(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		t.Fatalf("identifyPS must not be called for pid <= 0; got pid=%d", pid)
		return "", nil
	})

	got, err := IdentifyDaemon(-1)
	if err != nil {
		t.Fatalf("IdentifyDaemon(-1): unexpected err: %v", err)
	}
	if got != IdentifyDead {
		t.Errorf("got = %v; want IdentifyDead", got)
	}
}

func TestIdentifyDaemon_HandlesWhitespacePaddedPSOutput(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "  portal portal state daemon  \n\n", nil
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyIsPortalDaemon {
		t.Errorf("got = %v; want IdentifyIsPortalDaemon", got)
	}
}

func TestIdentifyDaemon_DoesNotMatchPortalStateDaemonWithoutSpaces(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal-state-daemon portal-state-daemon\n", nil
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyNotPortalDaemon {
		t.Errorf("got = %v; want IdentifyNotPortalDaemon", got)
	}
}

// fakePSExitError mimics the surface used in IdentifyDaemon's branching:
// "ps exited non-zero" without depending on os/exec.ExitError construction.
type fakePSExitError struct{}

func (fakePSExitError) Error() string { return "fake ps exit error" }

// Guard against the sentinel being interpreted as something specific via
// errors.Is.
var _ error = fakePSExitError{}

func TestIdentifyDaemon_TransientErrorPreservesUnderlyingErrorViaWrap(t *testing.T) {
	sentinel := errors.New("ps boom")
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal portal state daemon\n", sentinel
	})

	_, err := IdentifyDaemon(1234)
	if err == nil {
		t.Fatalf("IdentifyDaemon: expected transient error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v; expected wrapped sentinel via errors.Is", err)
	}
}
