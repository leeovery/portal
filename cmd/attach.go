package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach [name]",
	Short: "Attach to a tmux session by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "attach: not yet implemented")
		return err
	},
}

func init() {
	rootCmd.AddCommand(attachCmd)
}
