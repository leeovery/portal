// Package cmd defines the CLI commands for Portal.
package cmd

import (
	"context"

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

// ServerBootstrapper ensures a tmux server is running.
type ServerBootstrapper interface {
	EnsureServer() (bool, error)
}

// BootstrapDeps holds injectable dependencies for the bootstrap step.
// When nil, real implementations are used.
var bootstrapDeps *BootstrapDeps

// BootstrapDeps allows injecting the server bootstrapper for testing.
// When Client is non-nil it is stored in context; otherwise no client is set.
// RegisterHooks is the seam for Portal's global tmux hook registration; when
// nil (production default), tmux.RegisterPortalHooks is used.
type BootstrapDeps struct {
	Bootstrapper  ServerBootstrapper
	Client        *tmux.Client
	Waiter        func()
	RegisterHooks func(*tmux.Client) error
}

// buildBootstrapDeps returns the appropriate server bootstrapper, shared
// client, and hook-registration function. When bootstrapDeps is set
// (testing), uses injected dependencies. Otherwise, builds a real tmux
// client with RealCommander and uses tmux.RegisterPortalHooks.
func buildBootstrapDeps() (ServerBootstrapper, *tmux.Client, func(*tmux.Client) error) {
	if bootstrapDeps != nil {
		return bootstrapDeps.Bootstrapper, bootstrapDeps.Client, bootstrapDeps.RegisterHooks
	}
	client := tmux.NewClient(&tmux.RealCommander{})
	return client, client, tmux.RegisterPortalHooks
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
		bootstrapper, client, registerHooks := buildBootstrapDeps()
		serverStarted, err := bootstrapper.EnsureServer()
		if err != nil {
			return err
		}
		ctx := context.WithValue(cmd.Context(), serverStartedKey, serverStarted)
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

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
