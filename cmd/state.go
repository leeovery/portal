package cmd

import "github.com/spf13/cobra"

// stateCmd is the parent command for Portal session resurrection state.
// It has no Run/RunE so Cobra prints help when invoked bare.
var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Manage Portal session resurrection state",
}

func init() {
	rootCmd.AddCommand(stateCmd)
}
