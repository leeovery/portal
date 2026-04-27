package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// StateCleanupDeps allows injecting test dependencies for the state cleanup
// command. When nil, real implementations are used: a tmux.Client built on
// RealCommander and tmux.UnregisterPortalHooks. When non-nil, Client must be
// supplied; Unregister is optional and falls back to tmux.UnregisterPortalHooks.
type StateCleanupDeps struct {
	Client     *tmux.Client
	Unregister func(*tmux.Client) error
}

// stateCleanupDeps is the package-level injection point for tests. Production
// code path leaves it nil and uses real dependencies.
var stateCleanupDeps *StateCleanupDeps

// buildStateCleanupDeps returns the tmux client and hook-removal function the
// cleanup body should use. When stateCleanupDeps is set (testing), uses the
// injected dependencies, defaulting Unregister to tmux.UnregisterPortalHooks.
// Otherwise builds a real tmux client and uses tmux.UnregisterPortalHooks.
func buildStateCleanupDeps() (*tmux.Client, func(*tmux.Client) error) {
	if stateCleanupDeps != nil {
		unregister := stateCleanupDeps.Unregister
		if unregister == nil {
			unregister = tmux.UnregisterPortalHooks
		}
		return stateCleanupDeps.Client, unregister
	}
	client := tmux.NewClient(&tmux.RealCommander{})
	return client, tmux.UnregisterPortalHooks
}

// stateCleanupCmd performs explicit teardown of Portal's resurrection state.
// Phase 1 covers only the global hook removal step. The saver-kill (phase 2)
// and --purge state-directory removal (phase 6) land in later tasks.
var stateCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Tear down Portal's save daemon, hooks, and (optionally) state directory",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, unregister := buildStateCleanupDeps()

		// No tmux server = nothing to clean. Exit 0.
		if !client.ServerRunning() {
			return nil
		}

		err := unregister(client)
		// TODO(phase-2): kill-session -t _portal-saver
		// TODO(phase-6): if --purge, remove ~/.config/portal/state/
		if err != nil {
			return fmt.Errorf("hook removal: %w", err)
		}
		return nil
	},
}

func init() {
	stateCleanupCmd.Flags().Bool("purge", false, "Also remove ~/.config/portal/state/ on cleanup")
	stateCmd.AddCommand(stateCleanupCmd)
}
