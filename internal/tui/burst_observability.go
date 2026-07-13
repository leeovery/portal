package tui

// restore-host-terminal-windows-6-10 — spawn batch-summary observability from the
// burst completion chokepoint.
//
// The picker burst emits the same `spawn`-component instrumentation the CLI does
// (cmd/spawn.go), matching the bootstrap/restore/daemon shape: one INFO cycle
// summary (`spawn: opened N/N`) plus one DEBUG per external window, using only the
// closed spawn attr keys (batch/terminal/bundle_id/resolution/session/ack/opened/
// total/detail). The emit methods here MIRROR cmd/spawn.go's logSpawnSummary /
// tallyWindowResults / logSpawnUnsupported / logSpawnGone so the two emission paths
// stay byte-identical. The driver's OS-specific string rides up as the opaque
// `detail` attr, never parsed (driver-quarantine).

import (
	"fmt"
	"log/slog"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/spawn"
)

// WithSpawnLogger wires the §6-10 spawn-component logger the burst completion
// chokepoint emits its batch summary + per-window detail through. Nil-tolerant: the
// emit methods wrap it in log.OrDiscard, so a nil logger (the offline capture
// harness / unit tests that never assert logging) silently discards. Production
// passes log.For("spawn").
func WithSpawnLogger(logger *slog.Logger) Option {
	return func(m *Model) { m.spawnLogger = logger }
}

// emitBurstSummary emits the §6-10 spawn batch summary from the burst completion
// chokepoint: one DEBUG per external window (session + ack + the opaque driver
// detail) followed by one INFO cycle summary (`opened <opened>/<total>`). It mirrors
// cmd/spawn.go's tallyWindowResults + logSpawnSummary.
//
// opened counts every confirmed external window, plus 1 for the trigger self-attach
// when it occurs (triggerAttached — full success ONLY; a partial/permission failure
// skips the self-attach and never counts it). total is N — the external set plus the
// one trigger self-attach target — so total == N on every batch-summary path. Only
// the closed spawn attr keys appear; the baseline pid/version/process_role are
// injected by the handler.
func (m Model) emitBurstSummary(batch string, id spawn.Identity, resolution spawn.Resolution, results []spawn.WindowResult, triggerAttached bool) {
	logger := log.OrDiscard(m.spawnLogger)
	opened := 0
	for _, r := range results {
		logger.Debug("external window", "session", r.Session, "ack", string(r.Ack), "detail", r.Result.Detail)
		if r.Confirmed() {
			opened++
		}
	}
	if triggerAttached {
		opened++
	}
	total := len(m.burstExternal) + 1
	logger.Info(fmt.Sprintf("opened %d/%d", opened, total),
		"resolution", string(resolution),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
		"opened", opened,
		"total", total,
		"batch", batch,
	)
}

// emitPermission emits the §7-1 permission-required outcome line from the burst
// completion chokepoint — a distinct entry in the closed `spawn` event catalog. It
// mirrors cmd/spawn.go's logSpawnPermission exactly: the closed resolution/terminal/
// bundle_id attrs plus the opaque driver detail (never an AppleEvent number this
// layer interpreted — the burst switched on the generic Outcome via
// spawn.FirstPermission), and NO opened/total/batch summary attrs (the burst stopped
// on the first wall, so there is no cycle summary to report). Routing the permission
// arm here instead of emitBurstSummary is what makes the picker — the dominant real
// path — emit the spec-catalogued permission event in production.
func (m Model) emitPermission(id spawn.Identity, resolution spawn.Resolution, detail string) {
	log.OrDiscard(m.spawnLogger).Info("permission required — nothing self-attached",
		"resolution", string(resolution),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
		"detail", detail,
	)
}

// emitUnsupportedNoop emits the §6-10 outcome line for the N≥2 atomic no-op (§6-9)
// on an unsupported/NULL terminal. Nothing was attempted, so it carries only the
// closed resolution/terminal/bundle_id attrs — no per-window records and no
// opened/total counts. Mirrors cmd/spawn.go's logSpawnUnsupported.
func (m Model) emitUnsupportedNoop(id spawn.Identity) {
	log.OrDiscard(m.spawnLogger).Info("unsupported terminal — nothing opened",
		"resolution", string(spawn.ResolutionUnsupported),
		"terminal", id.Name,
		"bundle_id", id.BundleID,
	)
}

// emitPreflightAbort emits the §6-10 outcome line for a pre-flight abort (§6-7): the
// burst goroutine aborted before spawning anything, so it names the gone session(s)
// and carries no per-window records and no resolution/opened/total attrs. Mirrors
// cmd/spawn.go's logSpawnGone.
func (m Model) emitPreflightAbort(gone []string) {
	log.OrDiscard(m.spawnLogger).Info(fmt.Sprintf("%s %s gone — nothing opened", spawn.QuoteJoin(gone), spawn.GoneVerb(len(gone))))
}
