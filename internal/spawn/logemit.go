package spawn

import (
	"fmt"
	"log/slog"

	"github.com/leeovery/portal/internal/log"
)

// LogWindowResults skips WARN for a permission-required window: LogPermission
// already reports it, and both would be a double-report.
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

// LogBatchSummary takes total rather than deriving it from len(results): a
// pre-spawn abort or a cancelled burst leaves fewer results than windows.
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

// LogTriggerConnectFailed corrects the batch summary, which has to precede the
// self-connect — a successful attach holds the terminal until it ends, or
// exec-replaces the process — and so counts the trigger optimistically.
func LogTriggerConnectFailed(logger *slog.Logger, session, detail string) {
	log.OrDiscard(logger).Warn("trigger did not attach", "session", session, "detail", detail)
}

// LogPermission carries no cycle-summary attrs: the burst stopped at the first
// wall.
func LogPermission(logger *slog.Logger, id Identity, resolution Resolution, detail string) {
	log.OrDiscard(logger).Info("permission required — nothing self-attached",
		"resolution", string(resolution),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
		"detail", detail,
	)
}

// LogUnsupported has no per-window records: nothing was attempted.
func LogUnsupported(logger *slog.Logger, id Identity) {
	log.OrDiscard(logger).Info("unsupported terminal — nothing opened",
		"resolution", string(ResolutionUnsupported),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
	)
}

// LogAckCleanFailed is a burst's acks left behind: each goes once stale, with
// a later burst's clean.
func LogAckCleanFailed(logger *slog.Logger, batch string, err error) {
	log.OrDiscard(logger).Warn("spawn ack clean failed", "batch", batch, "error", err)
}

// LogGone carries no resolution or count attrs: detection never ran.
func LogGone(logger *slog.Logger, gone []string) {
	log.OrDiscard(logger).Info(GoneMessage(gone))
}
