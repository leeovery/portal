package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// bootstrapWait checks if the server was just started and, if so, prints a
// status message to stderr and waits for sessions to appear. The waiter
// parameter is injectable for testing; when nil, the default
// tmux.WaitForSessions with DefaultWaitConfig is used.
func bootstrapWait(cmd *cobra.Command, waiter func()) {
	if !serverWasStarted(cmd) {
		return
	}

	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Starting tmux server...")

	if waiter == nil {
		client := tmuxClient(cmd)
		cfg := tmux.DefaultWaitConfig(client)
		tmux.WaitForSessions(cfg)
		return
	}

	waiter()
}
