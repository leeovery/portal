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
// Diagnostics route through portal.log under state.ComponentNotify. The
// logger is opened only after EnsureDir succeeds (the log file lives inside
// the state directory); an EnsureDir failure therefore returns the wrapped
// error without a log line — the only surviving stderr path is the cobra
// error printer, which is the documented "fatal pre-logger" exception.
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

		// Open portal.log via the non-daemon append-only path so notify
		// failures (save.requested create/truncate failures) land in the
		// central log under state.ComponentNotify. Per spec § Log Rotation
		// → Concurrent-writer discipline, only the daemon rotates. On open
		// failure logger is nil and the *state.Logger nil-receiver no-ops
		// every call so notify never aborts on a diagnostics-only failure.
		logger, _ := openNoRotateLogger()
		defer func() { _ = logger.Close() }()

		if err := state.TouchSaveRequested(dir); err != nil {
			logger.Warn(state.ComponentNotify, "touch save.requested at %s: %v", state.SaveRequested(dir), err)
			return fmt.Errorf("touch save.requested: %w", err)
		}
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateNotifyCmd)
}
