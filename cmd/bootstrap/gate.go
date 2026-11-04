package bootstrap

import (
	"context"
	"log/slog"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
)

// Gate tells a command whether a bootstrap is owed before it runs.
type Gate struct {
	// StateDir holds the latch.
	StateDir string
	// Running is the latch a bootstrap of this boot and this version writes.
	Running state.Latch
	// Present reports whether the saver session is there, whatever it runs.
	Present func(ctx context.Context) (bool, error)
	Logger  *slog.Logger
}

// Owed reports whether a bootstrap is owed: the latch is another boot's or
// another version's, the restoring flag is raised, or there is no saver
// session. A raised flag is a bootstrap under way, or one cut short or failed
// before it finished its restore, which only a bootstrap finishes. A saver
// gone went with the sessions it watched, at a logout or as every zmx daemon
// died, and only the full bootstrap, restore and all, brings them back. A
// flag or a saver that can't be read is owed one too. With the latch current
// it asks nothing but the flag, by a stat, and the saver's presence, once:
// the warm path every command takes.
func (g Gate) Owed(ctx context.Context) bool {
	logger := log.OrDiscard(g.Logger)
	if !state.LatchCurrent(g.StateDir, g.Running) {
		logger.Debug("bootstrap owed", "reason", "latch")
		return true
	}
	switch raised, err := state.Restoring.Exists(g.StateDir); {
	case err != nil:
		logger.Debug("bootstrap owed", "reason", "restoring", "error", err)
		return true
	case raised:
		logger.Debug("bootstrap owed", "reason", "restoring")
		return true
	}
	present, err := g.Present(ctx)
	switch {
	case err != nil:
		logger.Debug("bootstrap owed", "reason", "saver unknown", "error", err)
	case !present:
		logger.Debug("bootstrap owed", "reason", "no saver")
	}
	return err != nil || !present
}
