package state

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/leeovery/portal/internal/xdg"
)

const (
	daemonPIDName     = "daemon.pid"
	daemonVersionName = "daemon.version"
	daemonLockName    = "daemon.lock"
	portalLogName     = "portal.log"
	scrollbackSubdir  = "scrollback"
	tmuxStateName     = "state-tmux"
	stagingName       = "state-migrating"
)

// Dir resolves the absolute path to Portal's state directory: $PORTAL_STATE_DIR
// when set, made absolute against the working directory, otherwise
// <XDG config base>/portal/state.
func Dir() (string, error) {
	return xdg.ConfigDirPath(xdg.OSEnv, xdg.StateDir)
}

// EnsureDir creates the state directory and its scrollback subdirectory with
// mode 0700 and returns the state directory path. Existing directories are
// left in place.
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

func DaemonPID(dir string) string { return filepath.Join(dir, daemonPIDName) }

func DaemonVersion(dir string) string { return filepath.Join(dir, daemonVersionName) }

func DaemonLock(dir string) string { return filepath.Join(dir, daemonLockName) }

func PortalLog(dir string) string { return filepath.Join(dir, portalLogName) }

func ScrollbackDir(dir string) string { return filepath.Join(dir, scrollbackSubdir) }

// TmuxStateDir is where the migration from the tmux version moves that
// version's state: state-tmux, beside the state directory dir. It exists only
// once that migration has run. dir is cleaned first: filepath.Dir of a path
// ending in a separator is that path itself.
func TmuxStateDir(dir string) string {
	return sibling(dir, tmuxStateName)
}

// MigrationStagingDir is where the migration from the tmux version builds the
// zmx version's state before swapping it in: state-migrating, beside the
// state directory dir. It exists only while a migration is under way, or
// after one cut short.
func MigrationStagingDir(dir string) string {
	return sibling(dir, stagingName)
}

// sibling is name beside the directory dir. dir is cleaned first:
// filepath.Dir of a path ending in a separator is that path itself.
func sibling(dir, name string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(dir)), name)
}
