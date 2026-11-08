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
