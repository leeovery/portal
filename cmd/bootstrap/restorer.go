package bootstrap

import (
	"context"

	"github.com/leeovery/portal/internal/restore"
)

// SessionRestorer is restore's Restorer as the bootstrap's restore step asks
// for it, its progress reported through the Restorer's own SetProgress.
type SessionRestorer struct {
	*restore.Restorer
}

var (
	_ Restorer            = SessionRestorer{}
	_ RestoreProgressSink = SessionRestorer{}
)

// Restore restores the saved sessions, this boot's too when noSaver says no
// saver ran to see them end.
func (r SessionRestorer) Restore(ctx context.Context, noSaver bool) restore.Result {
	if noSaver {
		return r.Restorer.Restore(ctx, restore.NoSaverRan())
	}
	return r.Restorer.Restore(ctx)
}
