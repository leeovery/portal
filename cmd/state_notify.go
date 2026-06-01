package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/state"
	"github.com/spf13/cobra"
)

// stateNotifyCmd is the minimal save-trigger notifier invoked by tmux hooks.
// Hidden from --help; invoked internally.
//
// Behavior is intentionally tiny: ensure the state directory exists, then
// touch save.requested (creating or truncating it to zero bytes) and bump its
// mtime. The hosted daemon polls this dirty flag on its 1-second tick and
// performs the actual capture. notify itself performs zero tmux calls and
// reads no other state files — it must remain trivially fast and side-effect
// minimal because tmux invokes it from hook contexts on every structural
// event.
//
// Diagnostics route through portal.log under the notify component. An
// EnsureDir failure returns the wrapped error before any diagnostics are
// emitted — the only surviving stderr path is the cobra error printer, the
// documented "fatal pre-state-dir" exception.
var stateNotifyCmd = &cobra.Command{
	Use:    "notify",
	Short:  "Bump the save-requested marker (internal, invoked by tmux hooks)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := state.EnsureDir()
		if err != nil {
			// Fatal pre-logger: without a state dir we have nowhere to open
			// portal.log. cobra prints the wrapped error to stderr.
			return fmt.Errorf("ensure state dir: %w", err)
		}

		// Diagnostics (save.requested create/truncate failures) land in the
		// central log under the notify component via the handler configured
		// once by main -> log.Init. Rotation is handler-owned (Phase 2), so
		// notify no longer opens or closes a per-process logger.
		if err := state.TouchSaveRequested(dir); err != nil {
			notifyLogger.Warn("touch save.requested failed", "path", state.SaveRequested(dir), "error", err)
			return fmt.Errorf("notify: %w", err)
		}
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateNotifyCmd)
}
