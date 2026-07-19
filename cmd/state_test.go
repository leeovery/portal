// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
package cmd

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// availableCommandNames parses Cobra help output and returns the set of names
// listed under the "Available Commands:" section. Each listed line begins with
// two-space indentation followed by the command name.
func availableCommandNames(help string) map[string]bool {
	names := make(map[string]bool)
	inSection := false
	for line := range strings.SplitSeq(help, "\n") {
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

	t.Run("state is an internal group absent from portal --help", func(t *testing.T) {
		// After the cli-verb-surface redesign, every `state` child is hidden
		// plumbing (spec § Command Surface Summary → Hidden). With no user-facing
		// child and no Run, Cobra stops surfacing `state` in `portal --help`
		// Available Commands — it stays registered and invocable (proven by the
		// sibling subtest above), but it is no longer a visible public verb.
		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		listed := availableCommandNames(buf.String())
		if listed["state"] {
			t.Errorf("portal --help must not list 'state' now that it has no user-facing children; got %v", listed)
		}
	})

	t.Run("portal state --help lists no user-facing subcommands", func(t *testing.T) {
		buf := new(bytes.Buffer)
		resetRootCmd()
		rootCmd.SetOut(buf)
		rootCmd.SetErr(buf)
		rootCmd.SetArgs([]string{"state", "--help"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		listed := availableCommandNames(buf.String())

		// status was removed (subsumed by `portal doctor`); cleanup was removed
		// (replaced by `portal uninstall`). After the cli-verb-surface redesign,
		// state has NO user-facing children — every remaining child is hidden.
		for _, removed := range []string{"status", "cleanup"} {
			if listed[removed] {
				t.Errorf("portal state --help must not list removed subcommand %q; got %v", removed, listed)
			}
		}
		// hidden subcommands must never appear
		hidden := []string{"daemon", "notify", "signal-hydrate", "hydrate", "migrate-rename", "commit-now"}
		for _, h := range hidden {
			if listed[h] {
				t.Errorf("portal state --help must not list hidden subcommand %q; got %v", h, listed)
			}
		}
		// only Cobra's built-in `help` / `completion` may remain
		for name := range listed {
			if name == "help" || name == "completion" {
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

		// hidden subcommands of state must not surface at root. (state itself is
		// no longer a visible top-level command — see the internal-group subtest
		// above — so we only assert its hidden plumbing children never leak up.)
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
	// Cobra default help output for a parent command includes the "Usage:"
	// section and the state Short description even when every child is hidden.
	if !strings.Contains(out, "Usage:") && !strings.Contains(out, "Manage Portal session resurrection state") {
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

// stateChildCommands is the canonical list of the six hidden `state` children,
// referenced by their package-level command vars so a rename or a dropped
// registration is a compile error rather than a silent miss.
var stateChildCommands = []*cobra.Command{
	stateDaemonCmd,
	stateHydrateCmd,
	stateSignalHydrateCmd,
	stateNotifyCmd,
	stateCommitNowCmd,
	stateMigrateRenameCmd,
}

// TestStateParentIsHidden locks the parent stateCmd as Hidden so the entire
// `state` subtree drops out of `portal --help` and generated completions in one
// move, independent of any future child. Hiding marks visibility only — every
// child stays fully argv-invocable (see TestStateChildrenRemainInvocableByArgv
// and TestStateInternalSubcommandsAcceptValidArgv).
func TestStateParentIsHidden(t *testing.T) {
	if !stateCmd.Hidden {
		t.Error("stateCmd.Hidden = false; want true (the whole state subtree must be hidden)")
	}
	if stateCmd.IsAvailableCommand() {
		t.Error("stateCmd.IsAvailableCommand() = true; want false (hidden parent with no user-facing children)")
	}
}

func TestStateHiddenSubcommandsAreHidden(t *testing.T) {
	t.Run("each of the six child command vars is Hidden", func(t *testing.T) {
		for _, c := range stateChildCommands {
			if !c.Hidden {
				t.Errorf("state child %q must have Hidden=true", c.Name())
			}
		}
	})

	// Every registered child must be hidden plumbing. Iterating the live child
	// set (which contains only the six real children — cobra adds no help /
	// completion command under a subcommand) means a future child added without
	// Hidden fails loudly here.
	t.Run("every registered state child is Hidden", func(t *testing.T) {
		children := stateCmd.Commands()
		if len(children) != len(stateChildCommands) {
			t.Errorf("state has %d children, want %d; a new child must be added to stateChildCommands and marked Hidden", len(children), len(stateChildCommands))
		}
		for _, c := range children {
			if !c.Hidden {
				t.Errorf("registered state child %q is not Hidden", c.Name())
			}
		}
	})
}

// TestStateChildrenRemainInvocableByArgv proves Hidden marks visibility only, not
// execution: every child still resolves through rootCmd.Find and would dispatch.
// The daemon, the hydrate helpers, and reboot hook-firing all invoke these by
// argv, so this invariant is load-bearing.
func TestStateChildrenRemainInvocableByArgv(t *testing.T) {
	names := []string{"daemon", "hydrate", "signal-hydrate", "notify", "commit-now", "migrate-rename"}
	for _, name := range names {
		t.Run(name+" resolves via Find", func(t *testing.T) {
			resetRootCmd()
			found, _, err := rootCmd.Find([]string{"state", name})
			if err != nil {
				t.Fatalf("rootCmd.Find([state %s]): %v", name, err)
			}
			if found.Name() != name {
				t.Fatalf("rootCmd.Find([state %s]) resolved to %q; Hidden must not disable resolution", name, found.Name())
			}
		})
	}
}

func TestStateHiddenSubcommandsAbsentFromShellCompletions(t *testing.T) {
	// All six hidden children plus the parent must be gone from every shell.
	hidden := []string{"daemon", "notify", "signal-hydrate", "hydrate", "migrate-rename", "commit-now"}
	// Whole-word matcher for the parent `state` entry: the completion boilerplate
	// contains the word "statement(s)", so a bare substring check for "state"
	// false-positives. \bstate\b matches only a standalone `state` command entry.
	wholeState := regexp.MustCompile(`\bstate\b`)

	shells := []struct {
		name string
		gen  func(*bytes.Buffer) error
	}{
		{"bash", func(b *bytes.Buffer) error { return rootCmd.GenBashCompletionV2(b, true) }},
		{"zsh", func(b *bytes.Buffer) error { return rootCmd.GenZshCompletion(b) }},
		{"fish", func(b *bytes.Buffer) error { return rootCmd.GenFishCompletion(b, true) }},
	}

	for _, sh := range shells {
		t.Run(sh.name+" completion omits the hidden state subtree", func(t *testing.T) {
			buf := new(bytes.Buffer)
			if err := sh.gen(buf); err != nil {
				t.Fatalf("generate %s completion: %v", sh.name, err)
			}
			out := buf.String()
			for _, h := range hidden {
				if strings.Contains(out, h) {
					t.Errorf("%s completion contains hidden subcommand %q", sh.name, h)
				}
			}
			if wholeState.MatchString(out) {
				t.Errorf("%s completion contains a bare 'state' command entry", sh.name)
			}
		})
	}
}
