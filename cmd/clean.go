package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// AllPaneLister returns the pane IDs for all panes across all tmux sessions.
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
	return tmux.NewClient(&tmux.RealCommander{})
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
		// Load hook store first to check if any hooks exist.
		hookStore, err := loadHookStore()
		if err != nil {
			return err
		}

		existingHooks, err := hookStore.Load()
		if err != nil {
			return err
		}

		// No hooks registered — nothing to clean.
		if len(existingHooks) == 0 {
			return nil
		}

		lister := buildCleanPaneLister()
		livePanes, err := lister.ListAllPanes()
		if err != nil {
			// Safety net — skip hook cleanup if ListAllPanes errors.
			return nil
		}

		// Empty pane list with existing hooks means no tmux server is running.
		// Skip cleanup to avoid destroying hooks needed after next reboot.
		if len(livePanes) == 0 {
			return nil
		}

		removedPanes, err := hookStore.CleanStale(livePanes)
		if err != nil {
			return err
		}

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
