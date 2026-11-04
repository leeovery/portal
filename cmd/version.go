package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/devversion"
	"github.com/spf13/cobra"
)

// version is set at build time via ldflags; renaming it breaks the release
// build's -X target.
var version = "dev"

// Version is the release version a release build injects, or a dev build's
// own, which changes with every rebuilt binary.
func Version() string {
	if version == "dev" {
		return devversion.String()
	}
	return version
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Portal version",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "portal version %s\n", Version())
		return err
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
