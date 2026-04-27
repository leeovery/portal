package state

import (
	"fmt"
	"os"
	"path/filepath"
)

// Filenames inside the state directory. Kept private — callers should compose
// paths via the accessor functions below so the layout stays in one place.
const (
	sessionsJSONName  = "sessions.json"
	saveRequestedName = "save.requested"
	daemonPIDName     = "daemon.pid"
	daemonVersionName = "daemon.version"
	portalLogName     = "portal.log"
	portalLogOldName  = "portal.log.old"
	scrollbackSubdir  = "scrollback"
)

// Dir resolves the absolute path to Portal's state directory.
//
// Resolution order:
//  1. $PORTAL_STATE_DIR — used verbatim, no suffix appended.
//  2. $XDG_CONFIG_HOME/portal/state — when XDG_CONFIG_HOME is set.
//  3. $HOME/.config/portal/state — fallback.
//
// Unlike the cmd-package config helpers, Dir does not perform any one-shot
// migration from legacy macOS paths: the state directory is new in this
// feature and has no prior location to migrate from.
func Dir() (string, error) {
	if envPath := os.Getenv("PORTAL_STATE_DIR"); envPath != "" {
		return envPath, nil
	}

	base, err := xdgConfigBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "portal", "state"), nil
}

// EnsureDir resolves and creates the state directory and its scrollback
// subdirectory with mode 0700, returning the resolved state directory path.
// It is idempotent: existing directories are left in place.
func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create state directory %s: %w", dir, err)
	}

	scrollback := filepath.Join(dir, scrollbackSubdir)
	if err := os.MkdirAll(scrollback, 0o700); err != nil {
		return "", fmt.Errorf("failed to create scrollback directory %s: %w", scrollback, err)
	}

	return dir, nil
}

// SessionsJSON returns the path to the structural index file.
func SessionsJSON(dir string) string { return filepath.Join(dir, sessionsJSONName) }

// SaveRequested returns the path to the dirty-flag file touched by
// `portal state notify`.
func SaveRequested(dir string) string { return filepath.Join(dir, saveRequestedName) }

// DaemonPID returns the path to the daemon's PID file.
func DaemonPID(dir string) string { return filepath.Join(dir, daemonPIDName) }

// DaemonVersion returns the path to the daemon's version-marker file.
func DaemonVersion(dir string) string { return filepath.Join(dir, daemonVersionName) }

// PortalLog returns the path to the current portal log file.
func PortalLog(dir string) string { return filepath.Join(dir, portalLogName) }

// PortalLogOld returns the path to the previous (rotated) portal log file.
func PortalLogOld(dir string) string { return filepath.Join(dir, portalLogOldName) }

// ScrollbackDir returns the path to the directory holding per-pane
// scrollback `.bin` files.
func ScrollbackDir(dir string) string { return filepath.Join(dir, scrollbackSubdir) }

// ScrollbackFile returns the path to the scrollback `.bin` file for the
// given canonical paneKey.
func ScrollbackFile(dir, paneKey string) string {
	return filepath.Join(dir, scrollbackSubdir, paneKey+".bin")
}

// FIFOPath returns the hydration FIFO path for the given canonical paneKey.
func FIFOPath(dir, paneKey string) string {
	return filepath.Join(dir, "hydrate-"+paneKey+".fifo")
}

// xdgConfigBase mirrors cmd.xdgConfigBase: it returns $XDG_CONFIG_HOME if set
// and non-empty, otherwise $HOME/.config. Duplicated here because the cmd
// package cannot be imported by internal packages.
func xdgConfigBase() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}
