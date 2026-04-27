package cmd

import "github.com/spf13/cobra"

// stateNotifyCmd is the minimal save-trigger notifier invoked by tmux hooks.
// Hidden from --help; invoked internally. The body lands in a later phase.
var stateNotifyCmd = &cobra.Command{
	Use:    "notify",
	Short:  "Bump the save-requested marker (internal, invoked by tmux hooks)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateNotifyCmd)
}
