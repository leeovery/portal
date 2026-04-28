package cmd

// Tests in this file mutate package-level state (bootstrapDeps, the
// memoisation gate) and MUST NOT use t.Parallel.

import (
	"context"
	"errors"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// recordingRunner counts Run invocations and returns a configurable
// (started, err) tuple. Substituted into BootstrapDeps.Orchestrator to
// verify PersistentPreRunE memoises the orchestrator call across repeated
// invocations.
type recordingRunner struct {
	calls   int
	started bool
	err     error
}

func (r *recordingRunner) Run(_ context.Context) (bool, error) {
	r.calls++
	return r.started, r.err
}

func TestPersistentPreRunE_OrchestratorMemoisedAcrossInvocations(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{started: true}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner, ForceMemoise: true}
	t.Cleanup(func() { bootstrapDeps = nil })

	listDeps = &ListDeps{
		Lister: &mockSessionLister{sessions: []tmux.Session{}},
		IsTTY:  func() bool { return false },
	}
	t.Cleanup(func() { listDeps = nil })

	for i := 0; i < 3; i++ {
		resetRootCmd()
		rootCmd.SetArgs([]string{"list"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}

	if runner.calls != 1 {
		t.Errorf("orchestrator Run call count = %d, want 1 across 3 invocations", runner.calls)
	}
}

func TestPersistentPreRunE_PopulatesServerStartedFromOrchestrator(t *testing.T) {
	t.Run("started=true", func(t *testing.T) {
		resetBootstrapOnce(t)

		runner := &recordingRunner{started: true}
		bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
		t.Cleanup(func() { bootstrapDeps = nil })

		var got bool
		var ran bool
		probe := &cobra.Command{
			Use: "orchprobe1",
			RunE: func(cmd *cobra.Command, args []string) error {
				got = serverWasStarted(cmd)
				ran = true
				return nil
			},
		}
		rootCmd.AddCommand(probe)
		t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

		resetRootCmd()
		rootCmd.SetArgs([]string{"orchprobe1"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ran {
			t.Fatal("probe RunE was not reached")
		}
		if !got {
			t.Errorf("serverWasStarted = false, want true")
		}
	})

	t.Run("started=false", func(t *testing.T) {
		resetBootstrapOnce(t)

		runner := &recordingRunner{started: false}
		bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
		t.Cleanup(func() { bootstrapDeps = nil })

		var got bool
		var ran bool
		probe := &cobra.Command{
			Use: "orchprobe2",
			RunE: func(cmd *cobra.Command, args []string) error {
				got = serverWasStarted(cmd)
				ran = true
				return nil
			},
		}
		rootCmd.AddCommand(probe)
		t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

		resetRootCmd()
		rootCmd.SetArgs([]string{"orchprobe2"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ran {
			t.Fatal("probe RunE was not reached")
		}
		if got {
			t.Errorf("serverWasStarted = true, want false")
		}
	})
}

func TestPersistentPreRunE_OrchestratorErrorPropagatesAndContextNotPopulated(t *testing.T) {
	resetBootstrapOnce(t)

	sentinel := errors.New("orchestrator boom")
	runner := &recordingRunner{started: true, err: sentinel}
	bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
	t.Cleanup(func() { bootstrapDeps = nil })

	// Probe RunE must NOT execute on orchestrator failure — Cobra short-
	// circuits the chain after PersistentPreRunE returns an error. If Run
	// were reached the panic from tmuxClient(cmd) would surface (no
	// client set in context), which is the exact "fail fast" semantic the
	// task asks us to preserve.
	probeRan := false
	probe := &cobra.Command{
		Use: "orchprobeerr",
		RunE: func(cmd *cobra.Command, args []string) error {
			probeRan = true
			return nil
		},
	}
	rootCmd.AddCommand(probe)
	t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

	resetRootCmd()
	rootCmd.SetArgs([]string{"orchprobeerr"})
	err := rootCmd.Execute()

	if err == nil {
		t.Fatal("expected error from orchestrator, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error %v does not wrap sentinel %v", err, sentinel)
	}
	if probeRan {
		t.Error("RunE must not run when PersistentPreRunE returns an error")
	}
}

func TestPersistentPreRunE_DoesNotInvokeOrchestratorForExemptCommands(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "portal version", args: []string{"version"}},
		{name: "portal state status", args: []string{"state", "status"}},
		{name: "portal init", args: []string{"init", "zsh"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resetBootstrapOnce(t)

			runner := &recordingRunner{}
			bootstrapDeps = &BootstrapDeps{Orchestrator: runner}
			t.Cleanup(func() { bootstrapDeps = nil })

			resetRootCmd()
			resetStateCmdFlags()
			// state status now renders a real diagnostic and exits
			// non-zero when the surface is unhealthy. Point the command
			// at an empty TempDir and treat ErrStatusUnhealthy as
			// expected — this test only cares whether the orchestrator
			// was invoked.
			if len(tt.args) >= 2 && tt.args[0] == "state" && tt.args[1] == "status" {
				t.Setenv("PORTAL_STATE_DIR", t.TempDir())
			}
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err != nil && err != ErrStatusUnhealthy {
				t.Fatalf("unexpected error: %v", err)
			}

			if runner.calls != 0 {
				t.Errorf("orchestrator Run call count for exempt command = %d, want 0", runner.calls)
			}
		})
	}
}

func TestPersistentPreRunE_PrefersOrchestratorOverBootstrapper(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{started: true}
	mock := &mockServerBootstrapper{}
	bootstrapDeps = &BootstrapDeps{
		Orchestrator: runner,
		Bootstrapper: mock,
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
	if mock.called {
		t.Error("Bootstrapper.EnsureServer must NOT be called when Orchestrator is set")
	}
}

func TestPersistentPreRunE_FallsBackToBootstrapperShimWhenOrchestratorNil(t *testing.T) {
	resetBootstrapOnce(t)

	mock := &mockServerBootstrapper{started: true}
	bootstrapDeps = &BootstrapDeps{Bootstrapper: mock}
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

	if !mock.called {
		t.Error("Bootstrapper.EnsureServer was not called when Orchestrator is nil")
	}
}

// Compile-time assertion: bootstrap.Runner is the type the cmd package
// stores in BootstrapDeps.Orchestrator. Catches any future drift in the
// interface signature.
var _ bootstrap.Runner = (*recordingRunner)(nil)
