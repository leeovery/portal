package cmd

import (
	"fmt"

	"github.com/leeovery/portal/internal/alias"
	"github.com/leeovery/portal/internal/resolver"
	"github.com/spf13/cobra"
)

var aliasCmd = &cobra.Command{
	Use:   "alias",
	Short: "Manage path aliases",
}

var aliasRmCmd = &cobra.Command{
	Use:   "rm [name]",
	Short: "Remove a path alias",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		store, err := loadAliasStore()
		if err != nil {
			return err
		}

		// DeleteAndSave is the audited mutation path: it emits the rm breadcrumb
		// under the aliases component (via=cli) on a successful persist. An
		// absent alias returns existed=false WITHOUT persisting or emitting, so
		// the pre-instrumentation "alias not found" error path is preserved
		// exactly.
		existed, err := store.DeleteAndSave(name, "cli")
		if !existed {
			return fmt.Errorf("alias not found: %s", name)
		}
		if err != nil {
			return fmt.Errorf("failed to save aliases: %w", err)
		}

		return nil
	},
}

var aliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all path aliases",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := loadAliasStore()
		if err != nil {
			return err
		}

		for _, a := range store.List() {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", a.Name, a.Path); err != nil {
				return err
			}
		}

		return nil
	},
}

var aliasSetCmd = &cobra.Command{
	Use:   "set [name] [path]",
	Short: "Set a path alias",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		normalised := resolver.NormalisePath(args[1])

		store, err := loadAliasStore()
		if err != nil {
			return err
		}

		// SetAndSave is the audited mutation path: it classifies set / modify /
		// set-noop from the pre-mutation map, persists (skipping the write on a
		// no-op), and emits the breadcrumb under the aliases component (via=cli).
		if err := store.SetAndSave(name, normalised, "cli"); err != nil {
			return fmt.Errorf("failed to save aliases: %w", err)
		}

		return nil
	},
}

// loadAliasStore creates and loads an alias store from the configured file path.
func loadAliasStore() (*alias.Store, error) {
	aliasFile, err := aliasFilePath()
	if err != nil {
		return nil, err
	}

	store := alias.NewStore(aliasFile)
	if _, err := store.Load(); err != nil {
		return nil, fmt.Errorf("failed to load aliases: %w", err)
	}

	return store, nil
}

// aliasFilePath returns the path to the aliases file.
// Uses PORTAL_ALIASES_FILE env var if set (for testing), otherwise
// defaults to ~/.config/portal/aliases.
func aliasFilePath() (string, error) {
	return configFilePath("PORTAL_ALIASES_FILE", "aliases")
}

func init() {
	aliasCmd.AddCommand(aliasSetCmd)
	aliasCmd.AddCommand(aliasRmCmd)
	aliasCmd.AddCommand(aliasListCmd)
	rootCmd.AddCommand(aliasCmd)
}
