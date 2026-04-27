package cmd

import "github.com/spf13/cobra"

// stateStatusCmd surfaces a human-readable diagnostic snapshot of Portal's
// resurrection machinery. The body is implemented in a later phase; this
// scaffold accepts the argv shape and exits 0.
var stateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Portal save daemon status and recent diagnostics",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateStatusCmd)
}
