package tui

import (
	"log/slog"

	"github.com/leeovery/portal/internal/spawn"
)

func WithSpawnLogger(logger *slog.Logger) Option {
	return func(m *Model) { m.spawnLogger = logger }
}

// total = external set + the trigger self-attach target, from model state so a
// cancelled burst still reports N even with fewer results than windows.
func (m Model) emitBurstSummary(batch string, id spawn.Identity, resolution spawn.Resolution, results []spawn.WindowResult, triggerAttached bool) {
	total := len(m.burstExternal) + 1
	spawn.LogBatchSummary(m.spawnLogger, id, resolution, results, total, triggerAttached, batch)
}

func (m Model) emitPermission(id spawn.Identity, resolution spawn.Resolution, detail string) {
	spawn.LogPermission(m.spawnLogger, id, resolution, detail)
}

func (m Model) emitNotStarted(err error) {
	spawn.LogNotStarted(m.spawnLogger, err)
}

func (m Model) emitUnsupportedNoop(id spawn.Identity) {
	spawn.LogUnsupported(m.spawnLogger, id)
}

func (m Model) emitPreflightAbort(gone []string) {
	spawn.LogGone(m.spawnLogger, gone)
}
