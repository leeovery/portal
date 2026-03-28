package cmd

import (
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// ServerOptionSetter sets a tmux server-level option.
type ServerOptionSetter interface {
	SetServerOption(name, value string) error
}

// hooksDeps holds injectable dependencies for the hooks set command.
// When nil, real implementations are used.
var hooksDeps *HooksDeps

// HooksDeps allows injecting the ServerOptionSetter for testing.
type HooksDeps struct {
	OptionSetter ServerOptionSetter
}

// buildHooksDeps returns the appropriate ServerOptionSetter.
// When hooksDeps is set (testing), uses the injected OptionSetter.
// Otherwise, creates a real tmux client.
func buildHooksDeps() ServerOptionSetter {
	if hooksDeps != nil {
		return hooksDeps.OptionSetter
	}
	return tmux.NewClient(&tmux.RealCommander{})
}

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

var hooksSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Register a resume hook for the current pane",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paneID := os.Getenv("TMUX_PANE")
		if paneID == "" {
			return fmt.Errorf("must be run from inside a tmux pane")
		}

		command, err := cmd.Flags().GetString("on-resume")
		if err != nil {
			return err
		}

		store, err := loadHookStore()
		if err != nil {
			return err
		}

		if err := store.Set(paneID, "on-resume", command); err != nil {
			return err
		}

		setter := buildHooksDeps()
		if err := setter.SetServerOption("@portal-active-"+paneID, "1"); err != nil {
			return err
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
	hooksSetCmd.Flags().String("on-resume", "", "Command to run when resuming the pane")
	_ = hooksSetCmd.MarkFlagRequired("on-resume")

	hooksCmd.AddCommand(hooksListCmd)
	hooksCmd.AddCommand(hooksSetCmd)
	rootCmd.AddCommand(hooksCmd)
}
