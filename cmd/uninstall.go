package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// UninstallDeps allows injecting test dependencies for the uninstall command.
// When nil, real implementations are used: a tmux.Client built on
// RealCommander, tmux.UnregisterPortalHooks for hook removal, and the daemon
// component's *slog.Logger for portal.log entries. When non-nil, Client must
// be supplied; Unregister is optional and falls back to
// tmux.UnregisterPortalHooks; Logger is optional (a nil Logger falls back to
// the daemon component's logger).
type UninstallDeps struct {
	Client     *tmux.Client
	Unregister func(*tmux.Client) error
	Logger     *slog.Logger
}

// uninstallDeps is the package-level injection point for tests. Production
// code path leaves it nil and uses real dependencies.
var uninstallDeps *UninstallDeps

// buildUninstallDeps returns the tmux client, hook-removal function, and logger
// the uninstall body should use. When uninstallDeps is set (testing), uses the
// injected dependencies, defaulting Unregister to tmux.UnregisterPortalHooks
// and Logger to the daemon component's logger. Otherwise builds a real tmux
// client, uses tmux.UnregisterPortalHooks, and logs under the daemon component
// via the handler configured once by main -> log.Init.
//
// No new log component is introduced: the taxonomy is closed, so uninstall
// reuses the daemon component's logger for its saver-kill breadcrumb (the same
// forensic surface the removed `state cleanup` used).
func buildUninstallDeps() (*tmux.Client, func(*tmux.Client) error, *slog.Logger) {
	if uninstallDeps != nil {
		unregister := uninstallDeps.Unregister
		if unregister == nil {
			unregister = tmux.UnregisterPortalHooks
		}
		logger := uninstallDeps.Logger
		if logger == nil {
			logger = daemonLogger
		}
		return uninstallDeps.Client, unregister, logger
	}
	return tmux.DefaultClient(), tmux.UnregisterPortalHooks, daemonLogger
}

// uninstallCompletionLine1 / uninstallCompletionLine2 are the byte-exact
// two-line completion message printed on every uninstall path (spec §
// uninstall — Runtime-Only Teardown). The message always appears — even on a
// partial-failure return — because uninstall never irreversibly destroys
// anything and the printed path is how the user learns what remains and how to
// remove it completely.
const uninstallCompletionLine1 = "Portal's tmux runtime removed. Your saved sessions and config are untouched at ~/.config/portal/."
const uninstallCompletionLine2 = "To remove Portal completely, uninstall the binary and delete that directory."

// uninstallCmd removes ONLY Portal's tmux-server footprint — the part that is
// hard to do by hand — and touches no files at all.
//
// Spec ("uninstall — Runtime-Only Teardown") defines the teardown as two
// actions on a running server, in a load-bearing order:
//  1. kill-session -t _portal-saver — terminates the daemon. tmux closes the
//     session's PTY, the kernel delivers SIGHUP, and the daemon's signal
//     handler performs a final atomic flush before exiting.
//  2. Remove Portal's global hook entries via index-based set-hook -gu.
//
// killSaver runs BEFORE UnregisterPortalHooks so the daemon's final SIGHUP
// flush observes the pre-teardown world (hooks still registered,
// _portal-saver still alive at flush start). Partial failures never
// short-circuit — both actions run and errors accumulate via errors.Join.
// When the tmux server is not running, both actions are skipped (no-op
// preconditions) and the command is a graceful no-op. The completion message
// is printed on every path.
//
// The command touches NO state-dir or config files — nothing irreversible
// happens, and `portal open` re-bootstraps the daemon + hooks on the next run.
// It also leaves every session running, including the load-bearing
// _portal-bootstrap anchor.
var uninstallCmd = &cobra.Command{
	Use:           "uninstall",
	Short:         "Remove Portal's tmux runtime (save daemon + global hooks); leaves saved sessions and config",
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, unregister, logger := buildUninstallDeps()

		var errs []error

		// No tmux server = no _portal-saver and no global hooks. Skip both
		// actions; the command is still a graceful no-op that prints the
		// completion message below.
		if client.ServerRunning() {
			if err := killSaver(client, logger); err != nil {
				errs = append(errs, fmt.Errorf("daemon kill: %w", err))
			}
			if err := unregister(client); err != nil {
				errs = append(errs, fmt.Errorf("hook removal: %w", err))
			}
		}

		// The message must always appear — even on a partial-failure return —
		// so print BEFORE returning the joined error.
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), uninstallCompletionLine1)
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), uninstallCompletionLine2)

		return errors.Join(errs...)
	},
}

// killSaverInfoMessage is the INFO/ComponentDaemon log line emitted for a
// successful (or idempotent-success) saver kill. Centralised so the two
// success paths in killSaver cannot drift.
const killSaverInfoMessage = "killed _portal-saver; daemon will flush final state on SIGHUP"

// killSaver kills the _portal-saver session, delivering SIGHUP to the daemon
// for a final atomic flush before exit. Idempotent across two failure modes:
//  1. _portal-saver absent at probe time (HasSession returns false) — no
//     KillSession call, returns nil.
//  2. _portal-saver auto-destroyed between probe and kill (KillSession returns
//     a "can't find session" error) — treated as success since the desired
//     state is "session gone."
//
// Other KillSession errors (e.g. permission denied, server error) are logged
// at WARN/ComponentDaemon and returned wrapped so RunE can accumulate them.
// Successful kills emit an INFO/ComponentDaemon line that names the SIGHUP
// flush behaviour for operator forensics.
func killSaver(c *tmux.Client, logger *slog.Logger) error {
	if !c.HasSession(tmux.PortalSaverName) {
		return nil
	}
	if err := c.KillSession(tmux.PortalSaverName); err != nil {
		if isSessionAbsentError(err) {
			logger.Info(killSaverInfoMessage)
			return nil
		}
		logger.Warn("kill _portal-saver failed", "error", err)
		return err
	}
	logger.Info(killSaverInfoMessage)
	return nil
}

// isSessionAbsentError reports whether err is tmux's "can't find session"
// shape — emitted both by has-session probes and by kill-session when the
// session has auto-destroyed. The substring is stable across tmux 3.0+; the
// case-insensitive match shields against future capitalisation changes.
func isSessionAbsentError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "can't find session")
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
