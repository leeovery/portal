package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/spf13/cobra"
)

type recordingRunner struct {
	calls    int
	started  bool
	warnings []bootstrap.Warning
	err      error
}

func (r *recordingRunner) Run(_ context.Context) (bool, []bootstrap.Warning, error) {
	r.calls++
	return r.started, r.warnings, r.err
}

func TestPersistentPreRunE_OrchestratorMemoisedAcrossInvocations(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{started: true}
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner, ForceMemoise: true})

	withBackendDeps(t, BackendDeps{Backend: backendtest.New()})

	for i := range 3 {
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
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
		withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
	withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

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
		{name: "portal state", args: []string{"state"}},
		{name: "portal init", args: []string{"init", "zsh"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resetBootstrapOnce(t)

			runner := &recordingRunner{}
			withBootstrapDeps(t, BootstrapDeps{Orchestrator: runner})

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if runner.calls != 0 {
				t.Errorf("orchestrator Run call count for exempt command = %d, want 0", runner.calls)
			}
		})
	}
}

var _ bootstrap.Runner = (*recordingRunner)(nil)
