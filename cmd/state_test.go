// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// availableCommandNames parses Cobra help output and returns the set of names
// listed under the "Available Commands:" section. Each listed line begins with
// two-space indentation followed by the command name.
func availableCommandNames(help string) map[string]bool {
	names := make(map[string]bool)
	inSection := false
	for _, line := range strings.Split(help, "\n") {
		switch {
		case strings.HasPrefix(line, "Available Commands:"):
			inSection = true
			continue
		case inSection && strings.TrimSpace(line) == "":
			inSection = false
		case inSection && strings.HasPrefix(line, "  "):
			fields := strings.Fields(line)
			if len(fields) > 0 {
				names[fields[0]] = true
			}
		}
	}
	return names
}

// resetStateCmdFlags resets flag values and Changed state on the state subcommands
// so successive test invocations are independent.
func resetStateCmdFlags() {
	if f := stateCleanupCmd.Flags().Lookup("purge"); f != nil {
		_ = f.Value.Set("false")
		f.Changed = false
	}
	for _, name := range []string{"fifo", "file", "hook-key"} {
		if f := stateHydrateCmd.Flags().Lookup(name); f != nil {
			_ = f.Value.Set("")
			f.Changed = false
		}
	}
}

func TestStateCommandRegistration(t *testing.T) {
	t.Run("state is registered as a top-level subcommand of root", func(t *testing.T) {
		var found bool
		for _, c := range rootCmd.Commands() {
			if c.Name() == "state" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("state subcommand is not registered on root command")
		}
	})

	t.Run("state appears in portal --help with its short description", func(t *testing.T) {
		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		listed := availableCommandNames(out)
		if !listed["state"] {
			t.Errorf("portal --help missing 'state' in Available Commands; got %v\noutput:\n%s", listed, out)
		}
		if !strings.Contains(out, "Manage Portal session resurrection state") {
			t.Errorf("portal --help missing state Short description:\n%s", out)
		}
	})

	t.Run("portal state --help lists only status and cleanup as available commands", func(t *testing.T) {
		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"state", "--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		listed := availableCommandNames(buf.String())

		// status and cleanup are required to appear
		required := []string{"status", "cleanup"}
		for _, name := range required {
			if !listed[name] {
				t.Errorf("portal state --help missing %q in Available Commands; got %v", name, listed)
			}
		}
		// hidden subcommands must never appear
		hidden := []string{"daemon", "notify", "signal-hydrate", "hydrate", "migrate-rename"}
		for _, h := range hidden {
			if listed[h] {
				t.Errorf("portal state --help must not list hidden subcommand %q; got %v", h, listed)
			}
		}
		// any other listed name beyond the user-facing two and Cobra's built-in
		// `help` is unexpected
		for name := range listed {
			if name == "status" || name == "cleanup" || name == "help" {
				continue
			}
			t.Errorf("portal state --help listed unexpected command %q; got %v", name, listed)
		}
	})

	t.Run("hidden subcommands do not appear in portal --help", func(t *testing.T) {
		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		listed := availableCommandNames(buf.String())

		// state itself is visible at root
		if !listed["state"] {
			t.Errorf("portal --help missing 'state' in Available Commands; got %v", listed)
		}
		// hidden subcommands of state should not surface at root either
		hidden := []string{"signal-hydrate", "migrate-rename", "hydrate", "notify"}
		for _, h := range hidden {
			if listed[h] {
				t.Errorf("portal --help must not list hidden subcommand %q; got %v", h, listed)
			}
		}
	})
}

func TestStateBareInvocationPrintsHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"state"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("portal state should exit 0, got error: %v", err)
	}
	out := buf.String()
	// Cobra default help output for a parent command includes "Available Commands:"
	if !strings.Contains(out, "Available Commands") && !strings.Contains(out, "status") {
		t.Errorf("portal state did not print help output:\n%s", out)
	}
}

func TestStateInternalSubcommandsAcceptValidArgv(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "daemon with no args", args: []string{"state", "daemon"}},
		{name: "notify with no args", args: []string{"state", "notify"}},
		{name: "signal-hydrate with session name", args: []string{"state", "signal-hydrate", "foo"}},
		{name: "hydrate with all required flags", args: []string{"state", "hydrate", "--fifo", "/tmp/f", "--file", "/tmp/s", "--hook-key", "k:0.0"}},
		{name: "migrate-rename with old and new", args: []string{"state", "migrate-rename", "old", "new"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Isolate every subtest against a fresh per-subtest temp state
			// dir so notify / signal-hydrate / hydrate / migrate-rename never
			// mutate (or fail to create) the developer's real
			// ~/.config/portal/state. The daemon case additionally needs the
			// run-func stub and lock-file reset.
			t.Setenv("PORTAL_STATE_DIR", t.TempDir())
			if len(tt.args) >= 2 && tt.args[0] == "state" && tt.args[1] == "daemon" {
				prev := daemonRunFunc
				daemonRunFunc = func(_ context.Context, _ *daemonDeps) error { return nil }
				t.Cleanup(func() { daemonRunFunc = prev })
				withDaemonLockFileReset(t)
			}

			// state hydrate's RunE blocks on a real FIFO; stub the run-func so
			// the command returns immediately for argv-only assertions.
			if len(tt.args) >= 2 && tt.args[0] == "state" && tt.args[1] == "hydrate" {
				prev := hydrateRunFunc
				hydrateRunFunc = func(_ hydrateConfig) error { return nil }
				t.Cleanup(func() { hydrateRunFunc = prev })
			}

			outBuf := new(bytes.Buffer)
			errBuf := new(bytes.Buffer)
			resetRootCmd()
			resetStateCmdFlags()
			rootCmd.SetOut(outBuf)
			rootCmd.SetErr(errBuf)
			rootCmd.SetArgs(tt.args)
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("expected exit 0, got error: %v\nstderr: %s", err, errBuf.String())
			}
			if errBuf.Len() != 0 {
				t.Errorf("expected no stderr noise, got: %s", errBuf.String())
			}
		})
	}
}

func TestStateUserFacingSubcommandsExitZero(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "status with no args", args: []string{"state", "status"}},
		{name: "cleanup with no flags", args: []string{"state", "cleanup"}},
		{name: "cleanup with --purge", args: []string{"state", "cleanup", "--purge"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outBuf := new(bytes.Buffer)
			errBuf := new(bytes.Buffer)
			resetRootCmd()
			resetStateCmdFlags()
			// Isolate every subtest against a fresh per-subtest temp state
			// dir so the cleanup / cleanup --purge cases never mutate the
			// developer's real ~/.config/portal/state. The assertion this
			// test cares about is that argv parsing succeeded and Cobra
			// handed control to RunE without a usage/parse error. The
			// status case is additionally allowed to exit non-zero with
			// ErrStatusUnhealthy, since an empty TempDir is an unhealthy
			// state surface (no daemon, stale save, recent warnings).
			t.Setenv("PORTAL_STATE_DIR", t.TempDir())
			rootCmd.SetOut(outBuf)
			rootCmd.SetErr(errBuf)
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err != nil && err != ErrStatusUnhealthy {
				t.Fatalf("expected exit 0 or ErrStatusUnhealthy, got error: %v\nstderr: %s", err, errBuf.String())
			}
		})
	}
}

func TestStateSignalHydrateRequiresSessionName(t *testing.T) {
	resetRootCmd()
	resetStateCmdFlags()
	rootCmd.SetOut(new(bytes.Buffer))
	rootCmd.SetErr(new(bytes.Buffer))
	rootCmd.SetArgs([]string{"state", "signal-hydrate"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected validation error for missing session name, got nil")
	}
}

func TestStateHydrateRequiresAllFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing --fifo",
			args: []string{"state", "hydrate", "--file", "/tmp/s", "--hook-key", "k:0.0"},
		},
		{
			name: "missing --file",
			args: []string{"state", "hydrate", "--fifo", "/tmp/f", "--hook-key", "k:0.0"},
		},
		{
			name: "missing --hook-key",
			args: []string{"state", "hydrate", "--fifo", "/tmp/f", "--file", "/tmp/s"},
		},
		{
			name: "missing all flags",
			args: []string{"state", "hydrate"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRootCmd()
			resetStateCmdFlags()
			rootCmd.SetOut(new(bytes.Buffer))
			rootCmd.SetErr(new(bytes.Buffer))
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.name)
			}
		})
	}
}

func TestStateCleanupAcceptsPurgeFlag(t *testing.T) {
	// Look up the flag definition directly to assert it's a bool flag
	flag := stateCleanupCmd.Flags().Lookup("purge")
	if flag == nil {
		t.Fatal("--purge flag not declared on state cleanup")
	}
	if flag.Value.Type() != "bool" {
		t.Errorf("--purge type = %q, want %q", flag.Value.Type(), "bool")
	}
}

func TestStateHiddenSubcommandsAreHidden(t *testing.T) {
	hidden := []string{"daemon", "notify", "signal-hydrate", "hydrate", "migrate-rename"}
	for _, name := range hidden {
		t.Run(name+" has Hidden=true", func(t *testing.T) {
			var match bool
			for _, c := range stateCmd.Commands() {
				if c.Name() == name {
					match = true
					if !c.Hidden {
						t.Errorf("subcommand %q must have Hidden=true", name)
					}
					break
				}
			}
			if !match {
				t.Errorf("subcommand %q not registered under state", name)
			}
		})
	}
}

func TestStateUserFacingSubcommandsAreVisible(t *testing.T) {
	visible := []string{"status", "cleanup"}
	for _, name := range visible {
		t.Run(name+" has Hidden=false", func(t *testing.T) {
			var match bool
			for _, c := range stateCmd.Commands() {
				if c.Name() == name {
					match = true
					if c.Hidden {
						t.Errorf("subcommand %q must not be hidden", name)
					}
					break
				}
			}
			if !match {
				t.Errorf("subcommand %q not registered under state", name)
			}
		})
	}
}

func TestStateHiddenSubcommandsAbsentFromShellCompletions(t *testing.T) {
	hidden := []string{"daemon", "notify", "signal-hydrate", "hydrate", "migrate-rename"}

	t.Run("bash completion does not reference hidden subcommands", func(t *testing.T) {
		buf := new(bytes.Buffer)
		if err := rootCmd.GenBashCompletionV2(buf, true); err != nil {
			t.Fatalf("GenBashCompletionV2: %v", err)
		}
		out := buf.String()
		for _, h := range hidden {
			if strings.Contains(out, h) {
				t.Errorf("bash completion contains hidden subcommand %q", h)
			}
		}
	})

	t.Run("zsh completion does not reference hidden subcommands", func(t *testing.T) {
		buf := new(bytes.Buffer)
		if err := rootCmd.GenZshCompletion(buf); err != nil {
			t.Fatalf("GenZshCompletion: %v", err)
		}
		out := buf.String()
		for _, h := range hidden {
			if strings.Contains(out, h) {
				t.Errorf("zsh completion contains hidden subcommand %q", h)
			}
		}
	})

	t.Run("fish completion does not reference hidden subcommands", func(t *testing.T) {
		buf := new(bytes.Buffer)
		if err := rootCmd.GenFishCompletion(buf, true); err != nil {
			t.Fatalf("GenFishCompletion: %v", err)
		}
		out := buf.String()
		for _, h := range hidden {
			if strings.Contains(out, h) {
				t.Errorf("fish completion contains hidden subcommand %q", h)
			}
		}
	})
}
