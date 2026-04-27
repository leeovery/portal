package cmd

import (
	"fmt"
	"os"
	"time"

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
var stateNotifyCmd = &cobra.Command{
	Use:    "notify",
	Short:  "Bump the save-requested marker (internal, invoked by tmux hooks)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := state.EnsureDir()
		if err != nil {
			return fmt.Errorf("ensure state dir: %w", err)
		}

		path := state.SaveRequested(dir)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("touch save.requested: %w", err)
		}
		_ = f.Close()

		now := time.Now()
		_ = os.Chtimes(path, now, now)
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateNotifyCmd)
}
