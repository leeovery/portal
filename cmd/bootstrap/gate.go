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
	// ZmxDir reports nil when the state directory belongs to the zmx directory
	// Portal runs against, or records none (ZmxDirOwner.Check).
	ZmxDir func() error
	// Present reports whether the saver session is there, whatever it runs.
	Present func(ctx context.Context) (bool, error)
	Logger  *slog.Logger
}

// Owed reports whether a bootstrap is owed: the latch is missing, another
// boot's or another version's, the state directory may belong to another zmx
// directory, or there is no saver session. A bootstrap removes the latch
// before its first change and writes it only once it has finished, so a
// missing latch is a bootstrap under way, or one cut short or failed before
// it finished. A state directory of another zmx directory owes the bootstrap
// that refuses it, so a warm command never records a session in another
// directory's index. A saver gone went with the sessions it watched, at a
// logout or as every zmx daemon died, and only the full bootstrap, restore
// and all, brings them back. A saver that can't be read is owed one too. With
// the latch current it reads the zmx directory's record and asks nothing but
// the saver's presence, once: the warm path every command takes.
func (g Gate) Owed(ctx context.Context) bool {
	logger := log.OrDiscard(g.Logger)
	if !state.LatchCurrent(g.StateDir, g.Running) {
		logger.Debug("bootstrap owed", "reason", "latch")
		return true
	}
	if err := g.ZmxDir(); err != nil {
		logger.Debug("bootstrap owed", "reason", "zmx dir", "error", err)
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
