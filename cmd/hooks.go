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

// ServerOptionDeleter deletes a tmux server-level option.
type ServerOptionDeleter interface {
	DeleteServerOption(name string) error
}

// hooksDeps holds injectable dependencies for the hooks commands.
// When nil, real implementations are used.
var hooksDeps *HooksDeps

// HooksDeps allows injecting dependencies for testing.
type HooksDeps struct {
	OptionSetter  ServerOptionSetter
	OptionDeleter ServerOptionDeleter
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

// buildHooksDeleteDeps returns the appropriate ServerOptionDeleter.
// When hooksDeps is set (testing), uses the injected OptionDeleter.
// Otherwise, creates a real tmux client.
func buildHooksDeleteDeps() ServerOptionDeleter {
	if hooksDeps != nil {
		return hooksDeps.OptionDeleter
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

var hooksRmCmd = &cobra.Command{
	Use:   "rm",
	Short: "Remove a resume hook for the current pane",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paneID := os.Getenv("TMUX_PANE")
		if paneID == "" {
			return fmt.Errorf("must be run from inside a tmux pane")
		}

		store, err := loadHookStore()
		if err != nil {
			return err
		}

		if err := store.Remove(paneID, "on-resume"); err != nil {
			return err
		}

		deleter := buildHooksDeleteDeps()
		if err := deleter.DeleteServerOption("@portal-active-" + paneID); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	hooksSetCmd.Flags().String("on-resume", "", "Command to run when resuming the pane")
	_ = hooksSetCmd.MarkFlagRequired("on-resume")

	hooksRmCmd.Flags().Bool("on-resume", false, "Remove the on-resume hook")
	_ = hooksRmCmd.MarkFlagRequired("on-resume")

	hooksCmd.AddCommand(hooksListCmd)
	hooksCmd.AddCommand(hooksSetCmd)
	hooksCmd.AddCommand(hooksRmCmd)
	rootCmd.AddCommand(hooksCmd)
}
