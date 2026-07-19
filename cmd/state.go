package cmd

import "github.com/spf13/cobra"

// stateCmd is the parent command for Portal session resurrection state.
// It has no Run/RunE so Cobra prints help when invoked bare.
//
// Hidden marks the entire subtree as invocable plumbing: `state` and all six
// children drop out of `portal --help` and generated shell completions in one
// move, yet stay fully argv-invocable (Hidden is a visibility flag only — it
// does NOT disable resolution or execution). The daemon, hydrate helpers, and
// reboot hook-firing depend on that invocability. The `state` prefix and every
// child name are preserved verbatim because the tmux idempotency substring
// matchers and PortalDaemonArgvPattern match the literal `state …` strings.
var stateCmd = &cobra.Command{
	Use:    "state",
	Short:  "Manage Portal session resurrection state",
	Hidden: true,
}

func init() {
	rootCmd.AddCommand(stateCmd)
}
