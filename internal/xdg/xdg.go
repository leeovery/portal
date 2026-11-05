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

// Absolute is path, read from the environment, made absolute against the
// working directory of the process reading it, which is where its user gave
// it. A process Portal starts runs elsewhere (the saver's daemon in the state
// directory, a session in its own), where a relative path names another file.
// An absolute path is kept as written, and an empty one, read as unset, stays
// empty.
func Absolute(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

// fromEnv is lookup's value of name, made Absolute.
func fromEnv(lookup Lookup, name string) (string, error) {
	path, err := Absolute(lookup(name))
	if err != nil {
		return "", fmt.Errorf("resolve %s against the working directory: %w", name, err)
	}
	return path, nil
}

// AbsoluteEnv sets each of Variables that this process holds a relative path
// in to that path made Absolute, so every process Portal starts inherits the
// one meaning the path has here. A Portal process runs it before it resolves
// anything.
func AbsoluteEnv() error {
	for _, name := range Variables() {
		path, err := fromEnv(OSEnv, name)
		if err != nil {
			return err
		}
		if path != os.Getenv(name) {
			if err := os.Setenv(name, path); err != nil {
				return fmt.Errorf("set %s: %w", name, err)
			}
		}
	}
	return nil
}

// ConfigBaseFrom returns the lookup's $XDG_CONFIG_HOME, made Absolute, when
// set and non-empty, else $HOME/.config. It errors only when neither is
// available, or the working directory a relative base needs can't be read.
// The home fallback reads the process's own home directory whatever the
// lookup is: a lookup carrying neither layer has nothing to say about where
// home is.
func ConfigBaseFrom(lookup Lookup) (string, error) {
	if base, err := fromEnv(lookup, configBaseVar); base != "" || err != nil {
		return base, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	return filepath.Join(home, ".config"), nil
}
