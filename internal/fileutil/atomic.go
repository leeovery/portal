// Package fileutil provides filesystem utilities for atomic file operations.
package fileutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Write-phase sentinels let callers errors.Is-discriminate which step of
// AtomicWrite failed without fileutil itself becoming audit-aware (it is shared
// with out-of-scope sessions.json and must not import internal/log). Each
// sentinel's string is deliberately the matching error_class token from the
// closed AtomicWrite-failure value space, so ClassifyWriteError is a 1:1 map and
// the token cannot drift between the sentinel and the classifier.
var (
	// ErrWriteTempCreate is the temp-file-creation prerequisite phase. It
	// covers both os.MkdirAll (the parent-directory precondition) and
	// os.CreateTemp. The closed error_class space has no write-failed-mkdir,
	// so an MkdirAll failure maps here — it is the temp file's creation
	// prerequisite, not a distinct phase.
	ErrWriteTempCreate = errors.New("write-failed-temp-create")
	// ErrWriteWrite is the temp-file write phase (tmp.Write).
	ErrWriteWrite = errors.New("write-failed-write")
	// ErrWriteFsync is the durability/flush phase. AtomicWrite has no explicit
	// Sync() — create -> write -> Close -> rename — so tmp.Close() is the flush
	// point where deferred write errors surface and is the closest analogue to
	// a failed fsync. We map Close -> ErrWriteFsync (option (a): not adding a
	// real Sync(), not leaving the fsync class unreachable).
	ErrWriteFsync = errors.New("write-failed-fsync")
	// ErrWriteRename is the atomic-rename (commit) phase (os.Rename).
	ErrWriteRename = errors.New("write-failed-rename")
)

// ClassifyWriteError maps a wrapped AtomicWrite error to its closed-space
// error_class token. It is a pure string mapping — no I/O, no logging.
//
// An error matching none of the write-phase sentinels falls back to
// "write-failed-write": a deliberate floor, not a sentinel match. It is the most
// representative "persist did not complete" classification for an unrecognised
// error, so an unexpected failure shape is still attributed to a write failure
// rather than dropped or mis-bucketed.
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

// AtomicWrite0600 writes data to path via AtomicWrite and then chmod's the
// final file to 0600.
//
// AtomicWrite already produces a 0600 temp file (os.CreateTemp default mode),
// but the post-rename chmod is a defensive belt-and-braces against an unusually
// permissive umask leaking broader bits through. Use this helper for any file
// that must be 0600 on disk; centralising it keeps the umask-defence rationale
// in one place.
//
// Note: WritePIDFile / WriteVersionFile in internal/state deliberately do NOT
// use this helper — they tolerate the user's umask and rely on AtomicWrite's
// documented temp-file mode alone.
func AtomicWrite0600(path string, data []byte) error {
	if err := AtomicWrite(path, data); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

// AtomicWrite writes data to path using a temp-file-and-rename strategy.
// It creates the parent directory if it does not exist. On any error the
// temp file is cleaned up.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		// MkdirAll is the temp file's creation prerequisite; the closed
		// error_class space has no write-failed-mkdir, so it maps to the
		// temp-create phase.
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
		// Close is AtomicWrite's flush point (no explicit Sync()), so a Close
		// failure is the closest analogue to a failed fsync — option (a).
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to close temp file: %w", ErrWriteFsync, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to rename temp file: %w", ErrWriteRename, err)
	}

	return nil
}
