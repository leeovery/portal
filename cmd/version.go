package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at build time via ldflags:
//
//	go build -ldflags "-X github.com/leeovery/portal/cmd.version=1.2.3"
var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Portal version",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "portal version %s\n", version)
		return err
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
