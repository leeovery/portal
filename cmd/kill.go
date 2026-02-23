package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill [name]",
	Short: "Kill a tmux session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "kill: not yet implemented")
		return err
	},
}

func init() {
	rootCmd.AddCommand(killCmd)
}
