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
		// (cleanStaleAdapter.CleanStale, which composes the same shared
		// helper). A nil logger is tolerated — runHookStaleCleanup
		// substitutes a no-op when nil is passed.
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

		// Early-exit Load — drives the persisted==0 short-circuit which
		// keeps the no-tmux-server ergonomics intact (the panickingPaneLister
		// integration subtest pins this: the lister MUST NOT be invoked
		// when persisted==0). The shared helper performs its own Load
		// after this branch returns; accepting the duplicate ReadFile is
		// intentional (see option (a) in the parent plan task) — both
		// Loads observe the same on-disk content and the helper stays
		// self-contained.
		//
		// On Load failure here we fall through to runHookStaleCleanup
		// rather than emit our own Warn — the helper performs its own
		// Load, reproduces the same failure deterministically, and emits
		// the canonical "stale-hook cleanup: hookStore.Load failed" Warn
		// at its single declaration site. This keeps the format string
		// declared exactly once across package cmd (acceptance criterion
		// 1 of the parent plan task). The trade-off is a redundant
		// ListAllPanes call on the Load-failure path — acceptable because
		// (a) the helper's swallow policy means ListAllPanes never fails
		// the user's command, and (b) Load failures are rare (corrupt or
		// permission-denied hooks.json).
		//
		// loadErr is captured (NOT discarded with `_`) so the persisted==0
		// early-exit only fires when Load actually succeeded with an empty
		// map. Without the err-gate, a (nil, EACCES) return from Load
		// satisfies len(nil) == 0 and silently short-circuits past the
		// helper — re-introducing the "silent at the adapter" defect class
		// the parent spec set out to eliminate (acceptance criterion 4).
		existingHooks, loadErr := hookStore.Load()

		// No hooks registered — nothing to clean. Emit a single Debug
		// breadcrumb so every invocation of portal clean produces at least
		// one log line (preserves no-tmux-server ergonomics while keeping
		// the cleanup callsite observable in portal.log). This breadcrumb
		// stays at the callsite — the shared helper does NOT emit it.
		// Gated on loadErr == nil so Load failures fall through to the
		// helper for canonical-Warn emission rather than silently exiting.
		if loadErr == nil && len(existingHooks) == 0 {
			logger.Debug(state.ComponentBootstrap, "stale-hook cleanup: persisted=0, skipping")
			return nil
		}

		// Delegate the six-branch algorithm to the shared helper.
		// swallowListError=true so a transient ListAllPanes failure never
		// fails the user's command (the Warn lands in portal.log for audit).
		// onRemoved prints "Removed stale hook: <key>" per removed entry,
		// preserving the pre-extraction user-facing stdout byte-for-byte.
		//
		// Return value is deliberately discarded: with swallowListError=true
		// the helper already returns nil for ListAllPanes errors. The
		// remaining return paths are (a) nil on the happy path and (b) a
		// hookStore.Load / CleanStale error on the destructive branches.
		// Per spec §Logger plumbing / portal clean: "the subcommand's
		// RunE continues to return nil for the hook-cleanup tail's
		// transient failures (matching the existing pre-fix safety-net
		// posture which already chose silence-and-continue over
		// user-facing error)". The helper has already emitted the
		// canonical Warn breadcrumb to portal.log before returning, so
		// the failure is post-hoc auditable.
		_ = runHookStaleCleanup(
			buildCleanPaneLister(),
			hookStore,
			logger,
			true,
			func(paneID string) {
				_, _ = fmt.Fprintf(w, "Removed stale hook: %s\n", paneID)
			},
		)
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
