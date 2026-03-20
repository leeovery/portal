package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// killDeps holds injectable dependencies for the kill command.
// When nil, real implementations are used.
var killDeps *KillDeps

// SessionKiller kills a tmux session by name.
type SessionKiller interface {
	KillSession(name string) error
}

// KillDeps allows injecting dependencies for testing.
type KillDeps struct {
	Killer    SessionKiller
	Validator SessionValidator
}

var killCmd = &cobra.Command{
	Use:   "kill [name]",
	Short: "Kill a tmux session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bootstrapWait(cmd, nil)

		name := args[0]

		killer, validator := buildKillDeps(cmd)

		if !validator.HasSession(name) {
			return fmt.Errorf("No session found: %s", name) //nolint:staticcheck // user-facing message per spec
		}

		return killer.KillSession(name)
	},
}

// buildKillDeps returns the appropriate killer and validator for the kill command.
// When killDeps is set (testing), uses injected dependencies.
// Otherwise, builds real implementations.
func buildKillDeps(cmd *cobra.Command) (SessionKiller, SessionValidator) {
	if killDeps != nil {
		return killDeps.Killer, killDeps.Validator
	}

	client := tmuxClient(cmd)
	return client, client
}

func init() {
	rootCmd.AddCommand(killCmd)
}
