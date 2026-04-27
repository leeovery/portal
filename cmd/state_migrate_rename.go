package cmd

import "github.com/spf13/cobra"

// stateMigrateRenameCmd migrates hooks.json keys after a tmux session rename.
// Invoked from a session-renamed hook with the old and new session names.
// Hidden from --help; body lands in a later phase.
var stateMigrateRenameCmd = &cobra.Command{
	Use:    "migrate-rename <old-name> <new-name>",
	Short:  "Migrate hook keys across a session rename (internal)",
	Args:   cobra.ExactArgs(2),
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateMigrateRenameCmd)
}
