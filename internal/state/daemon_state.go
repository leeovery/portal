package state

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/leeovery/portal/internal/fileutil"
)

// ErrPIDFileAbsent is distinct from an I/O or parse failure reading daemon.pid.
var ErrPIDFileAbsent = errors.New("daemon.pid absent")

// ErrVersionFileAbsent is distinct from an I/O failure reading daemon.version.
var ErrVersionFileAbsent = errors.New("daemon.version absent")

// WritePIDFile uses plain AtomicWrite deliberately — the pid is non-sensitive,
// so the umask defence applied to sessions.json and scrollback is unwanted here.
func WritePIDFile(dir string, pid int) error {
	content := strconv.Itoa(pid) + "\n"
	return fileutil.AtomicWrite(DaemonPID(dir), []byte(content))
}

// ReadPIDFile gives ErrPIDFileAbsent for a missing file; every other failure is
// wrapped, with a zero pid.
func ReadPIDFile(dir string) (int, error) {
	data, err := readDaemonFile(DaemonPID(dir), ErrPIDFileAbsent)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse daemon.pid: %w", err)
	}
	return pid, nil
}

// IsProcessAlive probes with kill(pid, 0): a process we lack permission to
// signal still counts as alive, while pid <= 0 and every other error are dead.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	return false
}

// WriteVersionFile uses plain AtomicWrite deliberately, as in WritePIDFile.
func WriteVersionFile(dir, version string, logger *slog.Logger) error {
	logger = loggerOrDiscard(logger)
	path := DaemonVersion(dir)
	// Order matters: the breadcrumb precedes the write, so a write failure still
	// leaves a record of the intent.
	logger.Debug("daemon.version write", "path", path)
	return fileutil.AtomicWrite(path, []byte(version+"\n"))
}

// ReadVersionFile gives ErrVersionFileAbsent for a missing file, but ("", nil)
// for an empty one — a recorded blank version is distinguishable from none.
func ReadVersionFile(dir string) (string, error) {
	data, err := readDaemonFile(DaemonVersion(dir), ErrVersionFileAbsent)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func readDaemonFile(path string, absentSentinel error) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, absentSentinel
		}
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	return data, nil
}
