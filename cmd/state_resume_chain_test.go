package cmd

import (
	"slices"
	"testing"
)

func TestResumeChainArgv(t *testing.T) {
	full := resumeChainPayload{
		Command: "claude --resume abc",
		Report:  "could not clear the pending marker",
		Session: "proj-a1b2",
		Width:   120,
		Height:  40,
	}

	t.Run("it composes the draw argv from the whole payload", func(t *testing.T) {
		got := resumeChainArgv("/usr/local/bin/portal", resumeDrawSubcommand, full)
		want := []string{
			"/usr/local/bin/portal", "state", "resume-draw",
			"--command", "claude --resume abc",
			"--report", "could not clear the pending marker",
			"--session", "proj-a1b2",
			"--width", "120",
			"--height", "40",
		}
		if !slices.Equal(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	})

	t.Run("it omits the report and size flags a payload does not carry", func(t *testing.T) {
		got := resumeChainArgv("/p", resumeWaitSubcommand, resumeChainPayload{
			Command: "c", Session: "s", Width: 0, Height: -3,
		})
		want := []string{
			"/p", "state", "resume-wait",
			"--command", "c",
			"--session", "s",
		}
		if !slices.Equal(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	})

	t.Run("it carries the session alone to the chain's tail", func(t *testing.T) {
		for _, payload := range []resumeChainPayload{full, {Session: "proj-a1b2"}} {
			got := resumeChainArgv("/p", resumeRecoverSubcommand, payload)
			want := []string{"/p", "state", "resume-recover", "--session", "proj-a1b2"}
			if !slices.Equal(got, want) {
				t.Errorf("argv = %q, want %q", got, want)
			}
		}
	})
}

func TestResumeChainExe(t *testing.T) {
	t.Run("it resolves the running binary", func(t *testing.T) {
		exe, err := resumeChainExe()
		if err != nil {
			t.Fatalf("resumeChainExe() error = %v, want the test binary's path", err)
		}
		if exe == "" {
			t.Error("resumeChainExe() returned an empty path with no error")
		}
	})
}

func TestResumeChainSteps_RequireTheSession(t *testing.T) {
	for _, step := range []string{resumeDrawSubcommand, resumeWaitSubcommand} {
		t.Run("state "+step+" refuses an argv naming no session", func(t *testing.T) {
			_, _, err := runRootCmd(t, "state", step, "--command", "make deploy")

			if err == nil || err.Error() != `required flag(s) "session" not set` {
				t.Errorf("state %s with no --session = %v, want the required-flag refusal", step, err)
			}
		})
	}
}
