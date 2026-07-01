package cmd

import (
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

// HookKeyResolver resolves a tmux pane ID (e.g. "%3") to its hook key
// (e.g. "<@portal-id or session_name>:window.pane") via HookKeyFormat — a
// stamped session resolves off the immutable @portal-id (rename-immune), an
// un-stamped session off the session name.
type HookKeyResolver interface {
	ResolveHookKey(paneID string) (string, error)
}

// Compile-time assertion that the production tmux client satisfies the seam,
// so a drift in ResolveHookKey's signature fails fast at build time rather
// than only via the implicit assignment in resolveCurrentPaneKey.
var _ HookKeyResolver = (*tmux.Client)(nil)

// hooksDeps holds injectable dependencies for the hooks commands.
// When nil, real implementations are used.
var hooksDeps *HooksDeps

// HooksDeps allows injecting dependencies for testing.
type HooksDeps struct {
	KeyResolver HookKeyResolver
}

// requireTmuxPane reads TMUX_PANE from the environment and returns an
// error if empty. This is the single validation point for both set and rm.
func requireTmuxPane() (string, error) {
	paneID := os.Getenv("TMUX_PANE")
	if paneID == "" {
		return "", fmt.Errorf("must be run from inside a tmux pane")
	}
	return paneID, nil
}

// buildHooksTmuxClient creates a real tmux.Client for hooks commands.
// Only called when hooksDeps is nil (production path).
func buildHooksTmuxClient() *tmux.Client {
	return tmux.DefaultClient()
}

// resolveCurrentPaneKey reads TMUX_PANE from the environment, resolves
// it to a hook key (e.g. "<@portal-id or session_name>:window.pane") via the
// injected or default HookKeyResolver, and returns the result.
func resolveCurrentPaneKey() (string, error) {
	paneID, err := requireTmuxPane()
	if err != nil {
		return "", err
	}

	var keyResolver HookKeyResolver
	if hooksDeps != nil && hooksDeps.KeyResolver != nil {
		keyResolver = hooksDeps.KeyResolver
	} else {
		keyResolver = buildHooksTmuxClient()
	}

	hookKey, err := keyResolver.ResolveHookKey(paneID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve hook key for current pane: %w", err)
	}

	return hookKey, nil
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
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", h.Key, h.Event, h.Command); err != nil {
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
		hookKey, err := resolveCurrentPaneKey()
		if err != nil {
			return err
		}

		command, err := cmd.Flags().GetString("on-resume")
		if err != nil {
			return err
		}

		store, err := loadHookStore()
		if err != nil {
			return err
		}

		return store.Set(hookKey, "on-resume", command, "cli")
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
		paneKey, err := cmd.Flags().GetString("pane-key")
		if err != nil {
			return err
		}

		var hookKey string
		if paneKey != "" {
			hookKey = paneKey
		} else {
			hookKey, err = resolveCurrentPaneKey()
			if err != nil {
				return err
			}
		}

		store, err := loadHookStore()
		if err != nil {
			return err
		}

		return store.Remove(hookKey, "on-resume", "cli")
	},
}

func init() {
	hooksSetCmd.Flags().String("on-resume", "", "Command to run when resuming the pane")
	_ = hooksSetCmd.MarkFlagRequired("on-resume")

	hooksRmCmd.Flags().Bool("on-resume", false, "Remove the on-resume hook")
	_ = hooksRmCmd.MarkFlagRequired("on-resume")
	hooksRmCmd.Flags().String("pane-key", "", "Structural key of the pane whose hook should be removed (defaults to the current pane)")

	hooksCmd.AddCommand(hooksListCmd)
	hooksCmd.AddCommand(hooksSetCmd)
	hooksCmd.AddCommand(hooksRmCmd)
	rootCmd.AddCommand(hooksCmd)
}
