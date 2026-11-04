package state

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"
)

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
	// Real ps seam, no stub: the test binary's comm is never "portal".
	got, err := IdentifyDaemon(os.Getpid())
	if err != nil {
		t.Fatalf("IdentifyDaemon(os.Getpid()): unexpected err: %v", err)
	}
	if got != IdentifyNotPortalDaemon {
		t.Errorf("got = %v; want IdentifyNotPortalDaemon", got)
	}
}

func TestIdentifyDaemon_DeadForNonExistentPID(t *testing.T) {
	// Real ps seam: this pid is effectively guaranteed not to exist.
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
		return "", psExited()
	})

	got, err := IdentifyDaemon(1234)
	if err != nil {
		t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
	}
	if got != IdentifyDead {
		t.Errorf("got = %v; want IdentifyDead", got)
	}
}

// psExited is a ps that ran and exited non-zero, as the exec helper wraps it.
func psExited() error { return fmt.Errorf("/bin/ps [-p 1234]: %w", &exec.ExitError{}) }

func TestIdentifyDaemon_ErrorWhenPSCannotRun(t *testing.T) {
	t.Run("a ps that never ran says nothing of the pid", func(t *testing.T) {
		withIdentifyPSFake(t, func(pid int) (string, error) {
			return "", fmt.Errorf("ps [-p 1234]: %w", exec.ErrNotFound)
		})

		if got, err := IdentifyDaemon(1234); !errors.Is(err, exec.ErrNotFound) {
			t.Errorf("IdentifyDaemon = %v, %v; want the failure to run ps, not the pid read as dead", got, err)
		}
	})

	t.Run("with no ps on PATH", func(t *testing.T) {
		t.Setenv("PATH", "")

		if got, err := IdentifyDaemon(0x7FFFFFFE); err == nil {
			t.Errorf("IdentifyDaemon = %v, nil; want an error, not a result", got)
		}
	})
}

func TestIdentifyDaemon_TransientErrorWhenPSExitsNonZeroWithNonEmptyStdout(t *testing.T) {
	withIdentifyPSFake(t, func(pid int) (string, error) {
		return "portal portal state daemon\n", psExited()
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

// daemonProgramCases are argv[0]s a daemon runs as, and near misses, for the
// identity check and the pgrep pattern alike: portal bare or at an absolute
// path, and nothing else.
var daemonProgramCases = []struct {
	program string
	daemon  bool
}{
	{"portal", true},
	{"/opt/homebrew/bin/portal", true},
	{"/x/y/portal", true},
	{"/portal", true},
	{"/x//portal", true},
	{"/x/y/notportal", false},
	{"/x/y/portal2", false},
	{"/x/y/portal.old", false},
	{"/x/portal/bin", false},
	{"x/y/portal", false},
	{"./portal", false},
	{"notportal", false},
	{"/xportal", false},
}

func TestIdentifyDaemon_ProgramIsPortalBareOrAtAnAbsolutePath(t *testing.T) {
	for _, tc := range daemonProgramCases {
		t.Run(tc.program, func(t *testing.T) {
			// ucomm is the exec'd file's name, the last element of argv[0].
			withIdentifyPSFake(t, func(int) (string, error) {
				return path.Base(tc.program) + " " + tc.program + " state daemon\n", nil
			})

			got, err := IdentifyDaemon(1234)
			if err != nil {
				t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
			}
			if want := map[bool]IdentifyResult{true: IdentifyIsPortalDaemon, false: IdentifyNotPortalDaemon}[tc.daemon]; got != want {
				t.Errorf("IdentifyDaemon(%q state daemon) = %v; want %v", tc.program, got, want)
			}
		})
	}
}

func TestIdentifyDaemon_NeedsBothCommAndArgvToNamePortal(t *testing.T) {
	for _, line := range []string{
		"notportal /x/y/portal state daemon",
		"/x/y/portal /x/y/portal state daemon",
		"portal /x/y/notportal state daemon",
		"sh /bin/sh -c /x/y/portal state daemon",
		"portal /x/y/portal state daemon-foo",
		"portal /x/y/portal state hydrate",
	} {
		t.Run(line, func(t *testing.T) {
			withIdentifyPSFake(t, func(int) (string, error) { return line + "\n", nil })

			got, err := IdentifyDaemon(1234)
			if err != nil {
				t.Fatalf("IdentifyDaemon: unexpected err: %v", err)
			}
			if got != IdentifyNotPortalDaemon {
				t.Errorf("IdentifyDaemon(%q) = %v; want IdentifyNotPortalDaemon", line, got)
			}
		})
	}
}

func TestPortalDaemonArgvPattern(t *testing.T) {
	for _, tc := range daemonProgramCases {
		for _, tail := range []string{"", " --flag"} {
			argv := tc.program + " state daemon" + tail
			t.Run(argv, func(t *testing.T) {
				if got := daemonArgvPattern.MatchString(argv); got != tc.daemon {
					t.Errorf("PortalDaemonArgvPattern matches %q = %v; want %v", argv, got, tc.daemon)
				}
			})
		}
	}
	for _, argv := range []string{
		"/bin/sh -c /x/y/portal state daemon",
		"/x/y z/portal state daemon",
		"/x/y/portal state daemon-foo",
		"/x/y/portal state daemons",
		"/x/y/portal open",
		" /x/y/portal state daemon",
	} {
		t.Run(argv, func(t *testing.T) {
			if daemonArgvPattern.MatchString(argv) {
				t.Errorf("PortalDaemonArgvPattern matches %q; want no match", argv)
			}
		})
	}
}

func TestIsDaemonArgv(t *testing.T) {
	for _, tc := range daemonProgramCases {
		argv := []string{tc.program, "state", "daemon"}
		if got := IsDaemonArgv(argv); got != tc.daemon {
			t.Errorf("IsDaemonArgv(%q) = %v; want %v", argv, got, tc.daemon)
		}
	}
	for _, argv := range [][]string{
		{"/x/y/portal", "state", "daemon", "--flag"},
	} {
		if !IsDaemonArgv(argv) {
			t.Errorf("IsDaemonArgv(%q) = false; want true", argv)
		}
	}
	for _, argv := range [][]string{
		nil,
		{"/bin/sh", "-c", "/x/y/portal state daemon"},
		{"/x/y/portal", "state"},
		{"/x/y/portal", "state", "daemon-foo"},
		{"/x/y/portal", "state", "hydrate"},
	} {
		if IsDaemonArgv(argv) {
			t.Errorf("IsDaemonArgv(%q) = true; want false", argv)
		}
	}
}

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
