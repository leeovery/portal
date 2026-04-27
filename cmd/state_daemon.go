package cmd

import "github.com/spf13/cobra"

// stateDaemonCmd is the long-running save daemon hosted in the
// _portal-saver tmux session. Hidden from --help; invoked internally.
// The implementation lands in later phases.
var stateDaemonCmd = &cobra.Command{
	Use:    "daemon",
	Short:  "Run the Portal save daemon (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateDaemonCmd)
}
