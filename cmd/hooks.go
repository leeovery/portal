package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/spf13/cobra"
)

var hooksCmd = &cobra.Command{
	Use:   "hooks",
	Short: "Manage resume hooks",
}

var hooksListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered hooks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadHookStore()
		if err != nil {
			return err
		}

		list, err := store.List()
		if err != nil {
			return err
		}

		for _, h := range list {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", h.PaneID, h.Event, h.Command); err != nil {
				return err
			}
		}

		return nil
	},
}

// loadHookStore creates a hook store from the configured file path.
func loadHookStore() (*hooks.Store, error) {
	path, err := hooksFilePath()
	if err != nil {
		return nil, err
	}

	return hooks.NewStore(path), nil
}

// hooksFilePath returns the path to the hooks JSON file.
// Uses PORTAL_HOOKS_FILE env var if set (for testing), otherwise
// defaults to ~/.config/portal/hooks.json.
func hooksFilePath() (string, error) {
	return configFilePath("PORTAL_HOOKS_FILE", "hooks.json")
}

func init() {
	hooksCmd.AddCommand(hooksListCmd)
	rootCmd.AddCommand(hooksCmd)
}
