package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// bootstrapWait checks if the server was just started and, if so, prints a
// status message to stderr and waits for sessions to appear. The wait
// behavior is injected via bootstrapDeps.Waiter; when bootstrapDeps is nil
// (production), the default tmux.WaitForSessions with DefaultWaitConfig is used.
func bootstrapWait(cmd *cobra.Command) {
	if !serverWasStarted(cmd) {
		return
	}

	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Starting tmux server...")

	buildWaiter(cmd)()
}

// buildWaiter returns the appropriate wait function.
// When bootstrapDeps is set (testing), uses the injected Waiter.
// Otherwise, builds a real waiter using the tmux client from context.
func buildWaiter(cmd *cobra.Command) func() {
	if bootstrapDeps != nil && bootstrapDeps.Waiter != nil {
		return bootstrapDeps.Waiter
	}
	return func() {
		client := tmuxClient(cmd)
		cfg := tmux.DefaultWaitConfig(client)
		tmux.WaitForSessions(cfg)
	}
}
