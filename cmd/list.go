package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List running tmux sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "list: not yet implemented")
		return err
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
