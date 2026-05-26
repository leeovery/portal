package cmd

// Tests in this file mutate package-level state (bootstrapDeps, listDeps) and MUST NOT use t.Parallel.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// resetRootCmd resets the root command's output streams and subcommand flags for testing.
func resetRootCmd() {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	_ = initCmd.Flags().Set("cmd", "x")       // reset to default; value is always valid
	_ = listCmd.Flags().Set("short", "false") // reset list flags
	_ = listCmd.Flags().Set("long", "false")
	if f := openCmd.Flags().Lookup("exec"); f != nil { // reset exec flag
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := hooksSetCmd.Flags().Lookup("on-resume"); f != nil { // reset hooks set flags
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := hooksRmCmd.Flags().Lookup("on-resume"); f != nil { // reset hooks rm flags
		_ = f.Value.Set("false")
		f.Changed = false
	}
	if f := hooksRmCmd.Flags().Lookup("pane-key"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
}

func TestTmuxDependentCommandsFailWithoutTmux(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "portal open fails without tmux", args: []string{"open"}},
		{name: "portal list fails without tmux", args: []string{"list"}},
		{name: "portal attach fails without tmux", args: []string{"attach", "test-session"}},
		{name: "portal kill fails without tmux", args: []string{"kill", "test-session"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", "/nonexistent/path")

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			errMsg := err.Error()
			want := "Portal requires tmux. Install with: brew install tmux"
			if errMsg != want {
				t.Errorf("error = %q, want %q", errMsg, want)
			}
		})
	}
}

func TestNonTmuxCommandsWorkWithoutTmux(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
	}{
		{name: "portal version works without tmux", args: []string{"version"}},
		{name: "portal init works without tmux", args: []string{"init", "zsh"}},
		{name: "portal help works without tmux", args: []string{"help"}},
		{
			name: "portal alias set works without tmux",
			args: []string{"alias", "set", "proj", "/some/path"},
			env:  map[string]string{"PORTAL_ALIASES_FILE": "TEMPDIR/aliases"},
		},
		{
			name: "portal clean works without tmux",
			args: []string{"clean"},
			env:  map[string]string{"PORTAL_PROJECTS_FILE": "TEMPDIR/projects.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", "/nonexistent/path")

			for k, v := range tt.env {
				if strings.HasPrefix(v, "TEMPDIR/") {
					v = filepath.Join(t.TempDir(), strings.TrimPrefix(v, "TEMPDIR/"))
				}
				t.Setenv(k, v)
			}

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRootCommandExecutesWithoutError(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{})
	err := rootCmd.Execute()

	if err != nil {
		t.Fatalf("root command returned error: %v", err)
	}
}

func TestOpenSubcommandIsRegistered(t *testing.T) {
	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "open" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("open subcommand is not registered on root command")
	}
}

func TestTmuxDependentCommandsSucceedWithTmux(t *testing.T) {
	// Ensure tmux is actually available for this test
	originalPath := os.Getenv("PATH")
	if originalPath == "" {
		t.Skip("PATH not set")
	}

	tests := []struct {
		name string
		args []string
	}{
		// open is excluded: it launches a full-screen TUI requiring a TTY
		{name: "portal list succeeds with tmux", args: []string{"list"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// nopRunner satisfies bootstrap.Runner with a Run that does nothing and
// returns (false, nil, nil). Substituted into BootstrapDeps.Orchestrator
// for tests that don't care about bootstrap behaviour.
type nopRunner struct{}

// Run is a no-op for tests that don't care about bootstrap behaviour.
func (nopRunner) Run(_ context.Context) (bool, []bootstrap.Warning, error) {
	return false, nil, nil
}

// panicRunner satisfies bootstrap.Runner but panics on any Run
// invocation. Used to prove PersistentPreRunE never reaches bootstrap
// for skip-tmux commands or short-circuit paths.
type panicRunner struct{}

// Run panics; never expected to be called in tests using this fake.
func (panicRunner) Run(_ context.Context) (bool, []bootstrap.Warning, error) {
	panic("buildBootstrapDeps / Run must not be reached")
}

// errRunner returns the configured error from Run verbatim. Used by
// tests asserting non-fatal bootstrap errors propagate without
// wrapping.
type errRunner struct {
	err error
}

// Run returns (false, nil, r.err) verbatim.
func (r *errRunner) Run(_ context.Context) (bool, []bootstrap.Warning, error) {
	return false, nil, r.err
}

func TestPersistentPreRunE_CallsEnsureServer(t *testing.T) {
	t.Run("orchestrator Run called for tmux-requiring commands", func(t *testing.T) {
		runner := &recordingRunner{}
		bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
		t.Cleanup(func() { bootstrapDeps = nil })

		listDeps = &ListDeps{
			Lister: &mockSessionLister{sessions: []tmux.Session{}},
			IsTTY:  func() bool { return false },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"list"})
		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.calls != 1 {
			t.Errorf("orchestrator Run call count = %d, want 1", runner.calls)
		}
	})

	t.Run("orchestrator error propagates to caller", func(t *testing.T) {
		runner := &recordingRunner{err: fmt.Errorf("failed to start tmux server: permission denied")}
		bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
		t.Cleanup(func() { bootstrapDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"list"})
		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		want := "failed to start tmux server: permission denied"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("orchestrator Run not called for skipTmuxCheck commands", func(t *testing.T) {
		runner := &recordingRunner{}
		bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
		t.Cleanup(func() { bootstrapDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"version"})
		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if runner.calls != 0 {
			t.Errorf("orchestrator Run call count for skip command = %d, want 0", runner.calls)
		}
	})

	t.Run("PersistentPreRunE stores serverStarted=true in context", func(t *testing.T) {
		runner := &recordingRunner{started: true}
		bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
		t.Cleanup(func() { bootstrapDeps = nil })

		// Create a test command that captures the context value from RunE
		var gotStarted bool
		testCmd := &cobra.Command{
			Use: "testcmd",
			RunE: func(cmd *cobra.Command, args []string) error {
				gotStarted = serverWasStarted(cmd)
				return nil
			},
		}
		rootCmd.AddCommand(testCmd)
		t.Cleanup(func() { rootCmd.RemoveCommand(testCmd) })

		resetRootCmd()
		rootCmd.SetArgs([]string{"testcmd"})
		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !gotStarted {
			t.Error("expected serverWasStarted=true, got false")
		}
	})

	t.Run("PersistentPreRunE stores serverStarted=false in context", func(t *testing.T) {
		runner := &recordingRunner{started: false}
		bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
		t.Cleanup(func() { bootstrapDeps = nil })

		var gotStarted bool
		var runCalled bool
		testCmd := &cobra.Command{
			Use: "testcmd2",
			RunE: func(cmd *cobra.Command, args []string) error {
				runCalled = true
				gotStarted = serverWasStarted(cmd)
				return nil
			},
		}
		rootCmd.AddCommand(testCmd)
		t.Cleanup(func() { rootCmd.RemoveCommand(testCmd) })

		resetRootCmd()
		rootCmd.SetArgs([]string{"testcmd2"})
		err := rootCmd.Execute()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !runCalled {
			t.Fatal("RunE was not called")
		}
		if gotStarted {
			t.Error("expected serverWasStarted=false, got true")
		}
	})
}

// recordingHookRegistrar records every call to its Register method along
// with the *tmux.Client argument it received. Substituted into BootstrapDeps
// to assert PersistentPreRunE invokes hook registration after bootstrap.
type recordingHookRegistrar struct {
	calls   int
	gotNil  bool
	gotSame bool
	want    *tmux.Client
	err     error
}

func (r *recordingHookRegistrar) Register(c *tmux.Client) error {
	r.calls++
	if c == nil {
		r.gotNil = true
	}
	if r.want != nil && c == r.want {
		r.gotSame = true
	}
	return r.err
}

func TestPersistentPreRunE_RegistersPortalHooks(t *testing.T) {
	t.Run("RegisterHooks is called once after orchestrator for non-exempt commands", func(t *testing.T) {
		runner := &recordingRunner{}
		client := tmux.NewClient(&tmux.RealCommander{})
		registrar := &recordingHookRegistrar{want: client}

		bootstrapDeps = &BootstrapDeps{
			Orchestrator:  runner,
			Client:        client,
			RegisterHooks: registrar.Register,
		}
		t.Cleanup(func() { bootstrapDeps = nil })

		listDeps = &ListDeps{
			Lister: &mockSessionLister{sessions: []tmux.Session{}},
			IsTTY:  func() bool { return false },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"list"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if runner.calls != 1 {
			t.Errorf("orchestrator Run call count = %d, want 1", runner.calls)
		}
		if registrar.calls != 1 {
			t.Errorf("RegisterHooks call count = %d, want 1", registrar.calls)
		}
		if registrar.gotNil {
			t.Error("RegisterHooks received nil client")
		}
		if !registrar.gotSame {
			t.Error("RegisterHooks did not receive the bootstrapped client instance")
		}
	})

	t.Run("RegisterHooks is NOT called for exempt commands", func(t *testing.T) {
		exempt := []struct {
			name string
			args []string
		}{
			{name: "portal version", args: []string{"version"}},
			{name: "portal state status", args: []string{"state", "status"}},
		}
		for _, tt := range exempt {
			t.Run(tt.name, func(t *testing.T) {
				registrar := &recordingHookRegistrar{}
				bootstrapDeps = &BootstrapDeps{
					Orchestrator:  &nopRunner{},
					RegisterHooks: registrar.Register,
				}
				t.Cleanup(func() { bootstrapDeps = nil })

				// state status renders a real diagnostic and exits
				// non-zero on an unhealthy surface. Point at an empty
				// TempDir; ErrStatusUnhealthy is irrelevant to the
				// RegisterHooks assertion below.
				if len(tt.args) >= 2 && tt.args[0] == "state" && tt.args[1] == "status" {
					t.Setenv("PORTAL_STATE_DIR", t.TempDir())
				}

				resetRootCmd()
				resetStateCmdFlags()
				rootCmd.SetArgs(tt.args)
				err := rootCmd.Execute()
				if err != nil && err != ErrStatusUnhealthy {
					t.Fatalf("unexpected error: %v", err)
				}

				if registrar.calls != 0 {
					t.Errorf("RegisterHooks call count for exempt command = %d, want 0", registrar.calls)
				}
			})
		}
	})

	t.Run("RegisterHooks error propagates from PersistentPreRunE", func(t *testing.T) {
		sentinel := errors.New("hook registration failed")
		client := tmux.NewClient(&tmux.RealCommander{})
		registrar := &recordingHookRegistrar{err: sentinel}

		bootstrapDeps = &BootstrapDeps{
			Orchestrator:  &nopRunner{},
			Client:        client,
			RegisterHooks: registrar.Register,
		}
		t.Cleanup(func() { bootstrapDeps = nil })

		listDeps = &ListDeps{
			Lister: &mockSessionLister{sessions: []tmux.Session{}},
			IsTTY:  func() bool { return false },
		}
		t.Cleanup(func() { listDeps = nil })

		resetRootCmd()
		rootCmd.SetArgs([]string{"list"})
		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error from RegisterHooks, got nil")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
		}
		if registrar.calls != 1 {
			t.Errorf("RegisterHooks call count = %d, want 1", registrar.calls)
		}
	})
}

// fatalRunner returns a pre-built *bootstrap.FatalError so tests can
// drive the FatalError-propagation paths without spinning up the full
// Orchestrator step graph.
type fatalRunner struct {
	fatal *bootstrap.FatalError
}

func (r *fatalRunner) Run(_ context.Context) (bool, []bootstrap.Warning, error) {
	return false, nil, r.fatal
}

func TestPersistentPreRunE_WrapsCheckTmuxAvailableErrorAsFatal(t *testing.T) {
	t.Setenv("PATH", "/nonexistent/path")

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var fatal *bootstrap.FatalError
	if !errors.As(err, &fatal) {
		t.Fatalf("expected *bootstrap.FatalError, got %T (%v)", err, err)
	}
	want := "Portal requires tmux. Install with: brew install tmux"
	if fatal.UserMessage != want {
		t.Errorf("UserMessage = %q, want %q", fatal.UserMessage, want)
	}
}

func TestPersistentPreRunE_WrapsVersionCheckErrorAsFatal(t *testing.T) {
	resetVersionCheckForTest()
	t.Cleanup(resetVersionCheckForTest)

	original := versionChecker
	versionChecker = func(tmux.Commander) error {
		return errors.New("Portal requires tmux \u2265 3.0 (found 2.9). Please upgrade.")
	}
	t.Cleanup(func() { versionChecker = original })

	bootstrapDeps = &BootstrapDeps{Orchestrator: &nopRunner{}}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var fatal *bootstrap.FatalError
	if !errors.As(err, &fatal) {
		t.Fatalf("expected *bootstrap.FatalError, got %T (%v)", err, err)
	}
	want := "Portal requires tmux \u2265 3.0 (found 2.9). Please upgrade."
	if fatal.UserMessage != want {
		t.Errorf("UserMessage = %q, want %q", fatal.UserMessage, want)
	}
}

func TestPersistentPreRunE_OrchestratorFatalErrorPropagatesUnwrapped(t *testing.T) {
	resetBootstrapOnce(t)

	cause := errors.New("hooks boom")
	want := "Portal failed to register tmux hooks: hooks boom"
	runner := &fatalRunner{fatal: bootstrap.NewFatal(want, cause)}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var fatal *bootstrap.FatalError
	if !errors.As(err, &fatal) {
		t.Fatalf("expected *bootstrap.FatalError, got %T (%v)", err, err)
	}
	if fatal.UserMessage != want {
		t.Errorf("UserMessage = %q, want %q", fatal.UserMessage, want)
	}
	if !errors.Is(err, cause) {
		t.Errorf("expected errors.Is(err, cause) to be true; err = %v", err)
	}
}

func TestExecute_EmitsFatalUserMessageToStderr(t *testing.T) {
	resetBootstrapOnce(t)

	want := "Portal failed to register tmux hooks: synthetic"
	runner := &fatalRunner{fatal: bootstrap.NewFatal(want, errors.New("synthetic"))}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	var stderr bytes.Buffer
	originalWriter := fatalErrorStderr
	fatalErrorStderr = &stderr
	t.Cleanup(func() { fatalErrorStderr = originalWriter })

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	err := Execute()

	if err == nil {
		t.Fatal("expected Execute to return error, got nil")
	}
	var fatal *bootstrap.FatalError
	if !errors.As(err, &fatal) {
		t.Fatalf("expected *bootstrap.FatalError, got %T (%v)", err, err)
	}

	got := stderr.String()
	wantOutput := want + "\n"
	if got != wantOutput {
		t.Errorf("stderr = %q, want %q (single line + newline)", got, wantOutput)
	}
	// Spec: single line. Reject any extra content.
	if strings.Count(got, "\n") != 1 {
		t.Errorf("stderr contained %d newlines; want exactly 1", strings.Count(got, "\n"))
	}
}

func TestExecute_NonFatalErrorWritesNothingToFatalStream(t *testing.T) {
	resetBootstrapOnce(t)

	// Use a plain errRunner — its Run returns the configured error
	// verbatim, without wrapping in FatalError. Verifies Execute writes
	// nothing to fatalErrorStderr when the error is non-fatal.
	runner := &errRunner{err: errors.New("transient")}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	var stderr bytes.Buffer
	originalWriter := fatalErrorStderr
	fatalErrorStderr = &stderr
	t.Cleanup(func() { fatalErrorStderr = originalWriter })

	resetRootCmd()
	rootCmd.SetArgs([]string{"list"})
	err := Execute()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var fatal *bootstrap.FatalError
	if errors.As(err, &fatal) {
		t.Fatalf("non-fatal error must not be *FatalError; got %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("fatalErrorStderr unexpectedly written: %q", stderr.String())
	}
}
