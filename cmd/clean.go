package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// AllPaneLister returns the structural keys for all panes across all tmux sessions.
// Each key uses the format session_name:window_index.pane_index.
type AllPaneLister interface {
	ListAllPanes() ([]string, error)
}

// CleanDeps holds injectable dependencies for the clean command.
// When nil, real implementations are used.
type CleanDeps struct {
	AllPaneLister AllPaneLister
}

// cleanDeps allows injecting dependencies for testing.
var cleanDeps *CleanDeps

// buildCleanPaneLister returns the appropriate AllPaneLister.
// When cleanDeps is set (testing), uses the injected lister.
// Otherwise, creates a real tmux client.
func buildCleanPaneLister() AllPaneLister {
	if cleanDeps != nil {
		return cleanDeps.AllPaneLister
	}
	return tmux.DefaultClient()
}

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove stale projects whose directories no longer exist",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadProjectStore()
		if err != nil {
			return err
		}

		removed, err := store.CleanStale()
		if err != nil {
			return err
		}

		w := cmd.OutOrStdout()
		for _, p := range removed {
			if _, err := fmt.Fprintf(w, "Removed stale project: %s (%s)\n", p.Name, p.Path); err != nil {
				return err
			}
		}

		// Hook cleanup: remove entries for panes that no longer exist.
		// Acquire a non-rotating logger so the hook-cleanup tail emits the
		// same auditable log breadcrumbs as bootstrap step 11
		// (cleanStaleAdapter.CleanStale). A nil logger is tolerated — all
		// *state.Logger methods are no-ops on nil receivers (see
		// internal/state/logger.go).
		logger, _ := openNoRotateLogger()
		defer func() {
			if logger != nil {
				_ = logger.Close()
			}
		}()

		// Load hook store first to check if any hooks exist.
		hookStore, err := loadHookStore()
		if err != nil {
			return err
		}

		existingHooks, err := hookStore.Load()
		if err != nil {
			// Mirror cleanStaleAdapter.CleanStale: emit a Warn breadcrumb
			// before returning the error. portal clean propagates the
			// Load() failure to the user (unlike the bootstrap adapter
			// which surfaces it as a soft warning) — preserve that
			// pre-fix behaviour.
			logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: hookStore.Load failed: %v", err)
			return err
		}

		// No hooks registered — nothing to clean. Emit a single Debug
		// breadcrumb so every invocation of portal clean produces at least
		// one log line (preserves no-tmux-server ergonomics while keeping
		// the cleanup callsite observable in portal.log).
		if len(existingHooks) == 0 {
			logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: persisted=0, skipping")
			return nil
		}

		lister := buildCleanPaneLister()
		livePanes, err := lister.ListAllPanes()
		if err != nil {
			// Safety net — skip hook cleanup if ListAllPanes errors. The
			// return-nil (rather than return-err) preserves the pre-fix
			// silence-and-continue posture at the RunE boundary so a
			// transient tmux failure never fails the user's command. The
			// Warn breadcrumb lands in portal.log for post-hoc audit.
			logger.Warn(state.ComponentBootstrap, "stale-hook cleanup: list-panes failed: %v", err)
			return nil
		}

		// Entry-point breadcrumb — emitted exactly once after both
		// dependencies have returned successfully so the live/persisted
		// counts are observable for both normal-path and hazard-guard
		// branches.
		logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: live=%d persisted=%d", len(livePanes), len(existingHooks))

		// Mass-deletion hazard guard. A silently-empty live-pane result
		// (transient tmux failure swallowed upstream, saver pane
		// mid-respawn returning exit 0 with empty stdout, or genuine zero
		// live panes during tmux instability) must not fall through to
		// "live set empty → delete every hooks.json entry". The deferral
		// surfaces as a Warn so portal.log captures the skipped wipe. The
		// persisted == 0 early-exit above guarantees existingHooks > 0
		// here — no second len==0 && len==0 branch is needed.
		if len(livePanes) == 0 {
			logger.Warn(state.ComponentBootstrap,
				"stale-hook cleanup: zero live panes parsed with %d hook(s) present; skipping to avoid mass-deletion hazard (next bootstrap retries)",
				len(existingHooks))
			return nil
		}

		removedPanes, err := hookStore.CleanStale(livePanes)
		if err != nil {
			return err
		}
		logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: removed=%d", len(removedPanes))

		for _, paneID := range removedPanes {
			if _, err := fmt.Fprintf(w, "Removed stale hook: %s\n", paneID); err != nil {
				return err
			}
		}

		return nil
	},
}

// loadProjectStore creates a project store from the configured file path.
// Uses PORTAL_PROJECTS_FILE env var if set (for testing), otherwise
// defaults to ~/.config/portal/projects.json.
func loadProjectStore() (*project.Store, error) {
	path, err := projectsFilePath()
	if err != nil {
		return nil, err
	}
	return project.NewStore(path), nil
}

// projectsFilePath returns the path to the projects.json file.
// Uses PORTAL_PROJECTS_FILE env var if set (for testing), otherwise
// defaults to ~/.config/portal/projects.json.
func projectsFilePath() (string, error) {
	return configFilePath("PORTAL_PROJECTS_FILE", "projects.json")
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}
