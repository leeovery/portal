package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

// errUninstallUnavailable refuses uninstall until it can stop the zmx saver.
// The tmux teardown it replaced would reach whatever tmux server TMUX names,
// so a build run beside the installed Portal would kill that Portal's saver
// and unregister its hooks.
var errUninstallUnavailable = errors.New("portal uninstall returns with the zmx saver")

var uninstallCmd = &cobra.Command{
	Use:           "uninstall",
	Short:         "Remove Portal's save daemon; leaves saved sessions and config",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(*cobra.Command, []string) error {
		return errUninstallUnavailable
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
