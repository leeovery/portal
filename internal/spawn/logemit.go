package spawn

import (
	"fmt"
	"log/slog"

	"github.com/leeovery/portal/internal/log"
)

// logemit.go is the SINGLE SOURCE of the closed `spawn`-component log-emission
// shapes the spec governs (spec §Observability). Both spawn surfaces — the CLI
// (cmd/spawn.go, the test seam) and the picker (internal/tui/burst_observability.go,
// the dominant production path) — delegate their emission to these helpers, so the
// closed message strings + attr-key list live in exactly one place and the two paths
// cannot drift. It sits alongside the other shared spawn renderers
// (message.go's GoneMessage/UnsupportedNoopMessage, classify.go's PartitionResults).
//
// Every helper is nil-logger tolerant (log.OrDiscard at entry): the offline capture
// harness and unit tests that never assert logging pass a nil logger and silently
// discard. Production passes log.For("spawn"). internal/spawn may import internal/log
// (internal/log never imports internal/spawn), so threading the stdlib *slog.Logger
// through here introduces no import cycle. Only the closed spawn attr keys appear —
// the baseline pid/version/process_role are injected per-record by the production
// handler, never at these call sites.

// LogWindowResults emits one "per-window spawn + ack outcome" record per external
// window, carrying its session, its ack outcome, and the opaque driver detail. The
// driver's OS-specific string rides up as the opaque `detail` attr, never parsed
// (driver-quarantine). It is called standalone by the CLI permission path (which
// emits the per-window detail before the permission INFO) and internally by
// LogBatchSummary (which pairs it with the cycle summary). The picker's permission
// path deliberately does NOT call it — that asymmetry is preserved at the call
// sites, not here.
//
// Records split by outcome so the operator can see WHY each window failed at the
// production-default INFO level, not just THAT windows failed (the batch summary's
// opened N/N counts). A window that FAILED (!Confirmed() — its ack is AckTimeout or
// AckFailed) and whose outcome is NOT permission-required emits at WARN with the
// distinct "external window failed" message. This deliberately spans BOTH
// non-permission failure modes: AckFailed (the adapter reported no window opened;
// detail is the osascript error text) AND AckTimeout (the window opened but its
// token never arrived within budget; detail is a benign success string). Both are
// genuine failures the operator must see; the ack attr distinguishes the mode
// (failed vs timeout). Restricting the WARN to open-failures would re-introduce the
// exact invisibility gap this closes.
//
// The permission-required window is excluded from the WARN even though it is also
// !Confirmed() (AckFailed): its detail is already carried by the dedicated
// LogPermission INFO event, and the CLI's permission arm calls LogWindowResults
// before LogPermission, so the exclusion prevents a double-report. Every other
// window — a confirmed window, or the permission-required window — emits at DEBUG
// with the unchanged "external window" message.
func LogWindowResults(logger *slog.Logger, results []WindowResult) {
	logger = log.OrDiscard(logger)
	for _, r := range results {
		failed := !r.Confirmed()
		nonPermission := r.Result.Outcome != OutcomePermissionRequired
		if failed && nonPermission {
			logger.Warn("external window failed", "session", r.Session, "ack", string(r.Ack), "detail", r.Result.Detail)
			continue
		}
		logger.Debug("external window", "session", r.Session, "ack", string(r.Ack), "detail", r.Result.Detail)
	}
}

// LogBatchSummary emits the spawn batch summary from the burst-completion chokepoint:
// the per-window DEBUG loop (via LogWindowResults) followed by one INFO cycle summary
// (`opened <opened>/<total>`) carrying the closed resolution/terminal/bundle_id/opened/
// total/batch attrs, in that order.
//
// The opened count is derived from the shared spawn.PartitionResults chokepoint
// (confirmed windows only) plus 1 when triggerAttached — the trigger self-attach counts
// only on full success; a partial/permission failure skips it and never counts it. total
// is N (the external set plus the one trigger self-attach target) and is passed through
// unchanged: it cannot be derived from len(results), because a pre-spawn abort or a
// cancelled burst can leave fewer results than external windows while total stays N.
func LogBatchSummary(logger *slog.Logger, id Identity, resolution Resolution, results []WindowResult, total int, triggerAttached bool, batch string) {
	logger = log.OrDiscard(logger)
	LogWindowResults(logger, results)

	confirmed, _ := PartitionResults(results)
	opened := len(confirmed)
	if triggerAttached {
		opened++
	}

	logger.Info(fmt.Sprintf("opened %d/%d", opened, total),
		"resolution", string(resolution),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
		"opened", opened,
		"total", total,
		"batch", batch,
	)
}

// LogPermission emits the permission-required outcome line — a distinct entry in the
// closed spawn event catalog. It carries the closed resolution/terminal/bundle_id attrs
// plus the opaque driver detail (never an AppleEvent number this layer interpreted; the
// orchestrator switched on the generic Outcome alone) and NO opened/total/batch summary
// attrs — the burst stopped on the first wall, so there is no cycle summary to report.
func LogPermission(logger *slog.Logger, id Identity, resolution Resolution, detail string) {
	log.OrDiscard(logger).Info("permission required — nothing self-attached",
		"resolution", string(resolution),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
		"detail", detail,
	)
}

// LogUnsupported emits the outcome line for the N≥2 atomic no-op on an unsupported/NULL
// terminal. Nothing was attempted, so it carries only the closed resolution/terminal/
// bundle_id attrs — no per-window records and no opened/total counts. The resolution
// attr is always the ResolutionUnsupported literal (the gate fires exactly when the
// identity resolved unsupported).
func LogUnsupported(logger *slog.Logger, id Identity) {
	log.OrDiscard(logger).Info("unsupported terminal — nothing opened",
		"resolution", string(ResolutionUnsupported),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
	)
}

// LogGone emits the single outcome line for a pre-flight abort, naming the gone
// session(s) via the shared GoneMessage renderer. Nothing was attempted (detection
// never ran), so it carries no per-window records and no resolution/opened/total attrs.
func LogGone(logger *slog.Logger, gone []string) {
	log.OrDiscard(logger).Info(GoneMessage(gone))
}
