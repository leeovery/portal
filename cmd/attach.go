package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/spawn"
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
	// AckWriter writes the @portal-spawn-<batch>-<token> confirmation marker
	// for a spawned window (the --spawn-ack carrier). See the spawn-ack
	// contract in RunE below.
	AckWriter spawn.AckWriter
}

var attachCmd = &cobra.Command{
	Use:   "attach [name]",
	Short: "Attach to a tmux session by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		connector, validator, ackWriter := buildAttachDeps(cmd)

		// --spawn-ack puts attach in spawn-ack mode: after confirming the
		// session exists, write the picker's @portal-spawn-<batch>-<token>
		// marker as the last action before the exec handoff into tmux. Validate
		// the flag value fast, before touching tmux — a malformed value is a
		// usage error (exit 2).
		ackVal, _ := cmd.Flags().GetString("spawn-ack")
		var ackBatch, ackToken string
		ackRequested := ackVal != ""
		if ackRequested {
			batch, token, ok := spawn.ParseSpawnAckFlag(ackVal)
			if !ok {
				return NewUsageError("attach: --spawn-ack must be <batch>:<token>")
			}
			ackBatch, ackToken = batch, token
		}

		if !validator.HasSession(name) {
			return fmt.Errorf("No session found: %s", name) //nolint:staticcheck // user-facing message per spec
		}

		if ackRequested {
			// Best-effort and strictly after the session-exists check: the write
			// is the last action before the exec handoff. A failed write just
			// means the picker times out and classifies this window failed —
			// safe — so do NOT return; fall through to Connect.
			if err := ackWriter.Write(ackBatch, ackToken); err != nil {
				spawnLogger.Debug("spawn-ack marker write failed",
					"session", name,
					"batch", ackBatch,
					"detail", err.Error(),
				)
			}
		}

		return connector.Connect(name)
	},
}

// buildAttachDeps returns the connector, validator, and ack writer for the
// attach command. When attachDeps is set (testing), uses injected dependencies.
// Otherwise, builds real implementations based on inside/outside tmux detection;
// the ack writer is a @portal-spawn- server-option channel over the shared tmux
// client (which satisfies both the writer and lister seams).
func buildAttachDeps(cmd *cobra.Command) (SessionConnector, SessionValidator, spawn.AckWriter) {
	if attachDeps != nil {
		return attachDeps.Connector, attachDeps.Validator, attachDeps.AckWriter
	}

	client := tmuxClient(cmd)
	connector := buildSessionConnector(client)
	ackWriter := spawn.NewServerOptionAckChannel(client, client)
	return connector, client, ackWriter
}

func init() {
	attachCmd.Flags().String("spawn-ack", "", "internal: write the @portal-spawn-<batch>:<token> ack marker before attaching")
	rootCmd.AddCommand(attachCmd)
}
