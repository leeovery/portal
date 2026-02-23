// Package cmd defines the CLI commands for Portal.
package cmd

import (
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// skipTmuxCheck contains command names that do not require tmux.
var skipTmuxCheck = map[string]bool{
	"version": true,
	"init":    true,
	"help":    true,
}

var rootCmd = &cobra.Command{
	Use:   "portal",
	Short: "An interactive session picker for tmux",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if skipTmuxCheck[cmd.Name()] {
			return nil
		}
		return tmux.CheckTmuxAvailable()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
