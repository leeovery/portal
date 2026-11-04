package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/xdg"
)

const (
	sessionsJSONName  = "sessions.json"
	saveRequestedName = "save.requested"
	daemonPIDName     = "daemon.pid"
	daemonVersionName = "daemon.version"
	daemonLockName    = "daemon.lock"
	portalLogName     = "portal.log"
	portalLogOldName  = "portal.log.old"
	scrollbackSubdir  = "scrollback"
)

// Dir resolves the absolute path to Portal's state directory: $PORTAL_STATE_DIR
// verbatim when set, otherwise <XDG config base>/portal/state.
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

func SessionsJSON(dir string) string { return filepath.Join(dir, sessionsJSONName) }

// SaveRequested is the dirty-flag file `portal state notify` touches.
func SaveRequested(dir string) string { return filepath.Join(dir, saveRequestedName) }

// TouchSaveRequested's mtime bump is best-effort and its error swallowed: the
// daemon's tick only checks the file's presence.
func TouchSaveRequested(dir string) error {
	path := SaveRequested(dir)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("touch save.requested: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("touch save.requested: %w", err)
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
	return nil
}

func DaemonPID(dir string) string { return filepath.Join(dir, daemonPIDName) }

func DaemonVersion(dir string) string { return filepath.Join(dir, daemonVersionName) }

func DaemonLock(dir string) string { return filepath.Join(dir, daemonLockName) }

func PortalLog(dir string) string { return filepath.Join(dir, portalLogName) }

func PortalLogOld(dir string) string { return filepath.Join(dir, portalLogOldName) }

func ScrollbackDir(dir string) string { return filepath.Join(dir, scrollbackSubdir) }

func ScrollbackFile(dir, paneKey string) string {
	return filepath.Join(dir, scrollbackSubdir, paneKey+".bin")
}

func FIFOPath(dir, paneKey string) string {
	return filepath.Join(dir, "hydrate-"+paneKey+".fifo")
}

// PaneKeyFromFIFOPath accepts a full FIFO path or a bare basename.
func PaneKeyFromFIFOPath(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, ".fifo")
	name = strings.TrimPrefix(name, "hydrate-")
	return name
}
