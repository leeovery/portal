package tui

// Picker-side spawn observability: the burst completion chokepoint emits the closed
// `spawn`-component instrumentation the multi-target open burst (cmd/open_burst_run.go)
// also emits — one INFO cycle summary (`spawn: opened N/N`) plus one DEBUG per external
// window, using only the closed spawn attr keys
// (batch/terminal/bundle_id/resolution/session/ack/opened/total/detail).
//
// The emission shapes themselves live ONCE in internal/spawn/logemit.go
// (spawn.LogBatchSummary / spawn.LogWindowResults / spawn.LogPermission /
// spawn.LogUnsupported / spawn.LogGone) — the single source of the closed spawn log
// vocabulary that both this picker path and the open burst delegate to, so the two paths
// cannot drift. The methods here are thin model-bound wrappers: they supply m.spawnLogger
// and
// the model-derived total, and route each terminal outcome to the matching shared shape.
// The driver's OS-specific string rides up as the opaque `detail` attr, never parsed
// (driver-quarantine).

import (
	"log/slog"

	"github.com/leeovery/portal/internal/spawn"
)

// WithSpawnLogger wires the spawn-component logger the burst completion chokepoint emits
// its batch summary + per-window detail through. Nil-tolerant: the shared spawn helpers
// wrap it in log.OrDiscard, so a nil logger (the offline capture harness / unit tests that
// never assert logging) silently discards. Production passes log.For("spawn").
func WithSpawnLogger(logger *slog.Logger) Option {
	return func(m *Model) { m.spawnLogger = logger }
}

// emitBurstSummary emits the spawn batch summary from the burst completion chokepoint —
// the per-window DEBUG loop plus one INFO `opened <opened>/<total>` — via the shared
// spawn.LogBatchSummary renderer.
//
// opened is derived inside the shared helper from spawn.PartitionResults (confirmed
// external windows) plus 1 when triggerAttached (the trigger self-attach — full success
// ONLY; a partial/permission failure skips it and never counts it). total is N — the
// external set plus the one trigger self-attach target — computed from the model's
// burst-external set so it stays N even when a cancelled/pre-spawn-aborted burst left
// fewer results than external windows.
func (m Model) emitBurstSummary(batch string, id spawn.Identity, resolution spawn.Resolution, results []spawn.WindowResult, triggerAttached bool) {
	total := len(m.burstExternal) + 1
	spawn.LogBatchSummary(m.spawnLogger, id, resolution, results, total, triggerAttached, batch)
}

// emitPermission emits the permission-required outcome line via the shared
// spawn.LogPermission renderer — the closed resolution/terminal/bundle_id attrs plus the
// opaque driver detail, and NO opened/total/batch summary attrs (the burst stopped on the
// first wall, so there is no cycle summary to report). Routing the permission arm here
// instead of emitBurstSummary is what keeps the picker's permission emission to ONLY the
// permission INFO (no per-window DEBUG lines) — the deliberate picker/open-burst asymmetry.
func (m Model) emitPermission(id spawn.Identity, resolution spawn.Resolution, detail string) {
	spawn.LogPermission(m.spawnLogger, id, resolution, detail)
}

// emitUnsupportedNoop emits the outcome line for the N≥2 atomic no-op on an
// unsupported/NULL terminal via the shared spawn.LogUnsupported renderer. Nothing was
// attempted, so it carries only the closed resolution/terminal/bundle_id attrs — no
// per-window records and no opened/total counts.
func (m Model) emitUnsupportedNoop(id spawn.Identity) {
	spawn.LogUnsupported(m.spawnLogger, id)
}

// emitPreflightAbort emits the outcome line for a pre-flight abort via the shared
// spawn.LogGone renderer: the burst goroutine aborted before spawning anything, so it
// names the gone session(s) and carries no per-window records and no
// resolution/opened/total attrs.
func (m Model) emitPreflightAbort(gone []string) {
	spawn.LogGone(m.spawnLogger, gone)
}
