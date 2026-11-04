package cmd

import (
	"context"
	"fmt"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/xdg"
	"github.com/spf13/cobra"
)

var uninstallDeps *UninstallDeps

// UninstallDeps overrides the saver's stop; production leaves uninstallDeps
// nil and stops the saver over the session backend.
type UninstallDeps struct {
	Saver saverStopper
}

// saverStopper stops the saver, reporting whether one was running.
type saverStopper interface {
	Stop(ctx context.Context) (bool, error)
}

func uninstallStopper() (saverStopper, error) {
	if uninstallDeps != nil {
		return uninstallDeps.Saver, nil
	}
	b, err := sessionBackend()
	if err != nil {
		return nil, err
	}
	return newSaverStopper(b)
}

// Uninstall stops the saver and nothing more: it keeps the bootstrapped
// latch, so the next Portal command, finding it current, brings the saver
// back, and it touches no state or config file and starts nothing.
var uninstallCmd = &cobra.Command{
	Use:           "uninstall",
	Short:         "Stop Portal's save daemon; leaves saved sessions and config",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		stopper, err := uninstallStopper()
		if err != nil {
			return err
		}
		stopped, err := stopper.Stop(cmd.Context())
		if err != nil {
			return fmt.Errorf("couldn't stop Portal's save daemon: %w", err)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), uninstallReport(stopped))
		return err
	},
}

// uninstallReport says what uninstall did, and where the saved sessions and
// config it left are.
func uninstallReport(stopped bool) string {
	done := "Portal's save daemon isn't running."
	if stopped {
		done = "Portal's save daemon stopped."
	}
	dir, err := xdg.PortalDir(xdg.OSEnv)
	if err != nil {
		return done + " Saved sessions and config are untouched."
	}
	return done + " Saved sessions and config are untouched at " + resolver.AbbreviateHome(resolver.ProcessHome(), dir) + "/."
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
