package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// StateCleanupDeps allows injecting test dependencies for the state cleanup
// command. When nil, real implementations are used: a tmux.Client built on
// RealCommander, tmux.UnregisterPortalHooks for hook removal, and a best-effort
// non-rotating *state.Logger for portal.log entries. When non-nil, Client must
// be supplied; Unregister is optional and falls back to
// tmux.UnregisterPortalHooks; Logger is optional (a nil *state.Logger is a
// valid no-op).
type StateCleanupDeps struct {
	Client     *tmux.Client
	Unregister func(*tmux.Client) error
	Logger     *state.Logger
}

// stateCleanupDeps is the package-level injection point for tests. Production
// code path leaves it nil and uses real dependencies.
var stateCleanupDeps *StateCleanupDeps

// buildStateCleanupDeps returns the tmux client, hook-removal function, and
// logger the cleanup body should use. When stateCleanupDeps is set (testing),
// uses the injected dependencies, defaulting Unregister to
// tmux.UnregisterPortalHooks. Otherwise builds a real tmux client, uses
// tmux.UnregisterPortalHooks, and opens portal.log via the non-rotating
// helper. A logger open failure degrades to nil (which the *state.Logger
// nil-receiver treats as a no-op) so cleanup never aborts on a diagnostics-
// only failure.
func buildStateCleanupDeps() (*tmux.Client, func(*tmux.Client) error, *state.Logger) {
	if stateCleanupDeps != nil {
		unregister := stateCleanupDeps.Unregister
		if unregister == nil {
			unregister = tmux.UnregisterPortalHooks
		}
		return stateCleanupDeps.Client, unregister, stateCleanupDeps.Logger
	}
	client := tmux.NewClient(&tmux.RealCommander{})
	logger, _ := openNoRotateLogger()
	return client, tmux.UnregisterPortalHooks, logger
}

// stateCleanupCmd performs explicit teardown of Portal's resurrection state.
//
// Spec ("CLI Surface → portal state cleanup") defines three actions in order:
//  1. kill-session -t _portal-saver — terminates the daemon. tmux closes the
//     session's PTY, the kernel delivers SIGHUP, and the daemon's signal
//     handler performs a final atomic flush before exiting (Phase 2).
//  2. Remove Portal's global hook entries via index-based set-hook -gu.
//  3. Optionally remove ~/.config/portal/state/ when --purge is passed.
//
// Ordering matters: the daemon's final flush must observe the pre-cleanup
// world (hooks still registered, _portal-saver still alive at flush start).
// killSaver therefore runs BEFORE UnregisterPortalHooks; purge runs LAST so
// the daemon's final flush has somewhere to write. Partial failures never
// short-circuit — every action runs and errors accumulate via errors.Join
// so cleanup never leaves mixed state. When the tmux server is not running,
// killSaver and UnregisterPortalHooks are skipped (no-op preconditions); the
// purge step still runs because the state directory is independent of tmux.
var stateCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Tear down Portal's save daemon, hooks, and (optionally) state directory",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		purge, _ := cmd.Flags().GetBool("purge")

		client, unregister, logger := buildStateCleanupDeps()

		var errs []error

		// No tmux server = no _portal-saver and no global hooks. Skip those
		// actions but still honour --purge: the state directory lives on disk
		// independent of tmux server state.
		if client.ServerRunning() {
			if err := killSaver(client, logger); err != nil {
				errs = append(errs, fmt.Errorf("daemon kill: %w", err))
			}
			if err := unregister(client); err != nil {
				errs = append(errs, fmt.Errorf("hook removal: %w", err))
			}
		}
		if purge {
			if err := runPurge(logger); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		return nil
	},
}

// runPurge resolves the state directory and removes it via purgeStateDir,
// wrapping any error with a "purge state dir" prefix so the joined error
// message in RunE identifies the failing action.
func runPurge(logger *state.Logger) error {
	dir, err := state.Dir()
	if err != nil {
		return fmt.Errorf("purge state dir: %w", err)
	}
	if err := purgeStateDir(dir, logger); err != nil {
		return fmt.Errorf("purge state dir: %w", err)
	}
	return nil
}

// purgeStateDir removes dir and all contents when --purge is supplied. It is
// idempotent on a missing directory and refuses to follow symlinks: if dir is
// itself a symlink, or its filepath.EvalSymlinks-resolved path differs from
// the cleaned input path, the function returns an error rather than removing
// content the operator did not intend to expose to RemoveAll.
//
// Successful purges and RemoveAll failures are logged at INFO and ERROR
// respectively under ComponentDaemon. The logger may be nil; *state.Logger's
// nil-receiver semantics make those calls safe.
func purgeStateDir(dir string, logger *state.Logger) error {
	info, err := os.Lstat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("lstat %s: %w", dir, err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to purge symlinked state dir: %s", dir)
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return fmt.Errorf("eval symlinks %s: %w", dir, err)
	}
	cleanDir := filepath.Clean(dir)
	if filepath.Clean(resolved) != cleanDir {
		return fmt.Errorf("state dir %s resolves to %s — refusing to purge", dir, resolved)
	}

	if err := os.RemoveAll(dir); err != nil {
		logger.Error(state.ComponentDaemon, "purge failed: %v", err)
		return fmt.Errorf("remove all: %w", err)
	}
	logger.Info(state.ComponentDaemon, "purged state directory %s", dir)
	return nil
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
func killSaver(c *tmux.Client, logger *state.Logger) error {
	if !c.HasSession(tmux.PortalSaverName) {
		return nil
	}
	if err := c.KillSession(tmux.PortalSaverName); err != nil {
		if isSessionAbsentError(err) {
			logger.Info(state.ComponentDaemon, killSaverInfoMessage)
			return nil
		}
		logger.Warn(state.ComponentDaemon, "kill _portal-saver failed: %v", err)
		return err
	}
	logger.Info(state.ComponentDaemon, killSaverInfoMessage)
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
	stateCleanupCmd.Flags().Bool("purge", false, "Also remove ~/.config/portal/state/ on cleanup")
	stateCmd.AddCommand(stateCleanupCmd)
}
