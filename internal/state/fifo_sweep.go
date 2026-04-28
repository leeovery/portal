package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SweepOrphanFIFOs removes hydrate-*.fifo files in dir whose paneKey is not
// present in liveMarkerKeys. Non-FIFO files matching the glob (regular files,
// symlinks) are preserved so a misconfigured filesystem entry is not silently
// destroyed. Per-file errors are logged via logger and skipped — one bad
// entry must not abort the rest of the sweep. A missing state directory is
// treated as "nothing to sweep" and returns nil silently, which lets the
// caller (Phase 5 bootstrap) invoke the sweep unconditionally before
// EnsureDir runs in early-startup paths.
//
// liveMarkerKeys must contain bare paneKey strings without the
// @portal-skeleton- prefix. The caller is expected to build this set from
// ListSkeletonMarkers after Restore() completes, so that any leftover
// hydrate-*.fifo file from a crashed prior bootstrap is removed before new
// FIFOs are created in the next bootstrap cycle.
//
// logger may be nil; nil is treated as a no-op logger so callers do not need
// to check before calling.
func SweepOrphanFIFOs(dir string, liveMarkerKeys map[string]struct{}, logger *Logger) error {
	pattern := filepath.Join(dir, "hydrate-*.fifo")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob fifos in %s: %w", dir, err)
	}

	for _, path := range matches {
		fi, err := os.Lstat(path)
		if err != nil {
			logger.Warn("fifo-sweep", "lstat %s: %v", path, err)
			continue
		}
		if fi.Mode()&os.ModeNamedPipe == 0 {
			// Not a FIFO — could be a regular file or symlink. Preserve.
			continue
		}
		paneKey := paneKeyFromFIFOFilename(filepath.Base(path))
		if _, alive := liveMarkerKeys[paneKey]; alive {
			continue
		}
		if err := os.Remove(path); err != nil {
			logger.Warn("fifo-sweep", "remove orphan FIFO %s: %v", path, err)
			continue
		}
		logger.Info("fifo-sweep", "removed orphan FIFO %s", path)
	}
	return nil
}

// paneKeyFromFIFOFilename strips the "hydrate-" prefix and ".fifo" suffix
// from the base name to recover the canonical paneKey embedded in the FIFO
// filename. Inverse of FIFOPath's filename component.
func paneKeyFromFIFOFilename(name string) string {
	name = strings.TrimSuffix(name, ".fifo")
	name = strings.TrimPrefix(name, "hydrate-")
	return name
}
