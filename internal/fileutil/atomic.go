package fileutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Write-phase sentinels let a caller errors.Is which step of AtomicWrite failed.
// Each sentinel's string is its error_class token.
var (
	// ErrWriteTempCreate covers creating the parent directory as well as the
	// temp file: the closed error_class space has no separate mkdir phase.
	ErrWriteTempCreate = errors.New("write-failed-temp-create")
	ErrWriteWrite      = errors.New("write-failed-write")
	// AtomicWrite has no explicit Sync, so Close is where deferred write errors
	// surface and maps here.
	ErrWriteFsync  = errors.New("write-failed-fsync")
	ErrWriteRename = errors.New("write-failed-rename")
)

// ClassifyWriteError maps a wrapped AtomicWrite error to its error_class token.
// An error matching no sentinel falls back to "write-failed-write" — a deliberate
// floor, so an unrecognised failure is attributed to a write rather than dropped.
func ClassifyWriteError(err error) string {
	switch {
	case errors.Is(err, ErrWriteTempCreate):
		return "write-failed-temp-create"
	case errors.Is(err, ErrWriteWrite):
		return "write-failed-write"
	case errors.Is(err, ErrWriteFsync):
		return "write-failed-fsync"
	case errors.Is(err, ErrWriteRename):
		return "write-failed-rename"
	default:
		return "write-failed-write"
	}
}

// IsWriteFailure reports whether err carries one of AtomicWrite's write-phase
// sentinels. Unlike ClassifyWriteError it has no floor: an error from anywhere
// else reads as false.
func IsWriteFailure(err error) bool {
	for _, phase := range []error{ErrWriteTempCreate, ErrWriteWrite, ErrWriteFsync, ErrWriteRename} {
		if errors.Is(err, phase) {
			return true
		}
	}
	return false
}

// AtomicWrite0600 writes data to path and chmods the result to 0600. The temp
// file is already created 0600; the extra chmod defends against a permissive
// umask leaking broader bits through.
func AtomicWrite0600(path string, data []byte) error {
	if err := AtomicWrite(path, data); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

// AtomicWrite writes data to path via temp-file-and-rename, creating the parent
// directory if absent and removing the temp file on any error.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w: failed to create directory: %w", ErrWriteTempCreate, err)
	}

	tmp, err := os.CreateTemp(dir, ".atomic-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: failed to create temp file: %w", ErrWriteTempCreate, err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to write temp file: %w", ErrWriteWrite, err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to close temp file: %w", ErrWriteFsync, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to rename temp file: %w", ErrWriteRename, err)
	}

	return nil
}
