package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// configFilePath returns a config file path by checking the given environment
// variable first. If the env var is set and non-empty, its value is returned.
// Otherwise, it uses XDG-compliant resolution: XDG_CONFIG_HOME if set,
// falling back to ~/.config, then appends portal/<filename>.
func configFilePath(envVar, filename string) (string, error) {
	if envPath := os.Getenv(envVar); envPath != "" {
		return envPath, nil
	}

	configDir, err := xdgConfigBase()
	if err != nil {
		return "", fmt.Errorf("failed to determine config directory: %w", err)
	}

	return filepath.Join(configDir, "portal", filename), nil
}

// xdgConfigBase returns the XDG-compliant base config directory.
// It checks XDG_CONFIG_HOME first; if unset or empty, falls back to ~/.config.
func xdgConfigBase() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config"), nil
}
