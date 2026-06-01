package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at build time via ldflags:
//
//	go build -ldflags "-X github.com/leeovery/portal/cmd.version=1.2.3"
var version = "dev"

// Version exposes the build-time version string to package main, which needs
// it for log.Init before Cobra runs. The variable itself stays unexported so
// the ldflags target (cmd.version) and the rest of the package are unchanged.
func Version() string { return version }

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
