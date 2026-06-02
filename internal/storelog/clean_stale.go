// Package storelog owns the shared clean-stale batch-summary emission for the
// JSON-backed stores (internal/hooks, internal/project).
//
// Both stores' CleanStale methods are structurally identical batch mutations
// whose only legitimate differences are the per-entry DEBUG attr key and the
// kept/removed partition predicate. The TERMINAL summary emission — the
// success-INFO vs failure-WARN branch and its identical attr list
// (op/entries/via/error/error_class/took) — was duplicated verbatim. This
// package extracts that single emission point so the batch-summary contract
// lives in one place and the two stores cannot drift.
//
// Placement rationale (import-cycle / leaf preservation): the helper needs both
// log.Took (the reserved took attr) and fileutil.ClassifyWriteError (the
// error_class token). internal/log must stay a leaf, so it cannot import
// fileutil; internal/fileutil deliberately must NOT import internal/log (it is
// shared with out-of-scope sessions.json — see fileutil/atomic.go). Neither of
// those two packages can therefore host a helper that references the other.
// internal/storelog is a thin composition package that imports both leaves
// (acyclic: neither leaf imports the other, nor this package), keeping the exact
// (logger, removed, start, saveErr) signature with ClassifyWriteError owned
// inside the helper.
package storelog

import (
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
)

// EmitCleanStaleSummary emits the single terminal batch-summary breadcrumb for a
// store's CleanStale, routing the success vs failure branch through one place so
// the attr list stays identical across the hooks and project stores.
//
//   - saveErr == nil (successful whole-batch Save): one INFO "clean-stale" with
//     op=clean-stale, entries=removed, via=internal, took=<elapsed>.
//   - saveErr != nil (whole-batch Save failed): one WARN "clean-stale" with the
//     same op/entries/via/took plus error=<saveErr> and error_class=<classified>,
//     where error_class is fileutil.ClassifyWriteError(saveErr) — a write-failed-*
//     token from the AtomicWrite phase space, never "unexpected".
//
// took is emitted via log.Took(start) so the attr key and Duration type stay
// pinned in one place. The caller owns Load, the kept/removed partition, the
// zero-removal early return (which skips both Save and this summary), and the
// per-entry DEBUG loop with its store-specific attr key; only the terminal
// summary routes through here.
func EmitCleanStaleSummary(logger *slog.Logger, removed int, start time.Time, saveErr error) {
	if saveErr != nil {
		// Whole-batch persist failure: error_class is a write-failed-* value from
		// the AtomicWrite phase space, NOT "unexpected".
		logger.Warn("clean-stale", "op", "clean-stale", "entries", removed, "via", "internal",
			"error", saveErr, "error_class", fileutil.ClassifyWriteError(saveErr), log.Took(start))
		return
	}

	// entries_failed is omitted: there is no per-entry failure path in either
	// store's batched Save, so M is always 0 and the attr stays absent.
	logger.Info("clean-stale", "op", "clean-stale", "entries", removed, "via", "internal", log.Took(start))
}
