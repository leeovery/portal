// Package cmd defines the CLI commands for Portal.
package cmd

import (
	"context"
	"sync"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// skipTmuxCheck contains command names that do not require tmux.
// If any command in the parent chain matches, the tmux check is skipped.
//
// Per the resurrection spec, the exempt set is:
//   - alias / clean / help / init / version: user-facing config or help
//   - state: every `portal state ...` subcommand. User-facing children
//     (status, cleanup) inspect or tear down machinery the bootstrap sets
//     up — running bootstrap first would be circular. Internal children
//     (daemon, notify, signal-hydrate, hydrate, migrate-rename) are invoked
//     by tmux hooks or as the pane's initial process; re-running bootstrap
//     would recursively register hooks and could spawn nested daemons.
//
// Note: 'hooks' is intentionally NOT in this map. Phase 4 moved hook
// firing into the hydrate helper, so `portal hooks set/list/rm` now go
// through full bootstrap to keep CleanStale and skeleton restoration in
// the path where the user expects it.
var skipTmuxCheck = map[string]bool{
	"alias":   true,
	"clean":   true,
	"help":    true,
	"init":    true,
	"state":   true,
	"version": true,
}

// ServerBootstrapper ensures a tmux server is running. Retained as the
// legacy injection seam for cmd-package tests written before the full
// orchestrator landed; production code goes through bootstrap.Runner.
type ServerBootstrapper interface {
	EnsureServer() (bool, error)
}

// bootstrapDeps holds injectable dependencies for PersistentPreRunE. When
// nil, real implementations are used.
var bootstrapDeps *BootstrapDeps

// BootstrapDeps allows injecting bootstrap dependencies for testing.
//
// Orchestrator is the preferred seam — when set, PersistentPreRunE calls
// its Run method directly. Bootstrapper is the legacy seam wrapped by
// bootstrap.NewShim when only Bootstrapper is set; it is scheduled for
// removal in Phase 6 once every cmd-package test migrates to the
// Orchestrator field. Client populates the tmuxClientKey context value
// (helpers like tmuxClient(cmd) panic without it). RegisterHooks is the
// seam for Portal's global tmux hook registration; when nil (production
// default), tmux.RegisterPortalHooks is used.
type BootstrapDeps struct {
	// Orchestrator is the primary test seam — implementations of
	// bootstrap.Runner whose Run is invoked by PersistentPreRunE.
	Orchestrator bootstrap.Runner

	// Bootstrapper is the legacy test seam, wrapped via bootstrap.NewShim
	// when Orchestrator is nil. TODO(phase-6): delete after every
	// cmd-package test migrates to Orchestrator.
	Bootstrapper ServerBootstrapper

	// Client is exposed in cmd.Context() under tmuxClientKey so downstream
	// commands (list, attach, kill, …) can look it up.
	Client *tmux.Client

	// RegisterHooks is invoked after the orchestrator returns to register
	// Portal's global tmux hook table on the live client. Production
	// default is tmux.RegisterPortalHooks.
	RegisterHooks func(*tmux.Client) error

	// ForceMemoise opts the test into the production sync.Once gate. By
	// default tests bypass the gate so subtests that swap mocks do not
	// have to reset shared package state between Execute() calls. Only
	// the dedicated memoisation test sets this to true.
	ForceMemoise bool
}

// bootstrapOnce, bootstrapStarted, and bootstrapErr memoise the
// orchestrator call so PersistentPreRunE invokes Run exactly once per
// process. Tests reset the gate via resetBootstrapOnce(t). The pattern
// mirrors versionCheckOnce in cmd/version_guard.go.
var (
	bootstrapOnce    sync.Once
	bootstrapStarted bool
	bootstrapErr     error
)

// buildBootstrapDeps returns the runner, shared client, and hook
// registration function used by PersistentPreRunE. When bootstrapDeps is
// set (test mode), uses injected dependencies — preferring Orchestrator
// over the legacy Bootstrapper shim. Otherwise builds a real tmux client
// with RealCommander and wraps it via bootstrap.NewShim. Production
// keeps using NewShim until a follow-up adapter task wires the full
// Orchestrator with all step implementations.
func buildBootstrapDeps() (bootstrap.Runner, *tmux.Client, func(*tmux.Client) error) {
	if bootstrapDeps != nil {
		var runner bootstrap.Runner
		if bootstrapDeps.Orchestrator != nil {
			runner = bootstrapDeps.Orchestrator
		} else if bootstrapDeps.Bootstrapper != nil {
			runner = bootstrap.NewShim(bootstrapDeps.Bootstrapper) //nolint:staticcheck // shim is the legacy seam during Phase 5 cutover
		}
		return runner, bootstrapDeps.Client, bootstrapDeps.RegisterHooks
	}
	client := tmux.NewClient(&tmux.RealCommander{})
	return bootstrap.NewShim(client), client, tmux.RegisterPortalHooks //nolint:staticcheck // production wraps the client in shim until follow-up adapter task lands
}

// runBootstrap invokes the runner with per-process memoisation. In
// production (bootstrapDeps == nil) the sync.Once gate guarantees Run is
// called exactly once. In test mode the gate is bypassed by default so
// tests that rebuild bootstrapDeps between subtests do not need to reset
// shared state — set BootstrapDeps.ForceMemoise to opt back into the
// gate when verifying memoisation behaviour itself.
func runBootstrap(ctx context.Context, runner bootstrap.Runner) (bool, error) {
	if bootstrapDeps != nil && !bootstrapDeps.ForceMemoise {
		if runner == nil {
			return false, nil
		}
		return runner.Run(ctx)
	}
	bootstrapOnce.Do(func() {
		if runner == nil {
			return
		}
		bootstrapStarted, bootstrapErr = runner.Run(ctx)
	})
	return bootstrapStarted, bootstrapErr
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
			return err
		}
		if err := runVersionCheck(); err != nil {
			return err
		}

		runner, client, registerHooks := buildBootstrapDeps()
		started, err := runBootstrap(cmd.Context(), runner)
		if err != nil {
			return err
		}

		ctx := context.WithValue(cmd.Context(), serverStartedKey, started)
		if client != nil {
			ctx = context.WithValue(ctx, tmuxClientKey, client)
		}
		cmd.SetContext(ctx)

		// Hook registration sits outside the orchestrator's path because
		// production has not yet wired the full Orchestrator (planned
		// follow-up task). The shim Runner only does EnsureServer; we
		// still need RegisterHooks to keep Phase 1's hook table in scope.
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

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
