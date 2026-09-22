package cmd

import (
	"slices"
	"testing"
)

func TestResumeChainArgv(t *testing.T) {
	full := resumeChainPayload{
		Command: "claude --resume abc",
		Report:  "could not clear the pending marker",
		HookKey: "tok123",
		Pane:    "%7",
		PaneKey: "proj-a1b2:0.1",
		Width:   120,
		Height:  40,
	}

	t.Run("it composes the draw argv from the whole payload", func(t *testing.T) {
		got := resumeChainArgv("/usr/local/bin/portal", resumeDrawSubcommand, full)
		want := []string{
			"/usr/local/bin/portal", "state", "resume-draw",
			"--command", "claude --resume abc",
			"--report", "could not clear the pending marker",
			"--hook-key", "tok123",
			"--pane", "%7",
			"--pane-key", "proj-a1b2:0.1",
			"--width", "120",
			"--height", "40",
		}
		if !slices.Equal(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	})

	t.Run("it omits the report and size flags a payload does not carry", func(t *testing.T) {
		got := resumeChainArgv("/p", resumeWaitSubcommand, resumeChainPayload{
			Command: "c", HookKey: "k", Pane: "%1", PaneKey: "s:0.0", Width: 0, Height: -3,
		})
		want := []string{
			"/p", "state", "resume-wait",
			"--command", "c",
			"--hook-key", "k",
			"--pane", "%1",
			"--pane-key", "s:0.0",
		}
		if !slices.Equal(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	})

	t.Run("it carries the pane and the pane key alone for the chain's tail", func(t *testing.T) {
		for _, payload := range []resumeChainPayload{full, {Pane: "%7", PaneKey: "proj-a1b2:0.1"}} {
			got := resumeChainArgv("/p", resumeRecoverSubcommand, payload)
			want := []string{"/p", "state", "resume-recover", "--pane", "%7", "--pane-key", "proj-a1b2:0.1"}
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
