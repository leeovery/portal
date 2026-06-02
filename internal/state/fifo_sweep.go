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
// breadcrumb. Per-item lstat/remove WARNs stay on SweepOrphanFIFOs' injected
// callerLogger seam so an operator can correlate a failure with its bootstrap
// step; the summary and per-item reaped breadcrumb group the sweep's own detail
// under the clean component. See SweepOrphanFIFOs' doc comment for the full
// rationale of this deliberate two-component split. It routes through the
// process-wide handler indirection, so log.SetTestHandler captures it in tests.
var cleanLogger = log.For("clean")

// SweepOrphanFIFOs removes hydrate-*.fifo files in dir whose paneKey is not
// present in liveMarkerKeys. Non-FIFO files matching the glob (regular files,
// symlinks) are preserved so a misconfigured filesystem entry is not silently
// destroyed. Per-file errors are logged and skipped — one bad entry must not
// abort the rest of the sweep. A missing state directory is treated as
// "nothing to sweep" and returns nil silently, which lets the caller (Phase 5
// bootstrap) invoke the sweep unconditionally before EnsureDir runs in
// early-startup paths.
//
// liveMarkerKeys must contain bare paneKey strings without the
// @portal-skeleton- prefix. The caller is expected to build this set from
// ListSkeletonMarkers after Restore() completes, so that any leftover
// hydrate-*.fifo file from a crashed prior bootstrap is removed before new
// FIFOs are created in the next bootstrap cycle.
//
// Logging — deliberate two-component split (do not "consolidate"):
//
//   - callerLogger is the CALLER-component WARN sink. Per-item lstat/remove
//     failures are emitted on it so they carry the bootstrap step's component
//     (e.g. component=bootstrap when driven from step 10). This is a
//     spec-backed correlation feature: on a reboot-recovery day an operator
//     greps the failing pane's WARN and immediately sees which bootstrap step
//     drove the sweep that hit it (mirrors the spec's "tmux-call detail rides
//     as DEBUG breadcrumbs under the caller's component" rule). It is guarded
//     via loggerOrDiscard so a nil argument is safe.
//   - The cycle-summary INFO ("orphan-fifo sweep complete") and the per-reaped
//     DEBUG breadcrumb ("orphan fifo reaped") are emitted on the package-level
//     cleanLogger (component=clean) BY DESIGN — the spec's concrete cycle
//     catalog pins the orphan-FIFO sweep summary to the clean component
//     ("clean owns sweep summaries"). They never route through callerLogger.
//
// The split is intentional and asymmetric: dropping callerLogger and emitting
// everything under cleanLogger would silently re-attribute the per-item WARNs
// away from the caller's bootstrap step (regressing the correlation feature);
// "consolidating" the summary onto callerLogger would silently re-attribute the
// summary away from clean (violating the cycle catalog). callerLogger's name
// (rather than the pervasive bare logger) makes this boundary contract explicit
// so neither re-attribution happens by accident. SweepOrphanFIFOs is the only
// state-package cycle function with this caller-vs-self split; siblings
// (Commit, CaptureStructure, scrollback seeding) use the injected logger
// uniformly, and WriteFIFOSignal documents its summary/caller split inline
// without taking a misleading injected-logger parameter.
func SweepOrphanFIFOs(dir string, liveMarkerKeys map[string]struct{}, callerLogger *slog.Logger) error {
	callerLogger = loggerOrDiscard(callerLogger)
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
			callerLogger.Warn("orphan fifo lstat failed", "path", path, "error", err)
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
			callerLogger.Warn("remove orphan fifo failed", "path", path, "error", err)
			skipped++
			continue
		}
		reaped++
		cleanLogger.Debug("orphan fifo reaped", "path", path)
	}
	cleanLogger.Info("orphan-fifo sweep complete", "reaped", reaped, "skipped", skipped, log.Took(start))
	return nil
}
