// Package cmd defines the CLI commands for Portal.
package cmd

import (
	"context"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// skipTmuxCheck contains command names that do not require tmux.
// If any command in the parent chain matches, the tmux check is skipped.
var skipTmuxCheck = map[string]bool{
	"version": true,
	"init":    true,
	"help":    true,
	"alias":   true,
	"clean":   true,
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
type BootstrapDeps struct {
	Bootstrapper ServerBootstrapper
	Client       *tmux.Client
	Waiter       func()
}

// buildBootstrapDeps returns the appropriate server bootstrapper and shared client.
// When bootstrapDeps is set (testing), uses injected dependencies.
// Otherwise, builds a real tmux client with RealCommander.
func buildBootstrapDeps() (ServerBootstrapper, *tmux.Client) {
	if bootstrapDeps != nil {
		return bootstrapDeps.Bootstrapper, bootstrapDeps.Client
	}
	client := tmux.NewClient(&tmux.RealCommander{})
	return client, client
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
		bootstrapper, client := buildBootstrapDeps()
		serverStarted, err := bootstrapper.EnsureServer()
		if err != nil {
			return err
		}
		ctx := context.WithValue(cmd.Context(), serverStartedKey, serverStarted)
		if client != nil {
			ctx = context.WithValue(ctx, tmuxClientKey, client)
		}
		cmd.SetContext(ctx)
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
