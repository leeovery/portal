// Package xdg resolves XDG Base Directory paths for Portal.
//
// It is a leaf package: it has no Portal dependencies, so any package — cmd
// or internal — may import it. This is the single source of truth for
// $XDG_CONFIG_HOME resolution; both cmd/config.go and internal/state/paths.go
// delegate here to avoid drift.
package xdg

import (
	"fmt"
	"os"
	"path/filepath"
)

// ConfigBase returns the XDG-compliant base config directory.
//
// Resolution order:
//  1. $XDG_CONFIG_HOME — used verbatim when set and non-empty.
//  2. $HOME/.config — fallback resolved via os.UserHomeDir().
//
// An error is returned only when the home directory cannot be determined
// and the env var is unset or empty.
func ConfigBase() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	return filepath.Join(home, ".config"), nil
}
