package cmd

import "github.com/spf13/cobra"

// stateCleanupCmd performs explicit teardown of Portal's resurrection state.
// Phase 1 wires only the argv surface; the cleanup body lands in a later task.
var stateCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Tear down Portal's save daemon, hooks, and (optionally) state directory",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	stateCleanupCmd.Flags().Bool("purge", false, "Also remove ~/.config/portal/state/ on cleanup")
	stateCmd.AddCommand(stateCleanupCmd)
}
