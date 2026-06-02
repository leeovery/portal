package state

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/portal/internal/log"
)

// cleanLogger is the clean-component-bound package logger used for the
// orphan-FIFO sweep's cycle summary and its demoted per-removal DEBUG
// breadcrumb. Per-item lstat/remove WARNs stay on the injected bootstrap-bound
// logger seam so an operator can correlate a failure with its bootstrap step;
// the summary and per-item reaped breadcrumb group the sweep's own detail under
// the clean component. It routes through the process-wide handler indirection,
// so log.SetTestHandler captures it in tests.
var cleanLogger = log.For("clean")

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
func SweepOrphanFIFOs(dir string, liveMarkerKeys map[string]struct{}, logger *slog.Logger) error {
	logger = loggerOrDiscard(logger)
	start := time.Now()
	var reaped, skipped int
	pattern := filepath.Join(dir, "hydrate-*.fifo")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob fifos in %s: %w", dir, err)
	}

	for _, path := range matches {
		fi, err := os.Lstat(path)
		if err != nil {
			logger.Warn("orphan fifo lstat failed", "path", path, "error", err)
			skipped++
			continue
		}
		if fi.Mode()&os.ModeNamedPipe == 0 {
			// Not a FIFO — could be a regular file or symlink. Preserve.
			skipped++
			continue
		}
		paneKey := PaneKeyFromFIFOPath(path)
		if _, alive := liveMarkerKeys[paneKey]; alive {
			skipped++
			continue
		}
		if err := os.Remove(path); err != nil {
			logger.Warn("remove orphan fifo failed", "path", path, "error", err)
			skipped++
			continue
		}
		reaped++
		cleanLogger.Debug("orphan fifo reaped", "path", path)
	}
	cleanLogger.Info("orphan-fifo sweep complete", "reaped", reaped, "skipped", skipped, "took", time.Since(start))
	return nil
}
