package cmd

import "github.com/spf13/cobra"

// stateHydrateCmd is the per-pane initial command at skeleton restore time.
// All three flags are required (see CLI Surface in the resurrection spec).
// Hidden from --help; body lands in a later phase.
var stateHydrateCmd = &cobra.Command{
	Use:    "hydrate",
	Short:  "Hydrate a restored pane from saved scrollback (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	stateHydrateCmd.Flags().String("fifo", "", "Absolute path to the per-pane FIFO")
	stateHydrateCmd.Flags().String("file", "", "Absolute path to the saved scrollback file")
	stateHydrateCmd.Flags().String("hook-key", "", "Saved structural identifier (<session>:<window>.<pane>)")
	_ = stateHydrateCmd.MarkFlagRequired("fifo")
	_ = stateHydrateCmd.MarkFlagRequired("file")
	_ = stateHydrateCmd.MarkFlagRequired("hook-key")

	stateCmd.AddCommand(stateHydrateCmd)
}
