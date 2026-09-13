package cmd

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
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func resetRootCmd() {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	// Cobra reuses a child's existing ctx when non-nil, so a context a prior
	// Execute stashed would otherwise leak across tests.
	rootCmd.SetContext(context.Background())
	for _, c := range rootCmd.Commands() {
		c.SetContext(context.Background())
	}
	// A --help run leaves the flag Changed, which makes every later Execute of
	// that command print help instead of running it.
	resetHelpFlag(rootCmd)
	for _, c := range rootCmd.Commands() {
		resetHelpFlag(c)
	}
	_ = initCmd.Flags().Set("cmd", "x")
	_ = listCmd.Flags().Set("short", "false")
	_ = listCmd.Flags().Set("long", "false")
	if f := openCmd.Flags().Lookup("exec"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := openCmd.Flags().Lookup("filter"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := openCmd.Flags().Lookup("session"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := openCmd.Flags().Lookup("path"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := openCmd.Flags().Lookup("alias"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := openCmd.Flags().Lookup("zoxide"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := openCmd.Flags().Lookup("ack"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	// pflag never resets argsLenAtDash between Parse calls, so a prior
	// `open <t> -- cmd` leaves a positive dash index that a later no-`--`
	// Execute would slice out of range. Re-Init restores it to -1 without
	// disturbing the defined flags.
	openCmd.Flags().Init(openCmd.Name(), pflag.ContinueOnError)
	if f := hooksSetCmd.Flags().Lookup("on-resume"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := hooksRmCmd.Flags().Lookup("on-resume"); f != nil {
		_ = f.Value.Set("false")
		f.Changed = false
	}
	if f := hooksRmCmd.Flags().Lookup("pane-key"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}
	if f := doctorCmd.Flags().Lookup("fix"); f != nil {
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

func resetHelpFlag(cmd *cobra.Command) {
	if f := cmd.Flags().Lookup("help"); f != nil {
		_ = f.Value.Set("false")
		f.Changed = false
	}
}

// runRootCmd executes the root command with args, handing back the buffers its
// stdout and stderr were routed to alongside the Execute error.
func runRootCmd(t *testing.T, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	resetRootCmd()
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return outBuf, errBuf, err
}

func TestTmuxDependentCommandsFailWithoutTmux(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "portal open fails without tmux", args: []string{"open"}},
		{name: "portal list fails without tmux", args: []string{"list"}},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", "/nonexistent/path")

			for k, v := range tt.env {
				if after, ok := strings.CutPrefix(v, "TEMPDIR/"); ok {
					v = filepath.Join(t.TempDir(), after)
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

func TestSpawnCommandIsRetired(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "spawn" {
			t.Fatal("spawn command must be retired (not registered on rootCmd)")
		}
	}
}

func TestTmuxDependentCommandsSucceedWithTmux(t *testing.T) {
	originalPath := os.Getenv("PATH")
	if originalPath == "" {
		t.Skip("PATH not set")
	}

	tests := []struct {
		name string
		args []string
	}{
		// open is excluded: it launches a full-screen TUI requiring a TTY.
		{name: "portal list succeeds with tmux", args: []string{"list"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Everything downstream of the tmux-availability precheck must
			// stay injected: uninjected, this ran a complete production
			// bootstrap against the developer's real tmux server.
			stub := &stubVersionChecker{}
			installStubVersionChecker(t, stub)
			withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})
			withListDeps(t, ListDeps{
				Lister: &mockSessionLister{sessions: []tmux.Session{}},
				IsTTY:  func() bool { return false },
			})

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

type nopRunner struct{}

func (nopRunner) Run(_ context.Context) (bool, []bootstrap.Warning, error) {
	return false, nil, nil
}

type panicRunner struct{}

func (panicRunner) Run(_ context.Context) (bool, []bootstrap.Warning, error) {
	panic("buildBootstrapDeps / Run must not be reached")
}

type errRunner struct {
	err error
}

func (r *errRunner) Run(_ context.Context) (bool, []bootstrap.Warning, error) {
	return false, nil, r.err
}

func TestPersistentPreRunE_CallsEnsureServer(t *testing.T) {
	t.Run("orchestrator Run called for tmux-requiring commands", func(t *testing.T) {
		runner := &recordingRunner{}
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

		withListDeps(t, ListDeps{
			Lister: &mockSessionLister{sessions: []tmux.Session{}},
			IsTTY:  func() bool { return false },
		})

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
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
		// skipTmuxCheck keys on cobra's canonical c.Name(), so the `hooks`
		// alias rows exercise the alias resolving to the single `hook` entry.
		tests := []struct {
			name string
			argv []string
		}{
			{name: "version", argv: []string{"version"}},
			// Load-bearing: cobra's __complete runs the root
			// PersistentPreRunE, so without the exemption every TAB press
			// fires the full bootstrap and starts tmux.
			{name: "__complete open", argv: []string{"__complete", "open", ""}},
			{name: "hook list", argv: []string{"hook", "list"}},
			{name: "hook set", argv: []string{"hook", "set", "--on-resume", "true"}},
			{name: "hook rm", argv: []string{"hook", "rm", "--on-resume"}},
			{name: "hooks alias list", argv: []string{"hooks", "list"}},
			{name: "hooks alias set", argv: []string{"hooks", "set", "--on-resume", "true"}},
			{name: "hooks alias rm", argv: []string{"hooks", "rm", "--on-resume"}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				runner := &recordingRunner{}
				withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

				// Without these the command reaches real tmux and the
				// developer's config dir.
				if len(tt.argv) >= 2 {
					switch tt.argv[1] {
					case "set", "rm":
						hooksFileInTempDir(t, map[string]map[string]string{
							hookstest.UnjudgeableSeedA: {"on-resume": "claude --resume abc123"},
						})
						t.Setenv("TMUX_PANE", "%3")

						resolver := &mockKeyResolver{key: hookstest.UnjudgeableSeedA}
						withHooksDeps(t, HooksDeps{KeyResolver: resolver})
					case "list":
						// The list body resolves each entry's location through the
						// pane enumeration, so it is the empty store — not a real
						// tmux server — that must be what leaves the read untaken.
						withHooksDeps(t, HooksDeps{PaneLister: &loudPaneHookLister{t: t}})
					}
				}

				resetRootCmd()
				rootCmd.SetOut(new(bytes.Buffer))
				rootCmd.SetErr(new(bytes.Buffer))
				rootCmd.SetArgs(tt.argv)
				err := rootCmd.Execute()

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if runner.calls != 0 {
					t.Errorf("orchestrator Run call count for %v = %d, want 0", tt.argv, runner.calls)
				}
			})
		}
	})

	t.Run("PersistentPreRunE stores serverStarted=true in context", func(t *testing.T) {
		runner := &recordingRunner{started: true}
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
		// An unsatisfied latch is what routes to the full-bootstrap path; the
		// abridged path deliberately skips hook registration.
		client := notSatisfiedLatchClient()
		registrar := &recordingHookRegistrar{want: client}

		withBootstrapDeps(t, BootstrapDeps{
			Orchestrator:  runner,
			Client:        client,
			RegisterHooks: registrar.Register,
		})

		withListDeps(t, ListDeps{
			Lister: &mockSessionLister{sessions: []tmux.Session{}},
			IsTTY:  func() bool { return false },
		})

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
			{name: "portal state", args: []string{"state"}},
		}
		for _, tt := range exempt {
			t.Run(tt.name, func(t *testing.T) {
				registrar := &recordingHookRegistrar{}
				withBootstrapDeps(t, BootstrapDeps{
					Orchestrator:  &nopRunner{},
					RegisterHooks: registrar.Register,
				})

				resetRootCmd()
				rootCmd.SetArgs(tt.args)
				err := rootCmd.Execute()
				if err != nil {
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
		// An unsatisfied latch is what routes to the full-bootstrap path; the
		// abridged path would skip RegisterHooks entirely.
		client := notSatisfiedLatchClient()
		registrar := &recordingHookRegistrar{err: sentinel}

		withBootstrapDeps(t, BootstrapDeps{
			Orchestrator:  &nopRunner{},
			Client:        client,
			RegisterHooks: registrar.Register,
		})

		withListDeps(t, ListDeps{
			Lister: &mockSessionLister{sessions: []tmux.Session{}},
			IsTTY:  func() bool { return false },
		})

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

	withFuncSeam(t, &versionChecker, func(tmux.Commander) error {
		return errors.New("Portal requires tmux \u2265 3.0 (found 2.9). Please upgrade.")
	})

	withBootstrapDeps(t, BootstrapDeps{Orchestrator: &nopRunner{}})

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
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
	if _, ok := errors.AsType[*bootstrap.FatalError](err); !ok {
		t.Fatalf("expected *bootstrap.FatalError, got %T (%v)", err, err)
	}

	got := stderr.String()
	wantOutput := want + "\n"
	if got != wantOutput {
		t.Errorf("stderr = %q, want %q (single line + newline)", got, wantOutput)
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("stderr contained %d newlines; want exactly 1", strings.Count(got, "\n"))
	}
}

func TestExecute_NonFatalErrorWritesNothingToFatalStream(t *testing.T) {
	resetBootstrapOnce(t)

	// errRunner returns its error unwrapped, so nothing here is a FatalError.
	runner := &errRunner{err: errors.New("transient")}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
	if _, ok := errors.AsType[*bootstrap.FatalError](err); ok {
		t.Fatalf("non-fatal error must not be *FatalError; got %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("fatalErrorStderr unexpectedly written: %q", stderr.String())
	}
}
