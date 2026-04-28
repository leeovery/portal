package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// waiterFunc is the package-level injection seam for the post-bootstrap
// session-population wait. Production uses tmux.WaitForSessions with the
// shared client; tests substitute a no-op or recorder. Replaces the
// pre-Phase-5 BootstrapDeps.Waiter field, which was removed when
// PersistentPreRunE moved to the bootstrap.Runner seam.
//
// TODO(phase-5/4): remove alongside bootstrap_wait.go itself once
// WaitForSessions is deleted; see spec "WaitForSessions / bootstrapWait
// Removal".
var waiterFunc = defaultWaiter

// defaultWaiter is the production wait implementation: it pulls the
// shared tmux client out of the command context and runs
// WaitForSessions with the package's default config.
func defaultWaiter(cmd *cobra.Command) {
	client := tmuxClient(cmd)
	cfg := tmux.DefaultWaitConfig(client)
	tmux.WaitForSessions(cfg)
}

// bootstrapWait checks if the server was just started and, if so, prints a
// status message to stderr and waits for sessions to appear. The wait
// behavior is provided by the package-level waiterFunc seam (production
// default: defaultWaiter).
func bootstrapWait(cmd *cobra.Command) {
	if !serverWasStarted(cmd) {
		return
	}

	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Starting tmux server...")

	waiterFunc(cmd)
}
