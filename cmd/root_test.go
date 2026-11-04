package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
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
	if f := hooksSetCmd.Flags().Lookup(resumeModeFlagName); f != nil {
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
	// Cobra's required-flag check is satisfied by a flag left Changed on the
	// shared command instance by an earlier Execute, so a refusal subtest would
	// pass while checking nothing.
	for _, c := range stateChildCommands {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
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

func TestExemptCommandsWorkWithNothingOnPath(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		// stage sets up what the command reads beyond its env.
		stage func(t *testing.T)
	}{
		{name: "portal version works with nothing on PATH", args: []string{"version"}},
		{name: "portal init works with nothing on PATH", args: []string{"init", "zsh"}},
		{name: "portal help works with nothing on PATH", args: []string{"help"}},
		{
			name: "portal alias set works with nothing on PATH",
			args: []string{"alias", "set", "proj", "/some/path"},
			env:  map[string]string{"PORTAL_ALIASES_FILE": "TEMPDIR/aliases"},
		},
		{
			name: "portal current works with nothing on PATH",
			args: []string{"current"},
			env:  map[string]string{"ZMX_SESSION": currentSessionName, "PORTAL_STATE_DIR": "TEMPDIR/"},
		},
		{
			name:  "portal rename works with nothing on PATH",
			args:  []string{"rename", "api work"},
			stage: func(t *testing.T) { stageRename(t) },
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
			if tt.stage != nil {
				tt.stage(t)
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

type nopRunner struct{}

func (nopRunner) Run(_ context.Context) ([]bootstrap.Warning, error) {
	return nil, nil
}

type panicRunner struct{}

func (panicRunner) Run(_ context.Context) ([]bootstrap.Warning, error) {
	panic("buildBootstrapDeps / Run must not be reached")
}

type errRunner struct {
	err error
}

func (r *errRunner) Run(_ context.Context) ([]bootstrap.Warning, error) {
	return nil, r.err
}

func TestPersistentPreRunE_RunsAnInjectedOrchestrator(t *testing.T) {
	t.Run("orchestrator Run called for non-exempt commands", func(t *testing.T) {
		runner := &recordingRunner{}
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})
		withBackendDeps(t, BackendDeps{Backend: backendtest.New()})

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
		runner := &recordingRunner{err: fmt.Errorf("take bootstrap.lock: permission denied")}
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

		resetRootCmd()
		rootCmd.SetArgs([]string{"list"})
		err := rootCmd.Execute()

		if err == nil {
			t.Fatal("expected error, got nil")
		}
		want := "take bootstrap.lock: permission denied"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("orchestrator Run not called for bootstrap-exempt commands", func(t *testing.T) {
		// bootstrapExempt keys on cobra's canonical c.Name(), so the `hooks`
		// alias rows exercise the alias resolving to the single `hook` entry.
		tests := []struct {
			name string
			argv []string
		}{
			{name: "version", argv: []string{"version"}},
			// Load-bearing: cobra's __complete runs the root
			// PersistentPreRunE, so without the exemption every TAB press
			// fires the full bootstrap.
			{name: "__complete open", argv: []string{"__complete", "open", ""}},
			{name: "hook list", argv: []string{"hook", "list"}},
			{name: "hook set", argv: []string{"hook", "set", "--on-resume", "true"}},
			{name: "hook rm", argv: []string{"hook", "rm", "--on-resume"}},
			{name: "hooks alias list", argv: []string{"hooks", "list"}},
			{name: "hooks alias set", argv: []string{"hooks", "set", "--on-resume", "true"}},
			{name: "hooks alias rm", argv: []string{"hooks", "rm", "--on-resume"}},
			{name: "current", argv: []string{"current"}},
			{name: "rename", argv: []string{"rename", "api work"}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				runner := &recordingRunner{}
				withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

				// Without these the command reaches the developer's config
				// and state dirs.
				hooksFileInTempDir(t, map[string]map[string]string{
					hookSessionA: {"on-resume": "claude --resume abc123"},
				})
				inSession(t, hookSessionA)

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
}

type fatalRunner struct {
	fatal *bootstrap.FatalError
}

func (r *fatalRunner) Run(_ context.Context) ([]bootstrap.Warning, error) {
	return nil, r.fatal
}

func TestPersistentPreRunE_OrchestratorFatalErrorPropagatesUnwrapped(t *testing.T) {
	resetBootstrapOnce(t)

	cause := errors.New("permission denied")
	want := "Portal failed to raise the restoring flag: permission denied"
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

	want := "Portal failed to raise the restoring flag: synthetic"
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

// decision 75: a mint whose create the backend refused, because a zmx 0.8.1
// first on PATH made the session, fails with the older-zmx fatal, as a
// bootstrap meeting that zmx does.
func TestExecute_UnsupportedCreateIsTheOlderZmxFatal(t *testing.T) {
	fake, _ := stageAttach(t)
	stageMint(t, fake, time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC))
	fake.Fail(backendtest.OpCreate, fmt.Errorf("create session %q: %w; zmx kill ended the session", "x", backend.ErrUnsupportedProtocol))

	var stderr bytes.Buffer
	originalWriter := fatalErrorStderr
	fatalErrorStderr = &stderr
	t.Cleanup(func() { fatalErrorStderr = originalWriter })

	resetRootCmd()
	rootCmd.SetArgs([]string{"open", t.TempDir()})
	err := Execute()

	fatal, ok := errors.AsType[*bootstrap.FatalError](err)
	if !ok || fatal.UserMessage != bootstrap.OlderZmxMessage || !errors.Is(err, backend.ErrUnsupportedProtocol) {
		t.Fatalf("Execute = %v, want the older-zmx fatal carrying the refusal", err)
	}
	if got, want := stderr.String(), bootstrap.OlderZmxMessage+"\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
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

func TestResetRootCmdClearsStateChildFlags(t *testing.T) {
	t.Run("it clears every flag on every state child", func(t *testing.T) {
		reset := 0
		for _, c := range stateChildCommands {
			c.Flags().VisitAll(func(f *pflag.Flag) {
				if err := f.Value.Set(nonDefaultFlagValue(t, f)); err != nil {
					t.Fatalf("setting %s --%s to a non-default: %v", c.Name(), f.Name, err)
				}
				f.Changed = true
				reset++
			})
		}

		resetRootCmd()

		for _, c := range stateChildCommands {
			c.Flags().VisitAll(func(f *pflag.Flag) {
				if got := f.Value.String(); got != f.DefValue {
					t.Errorf("%s --%s = %q after resetRootCmd, want its default %q", c.Name(), f.Name, got, f.DefValue)
				}
				if f.Changed {
					t.Errorf("%s --%s is still Changed after resetRootCmd", c.Name(), f.Name)
				}
			})
		}
		if reset == 0 {
			t.Fatal("no state child declares a flag; the reset was asserted over nothing")
		}
	})

	t.Run("it refuses a resume-wait naming no command after a sibling set one", func(t *testing.T) {
		withFuncSeam(t, &resumeWaitRunFunc, func(resumeWaitConfig) error { return nil })

		payload := resumeChainPayload{Command: "make deploy", Session: "proj-a1b2"}
		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		errBuf := new(bytes.Buffer)
		rootCmd.SetErr(errBuf)
		rootCmd.SetArgs(resumeChainArgv("portal", resumeWaitSubcommand, payload)[1:])
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("executing the composed argv: %v\nstderr: %s", err, errBuf)
		}

		resetRootCmd()
		rootCmd.SetOut(new(bytes.Buffer))
		rootCmd.SetErr(new(bytes.Buffer))
		rootCmd.SetArgs([]string{"state", resumeWaitSubcommand, "--session", "proj-a1b2"})
		if err := rootCmd.Execute(); err == nil {
			t.Error("executing resume-wait with no --command succeeded; the sibling's --command outlived the reset")
		}
	})
}

func nonDefaultFlagValue(t *testing.T, f *pflag.Flag) string {
	t.Helper()
	switch f.Value.Type() {
	case "bool":
		if f.DefValue == "true" {
			return "false"
		}
		return "true"
	case "int":
		if f.DefValue == "7" {
			return "9"
		}
		return "7"
	case "string":
		return "set-by-a-prior-run"
	}
	t.Fatalf("no non-default value known for --%s of type %s", f.Name, f.Value.Type())
	return ""
}
