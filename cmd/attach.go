package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// attachDeps holds injectable dependencies for the attach command.
// When nil, real implementations are used.
var attachDeps *AttachDeps

// SessionValidator checks whether a tmux session exists by name.
type SessionValidator interface {
	HasSession(name string) bool
}

// AttachDeps allows injecting dependencies for testing.
type AttachDeps struct {
	Connector SessionConnector
	Validator SessionValidator
}

var attachCmd = &cobra.Command{
	Use:   "attach [name]",
	Short: "Attach to a tmux session by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bootstrapWait(cmd)

		name := args[0]

		connector, validator := buildAttachDeps(cmd)

		if !validator.HasSession(name) {
			return fmt.Errorf("No session found: %s", name) //nolint:staticcheck // user-facing message per spec
		}

		return connector.Connect(name)
	},
}

// buildAttachDeps returns the appropriate connector and validator for the attach command.
// When attachDeps is set (testing), uses injected dependencies.
// Otherwise, builds real implementations based on inside/outside tmux detection.
func buildAttachDeps(cmd *cobra.Command) (SessionConnector, SessionValidator) {
	if attachDeps != nil {
		return attachDeps.Connector, attachDeps.Validator
	}

	client := tmuxClient(cmd)
	connector := buildSessionConnector(client)
	return connector, client
}

func init() {
	rootCmd.AddCommand(attachCmd)
}
