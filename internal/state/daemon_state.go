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

// ErrPIDFileAbsent is returned by ReadPIDFile when daemon.pid does not exist.
// Callers use errors.Is to distinguish a missing daemon (no PID file written
// yet) from genuine I/O or parse errors.
var ErrPIDFileAbsent = errors.New("daemon.pid absent")

// ErrVersionFileAbsent is returned by ReadVersionFile when daemon.version does
// not exist. Callers use errors.Is to distinguish a never-recorded version
// from genuine I/O errors.
var ErrVersionFileAbsent = errors.New("daemon.version absent")

// WritePIDFile atomically writes pid to daemon.pid inside dir. The file is
// created with mode 0600 (via fileutil.AtomicWrite's os.CreateTemp).
//
// Note: this deliberately uses plain AtomicWrite — not AtomicWrite0600 — and
// tolerates the user's umask leaking through. The PID file is non-sensitive,
// so the umask-defence chmod used by sessions.json / scrollback writers is
// not applied here.
func WritePIDFile(dir string, pid int) error {
	content := strconv.Itoa(pid) + "\n"
	return fileutil.AtomicWrite(DaemonPID(dir), []byte(content))
}

// ReadPIDFile reads daemon.pid from dir and returns the parsed PID.
//
// If the file does not exist, ErrPIDFileAbsent is returned. Other I/O or
// parse errors are wrapped and returned with a zero PID.
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

// IsProcessAlive reports whether a process with the given PID exists and is
// signalable from the current process. It uses a kill(pid, 0) probe:
//
//   - nil error             → process exists and we can signal it
//   - syscall.EPERM         → process exists but we lack permission (still alive)
//   - syscall.ESRCH         → no such process
//   - any other error / pid ≤ 0 → treated as dead
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

// DaemonAlive reports whether dir contains a daemon.pid pointing at a live
// process. Both conditions must hold: missing PID file, unparseable PID file,
// or a dead process all yield false.
func DaemonAlive(dir string) bool {
	pid, err := ReadPIDFile(dir)
	if err != nil {
		return false
	}
	return IsProcessAlive(pid)
}

// WriteVersionFile atomically writes the daemon's version marker to
// daemon.version inside dir. The file is created with mode 0600.
//
// Note: like WritePIDFile, this deliberately uses plain AtomicWrite — not
// AtomicWrite0600 — and tolerates the user's umask. The version marker is
// non-sensitive.
func WriteVersionFile(dir, version string, logger *slog.Logger) error {
	logger = loggerOrDiscard(logger)
	path := DaemonVersion(dir)
	// Emit the breadcrumb BEFORE the atomic-write side effect so a subsequent
	// write failure (read-only FS, ENOSPC) still leaves a paper trail of the
	// caller's intent in portal.log. version and pid are now baseline attrs
	// injected per-record by the configured handler, so they are no longer
	// passed at this call site. The path attr keeps the grep contract for
	// Defect 3 investigations — see spec § Change 3.
	logger.Debug("daemon.version write", "path", path)
	return fileutil.AtomicWrite(path, []byte(version+"\n"))
}

// ReadVersionFile reads daemon.version from dir, trims trailing whitespace,
// and returns the recorded version string.
//
// If the file does not exist, ErrVersionFileAbsent is returned. An empty file
// returns ("", nil) — distinct from absent — so callers can tell that the
// daemon recorded a version (even if blank) versus never having written one.
func ReadVersionFile(dir string) (string, error) {
	data, err := readDaemonFile(DaemonVersion(dir), ErrVersionFileAbsent)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readDaemonFile reads path and classifies the open error: a missing file
// surfaces as absentSentinel, any other I/O error is wrapped with a
// "read <basename>: %w" prefix. Successful reads return the raw bytes for the
// caller to trim/parse.
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
