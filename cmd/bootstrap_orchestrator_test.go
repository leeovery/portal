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
	warnings []bootstrap.Warning
	err      error
}

func (r *recordingRunner) Run(_ context.Context) ([]bootstrap.Warning, error) {
	r.calls++
	return r.warnings, r.err
}

func TestPersistentPreRunE_OrchestratorMemoisedAcrossInvocations(t *testing.T) {
	resetBootstrapOnce(t)

	runner := &recordingRunner{}
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

func TestPersistentPreRunE_RunsTheBootstrapTheGateOwes(t *testing.T) {
	for _, owed := range []bool{true, false} {
		name := map[bool]string{true: "owed", false: "not owed"}[owed]
		t.Run(name, func(t *testing.T) {
			resetBootstrapOnce(t)

			runner := &recordingRunner{}
			asked := 0
			withBootstrapDeps(t, BootstrapDeps{
				Owed:         func(context.Context) bool { asked++; return owed },
				Orchestrator: runner,
			})

			ran := false
			probe := &cobra.Command{
				Use: "owedprobe",
				RunE: func(*cobra.Command, []string) error {
					ran = true
					return nil
				},
			}
			rootCmd.AddCommand(probe)
			t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

			resetRootCmd()
			rootCmd.SetArgs([]string{"owedprobe"})
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := map[bool]int{true: 1, false: 0}[owed]
			if asked != 1 || runner.calls != want || !ran {
				t.Errorf("gate asked %d times, bootstrap run %d times, command ran %v; want the gate asked once, the bootstrap run %d times, then the command", asked, runner.calls, ran, want)
			}
		})
	}
}

func TestPersistentPreRunE_OrchestratorErrorPropagatesAndCommandDoesNotRun(t *testing.T) {
	resetBootstrapOnce(t)

	sentinel := errors.New("orchestrator boom")
	runner := &recordingRunner{err: sentinel}
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

func TestPersistentPreRunE_DoesNotAskTheGateForExemptCommands(t *testing.T) {
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
			asked := false
			withBootstrapDeps(t, BootstrapDeps{
				Owed:         func(context.Context) bool { asked = true; return true },
				Orchestrator: runner,
			})

			resetRootCmd()
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if asked || runner.calls != 0 {
				t.Errorf("gate asked %v, orchestrator Run call count = %d for an exempt command; want neither", asked, runner.calls)
			}
		})
	}
}

var _ bootstrap.Runner = (*recordingRunner)(nil)
