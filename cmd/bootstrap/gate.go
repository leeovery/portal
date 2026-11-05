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
	// Portal runs against, as recorded (ZmxDirOwner.Claimed).
	ZmxDir func() error
	// Saver reports whether the saver session is there, and fails unless the
	// state directory's daemon.pid names the process it runs
	// (saverhost.Host.Recorded).
	Saver  func(ctx context.Context) (bool, error)
	Logger *slog.Logger
}

// Owed reports whether a bootstrap is owed: the latch is missing, another
// boot's or another version's, the state directory records no zmx directory
// or may belong to another, or there is no saver session of its own. A
// bootstrap removes the latch before its first change and writes it only once
// it has finished, so a missing latch is a bootstrap under way, or one cut
// short or failed before it finished. A state directory of another zmx
// directory owes the bootstrap that refuses it, so a warm command never
// records a session in another directory's index, and one that records none
// owes the bootstrap that records the one in use, so the protection is never
// off for long. A saver gone went with the sessions it watched, at a logout
// or as every zmx daemon died, and only the full bootstrap, restore and all,
// brings them back. A saver that can't be read, or whose process daemon.pid
// doesn't name, is owed one too: it may be another state directory's, which
// the bootstrap refuses, and never a warm command's to run on. With the latch
// current it reads the zmx directory's record, the saver session once and
// daemon.pid: the warm path every command takes.
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
	present, err := g.Saver(ctx)
	switch {
	case err != nil:
		logger.Debug("bootstrap owed", "reason", "saver unknown", "error", err)
	case !present:
		logger.Debug("bootstrap owed", "reason", "no saver")
	}
	return err != nil || !present
}
