package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// migrateConfigFile moves a config file from oldPath to newPath if oldPath
// exists and newPath does not. This handles one-shot migration from the old
// macOS config location (~/Library/Application Support/portal/) to the
// XDG-compliant path. Migration is best-effort: failures log to stderr.
func migrateConfigFile(oldPath, newPath string) {
	if _, err := os.Stat(oldPath); err != nil {
		return
	}

	if _, err := os.Stat(newPath); err == nil {
		return
	} else if !os.IsNotExist(err) {
		return
	}

	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "portal: warning: failed to create config directory %s: %v\n", filepath.Dir(newPath), err)
		return
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		fmt.Fprintf(os.Stderr, "portal: warning: failed to migrate config file from %s to %s: %v\n", oldPath, newPath, err)
		return
	}

	// Clean up old directory if empty.
	_ = os.Remove(filepath.Dir(oldPath))
}

// configFilePath returns a config file path by checking the given environment
// variable first. If the env var is set and non-empty, its value is returned.
// Otherwise, it uses XDG-compliant resolution: XDG_CONFIG_HOME if set,
// falling back to ~/.config, then appends portal/<filename>.
// Before returning, it attempts a one-shot migration from the old macOS
// config path (~/Library/Application Support/portal/) if the file exists there.
func configFilePath(envVar, filename string) (string, error) {
	if envPath := os.Getenv(envVar); envPath != "" {
		return envPath, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	configDir := xdgConfigBase(homeDir)
	newPath := filepath.Join(configDir, "portal", filename)

	oldPath := filepath.Join(homeDir, "Library", "Application Support", "portal", filename)
	migrateConfigFile(oldPath, newPath)

	return newPath, nil
}

// xdgConfigBase returns the XDG-compliant base config directory.
// It checks XDG_CONFIG_HOME first; if unset or empty, falls back to
// homeDir/.config.
func xdgConfigBase(homeDir string) string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg
	}

	return filepath.Join(homeDir, ".config")
}
