// Package xdg resolves XDG Base Directory paths for Portal. A dependency-free
// leaf, so any package may import it rather than re-implementing the resolution.
package xdg

import (
	"fmt"
	"os"
	"path/filepath"
)

// configBaseVar overrides the config base every config path resolves under.
const configBaseVar = "XDG_CONFIG_HOME"

// Variables names every environment variable a Portal path resolves from: the
// config base and each config file's and directory's override. Another process
// with the same home, set with the values this one sees, finds every file and
// directory where this one does.
func Variables() []string {
	names := []string{configBaseVar}
	for _, id := range Files() {
		names = append(names, id.EnvVar)
	}
	for _, id := range Dirs() {
		names = append(names, id.EnvVar)
	}
	return names
}

// Files is every Portal config file.
func Files() []ConfigFileID {
	return []ConfigFileID{ProjectsFile, AliasesFile, HooksFile, PrefsFile, TerminalsFile}
}

// Dirs is every Portal directory under the config base.
func Dirs() []ConfigDirID {
	return []ConfigDirID{StateDir, ThemesDir}
}

// OSEnv is the Lookup over the process's own environment — what production
// resolution reads.
func OSEnv(name string) string { return os.Getenv(name) }

// ConfigBaseFrom returns the lookup's $XDG_CONFIG_HOME verbatim when set and
// non-empty, else $HOME/.config. It errors only when neither is available. The
// home fallback reads the process's own home directory whatever the lookup is: a
// lookup carrying neither layer has nothing to say about where home is.
func ConfigBaseFrom(lookup Lookup) (string, error) {
	if base := lookup(configBaseVar); base != "" {
		return base, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	return filepath.Join(home, ".config"), nil
}
