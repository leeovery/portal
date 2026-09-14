package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sync"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// Bootstrap must not run for these: it would heal doctor's subject, build what
// uninstall then tears down, recurse for `state` subcommands fired from tmux
// hooks, and start a server on every __complete TAB. A match anywhere in the
// parent chain exempts the invocation, keyed on cobra's canonical Name() so the
// silent `hooks` alias rides on "hook".
var skipTmuxCheck = map[string]bool{
	"__complete": true,
	"alias":      true,
	"doctor":     true,
	"help":       true,
	"hook":       true,
	"init":       true,
	"state":      true,
	"theme":      true,
	"uninstall":  true,
	"version":    true,
}

var bootstrapDeps *BootstrapDeps

// BootstrapDeps overrides bootstrap collaborators; production leaves the
// package-level bootstrapDeps nil.
type BootstrapDeps struct {
	// A nil Orchestrator short-circuits runBootstrap to a no-op result.
	Orchestrator bootstrap.Runner

	Client *tmux.Client

	// RegisterHooks is nil in production — the orchestrator owns registration.
	RegisterHooks func(*tmux.Client) error

	// ForceMemoise opts back into the production sync.Once gate, which tests
	// bypass by default so subtests can swap mocks without resetting it.
	ForceMemoise bool
}

var (
	bootstrapOnce          sync.Once
	bootstrapStarted       bool
	bootstrapWarningsSlice []bootstrap.Warning
	bootstrapErr           error
)

func buildBootstrapDeps() (bootstrap.Runner, *tmux.Client, func(*tmux.Client) error) {
	if bootstrapDeps != nil {
		return bootstrapDeps.Orchestrator, bootstrapDeps.Client, bootstrapDeps.RegisterHooks
	}
	orch, client := buildProductionOrchestrator()
	return orch, client, nil
}

func runBootstrap(ctx context.Context, runner bootstrap.Runner) (bool, []bootstrap.Warning, error) {
	if bootstrapDeps != nil && !bootstrapDeps.ForceMemoise {
		if runner == nil {
			return false, nil, nil
		}
		return runner.Run(ctx)
	}
	bootstrapOnce.Do(func() {
		if runner == nil {
			return
		}
		bootstrapStarted, bootstrapWarningsSlice, bootstrapErr = runner.Run(ctx)
	})
	return bootstrapStarted, bootstrapWarningsSlice, bootstrapErr
}

var rootCmd = &cobra.Command{
	Use:   "portal",
	Short: "An interactive session picker for tmux",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		for c := cmd; c != nil; c = c.Parent() {
			if skipTmuxCheck[c.Name()] {
				return nil
			}
		}
		if err := tmux.CheckTmuxAvailable(); err != nil {
			return bootstrap.NewFatal(err.Error(), err)
		}
		if err := runVersionCheck(); err != nil {
			return bootstrap.NewFatal(err.Error(), err)
		}

		runner, client, registerHooks := buildBootstrapDeps()

		// Read exactly once and threaded downstream; every not-satisfied case
		// (including a nil client) folds into a full bootstrap.
		latchSatisfied := client != nil && state.BootstrappedLatchSatisfied(client, version)

		// Setting no deferredBootstrapKey is load-bearing: openTUI keeps
		// serverStarted=false and reaches the picker with no loading page.
		if latchSatisfied {
			stateDir, _ := state.Dir()
			ensureSaverLiveness(client, stateDir)
			if !isTUIPath(cmd, args) {
				bootstrapWarnings.EmitTo(cmd.ErrOrStderr())
			}
			ctx := context.WithValue(cmd.Context(), serverStartedKey, false)
			ctx = context.WithValue(ctx, tmuxClientKey, client)
			cmd.SetContext(ctx)
			return nil
		}

		// openTUI runs the deferred orchestrator in a goroutine behind the loading
		// page.
		if shouldRunConcurrentBootstrap(cmd, args, client, latchSatisfied) {
			ctx := cmd.Context()
			if client != nil {
				ctx = context.WithValue(ctx, tmuxClientKey, client)
			}
			ctx = context.WithValue(ctx, deferredBootstrapKey, &deferredBootstrap{runner: runner})
			cmd.SetContext(ctx)
			// serverStartedKey is deliberately unset: openTUI reads it from the
			// progress pipe's terminal event.
			return nil
		}

		started, warnings, err := runBootstrap(cmd.Context(), runner)
		if err != nil {
			return err
		}

		// The CLI path drains here so warnings precede the command's own output;
		// the TUI path leaves them in the sink until the loading page dismisses.
		for _, w := range warnings {
			bootstrapWarnings.Add(w)
		}
		if !isTUIPath(cmd, args) {
			bootstrapWarnings.EmitTo(cmd.ErrOrStderr())
		}

		ctx := context.WithValue(cmd.Context(), serverStartedKey, started)
		if client != nil {
			ctx = context.WithValue(ctx, tmuxClientKey, client)
		}
		cmd.SetContext(ctx)

		if registerHooks != nil && client != nil {
			if err := registerHooks(client); err != nil {
				return err
			}
		}
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// isTUIPath reports whether this invocation will launch the picker, and so must
// not have warnings written to stderr — they would corrupt the alt-screen.
//
// A search form is classified on its shape rather than on its outcome: whether
// it ends at the picker depends on how many live sessions its term matches, and
// on a cold server there are none to match until restore has finished.
func isTUIPath(cmd *cobra.Command, args []string) bool {
	if cmd.Name() != "open" || anyOpenDomainPin(cmd) {
		return false
	}
	return len(args) == 0 || len(searchFormPositionals(cmd, args)) > 0
}

// A domain pin dispatches one resolved target directly, so it is not a picker
// launch. Reads openDomainPinFlags so a new pin cannot be omitted here.
func anyOpenDomainPin(cmd *cobra.Command) bool {
	return slices.ContainsFunc(openDomainPinFlags, cmd.Flags().Changed)
}

// The loading screen means "a full bootstrap is in progress", so the trigger is
// the threaded latch verdict, not a server-down probe: this decider issues no
// tmux round-trip and never re-reads the latch.
func shouldRunConcurrentBootstrap(cmd *cobra.Command, args []string, client *tmux.Client, latchSatisfied bool) bool {
	if !isTUIPath(cmd, args) || client == nil {
		return false
	}
	return !latchSatisfied
}

var fatalErrorStderr io.Writer = os.Stderr

// Execute writes a *bootstrap.FatalError's UserMessage to stderr before
// returning it, and never calls os.Exit — the caller maps the returned error to
// the process exit code.
func Execute() error {
	err := rootCmd.Execute()
	if err == nil {
		return nil
	}
	if fatal, ok := errors.AsType[*bootstrap.FatalError](err); ok {
		_, _ = fmt.Fprintln(fatalErrorStderr, fatal.UserMessage)
	}
	return err
}
