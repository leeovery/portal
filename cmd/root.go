// Package cmd defines the CLI commands for Portal.
package cmd

import (
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
type BootstrapDeps struct {
	Bootstrapper ServerBootstrapper
}

// buildBootstrapDeps returns the appropriate server bootstrapper.
// When bootstrapDeps is set (testing), uses injected dependency.
// Otherwise, builds a real tmux client with RealCommander.
func buildBootstrapDeps() ServerBootstrapper {
	if bootstrapDeps != nil {
		return bootstrapDeps.Bootstrapper
	}
	return tmux.NewClient(&tmux.RealCommander{})
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
		bootstrapper := buildBootstrapDeps()
		_, err := bootstrapper.EnsureServer()
		return err
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
