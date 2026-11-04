package cmd

import (
	"context"
	"fmt"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/xdg"
	"github.com/spf13/cobra"
)

var uninstallDeps *UninstallDeps

// UninstallDeps overrides the saver's stop and the end of a saver Portal
// can't use; production leaves uninstallDeps nil, stops the saver over the
// session backend and ends one as the bootstrap does. A field left nil is
// production's.
type UninstallDeps struct {
	Saver  saverStopper
	Daemon unusableSaver
}

// saverStopper stops the saver, reporting whether there was one, and hands a
// stop that fails to orElse under the same hold of saver.lock.
type saverStopper interface {
	StopOr(ctx context.Context, orElse func(ctx context.Context, err error) (bool, error)) (bool, error)
}

// unusableSaver is the saver session's daemon as uninstall ends one whose zmx
// protocol Portal can't use, and so can't read to stop.
type unusableSaver interface {
	unusable(ctx context.Context) bool
	End(ctx context.Context) error
}

func uninstallSaver() (saverStopper, unusableSaver, error) {
	var deps UninstallDeps
	if uninstallDeps != nil {
		deps = *uninstallDeps
	}
	if deps.Daemon == nil {
		deps.Daemon = newSaverDaemon()
	}
	if deps.Saver != nil {
		return deps.Saver, deps.Daemon, nil
	}
	b, err := sessionBackend()
	if err != nil {
		return nil, nil, err
	}
	dir, err := state.Dir()
	if err != nil {
		return nil, nil, err
	}
	host, err := newSaverHost(b, dir)
	return host, deps.Daemon, err
}

// Uninstall stops the saver and nothing more: it keeps the bootstrapped
// latch, touches no state or config file but saver.lock, which it holds for
// the stop (making the state directory for it only when a saver runs
// without one), and starts nothing. The next Portal command, finding no
// saver, runs the full bootstrap, which brings it back.
var uninstallCmd = &cobra.Command{
	Use:           "uninstall",
	Short:         "Stop Portal's save daemon; leaves saved sessions and config",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		stopped, err := stopSaver(cmd.Context())
		if err != nil {
			return fmt.Errorf("couldn't stop Portal's save daemon: %w", err)
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), uninstallReport(stopped))
		return err
	},
}

// stopSaver stops the saver, and reports whether there was one. A saver the
// stop fails on whose daemon speaks a zmx protocol Portal can't use is ended
// as the bootstrap ends one, still under saver.lock, so no saver an ensure
// makes meanwhile is ended in its place.
func stopSaver(ctx context.Context) (bool, error) {
	stopper, daemon, err := uninstallSaver()
	if err != nil {
		return false, err
	}
	return stopper.StopOr(ctx, func(ctx context.Context, stopErr error) (bool, error) {
		return endUnusable(ctx, daemon, stopErr)
	})
}

// endUnusable ends the saver a stop failed on with stopErr when its daemon
// speaks a zmx protocol Portal can't use, and reports whether it did. One it
// can neither stop nor end fails with the bootstrap's way out of it.
func endUnusable(ctx context.Context, daemon unusableSaver, stopErr error) (bool, error) {
	if !daemon.unusable(ctx) {
		return false, stopErr
	}
	if err := daemon.End(ctx); err != nil {
		return false, fmt.Errorf("%s (%w; %w)", bootstrap.SaverLeftMessage, stopErr, err)
	}
	return true, nil
}

// uninstallReport says what uninstall did, where the saved sessions and
// config it left are, and how to remove them too.
func uninstallReport(stopped bool) string {
	done := "Portal's save daemon isn't running."
	if stopped {
		done = "Portal's save daemon stopped."
	}
	dir, err := xdg.PortalDir(xdg.OSEnv)
	if err != nil {
		return done + " Saved sessions and config are untouched.\n" +
			"To remove Portal completely, uninstall the binary and delete Portal's config directory."
	}
	return done + " Saved sessions and config are untouched at " + resolver.AbbreviateHome(resolver.ProcessHome(), dir) + "/.\n" +
		"To remove Portal completely, uninstall the binary and delete that directory."
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
