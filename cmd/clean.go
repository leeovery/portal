package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove stale projects whose directories no longer exist",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "clean: not yet implemented")
		return err
	},
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}
