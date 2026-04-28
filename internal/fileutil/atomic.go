// Package fileutil provides filesystem utilities for atomic file operations.
package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

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
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".atomic-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
