package cmd

import (
	"fmt"
	"os"
	"path/filepath"

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

		if !store.Delete(name) {
			return fmt.Errorf("alias not found: %s", name)
		}

		if err := store.Save(); err != nil {
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

		store.Set(name, normalised)

		if err := store.Save(); err != nil {
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
	if envPath := os.Getenv("PORTAL_ALIASES_FILE"); envPath != "" {
		return envPath, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine config directory: %w", err)
	}

	return filepath.Join(configDir, "portal", "aliases"), nil
}

func init() {
	aliasCmd.AddCommand(aliasSetCmd)
	aliasCmd.AddCommand(aliasRmCmd)
	aliasCmd.AddCommand(aliasListCmd)
	rootCmd.AddCommand(aliasCmd)
}
