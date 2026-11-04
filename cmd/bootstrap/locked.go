package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

// The bounds of a bootstrap's longest run, step by step, each wait at its
// bound and each backend call at its ceiling. zmx's own ceilings are pinned
// within saverhost's.
const (
	// checkRun is zmx version, a run of the binary bounded as Create is.
	checkRun = saverhost.CreateCeiling
	// sweepRun is the sweep's reads of the saver session and of the listing
	// the flags are judged by.
	sweepRun = 2 * saverhost.ReadCeiling
	// saverRun is the wait for saver.lock, which an uninstall may hold, then
	// the ensure's own run, which that wait outlasts, then the protocol's
	// read.
	saverRun = 2*saverhost.LockWait + saverhost.ReadCeiling
	// restoreRun is restore's listing, before its first session.
	restoreRun = saverhost.ReadCeiling
	// restoreSessionRun is a session restore makes: its Create; when that
	// fails, a read, a probe ceiling's wait and a read again. Its record
	// waits on commit.lock besides.
	restoreSessionRun = saverhost.CreateCeiling + 3*saverhost.ReadCeiling
	// lockSlack covers what no bound does: pgrep and the proofs' ps calls,
	// and the flag, latch and index files.
	lockSlack = 15 * time.Second
)

// LockWait bounds the wait for bootstrap.lock while another bootstrap holds
// it, given the entries sessions.json holds: the holder's longest run, which
// restores at most that many sessions, with lockSlack over. A holder still
// running past it is stuck, and the wait gives up rather than queue behind it.
func LockWait(entries int) time.Duration {
	each := restoreSessionRun + commitlock.Bound()
	return checkRun + sweepRun + saverRun + restoreRun + time.Duration(entries)*each + lockSlack
}

// Locked runs a bootstrap one at a time on its state directory, under
// bootstrap.lock, so two windows opening at once never restore a session
// twice. One that waited out another runs only if a bootstrap is still owed
// once it holds the lock.
type Locked struct {
	// Lock takes bootstrap.lock, waiting within its bound, and returns its
	// release.
	Lock func(ctx context.Context) (release func(), err error)
	// Owed is the gate, asked again once the lock is held.
	Owed      func(ctx context.Context) bool
	Bootstrap Runner
	Logger    *slog.Logger
}

var _ Runner = Locked{}

// Run takes the lock, then runs the bootstrap if one is still owed. A lock it
// can't take, another bootstrap holding it past its bound included, is fatal,
// and nothing runs.
func (l Locked) Run(ctx context.Context) ([]Warning, error) {
	logger := log.OrDiscard(l.Logger)
	start := time.Now()
	release, err := l.Lock(ctx)
	if err != nil {
		logger.Error("bootstrap.lock unavailable", "error", err, log.Took(start))
		return nil, lockFatal(err)
	}
	defer release()
	if !l.Owed(ctx) {
		logger.Info("bootstrap skipped", "detail", "another bootstrap ran while this one waited", log.Took(start))
		return nil, nil
	}
	return l.Bootstrap.Run(ctx)
}

func lockFatal(err error) *FatalError {
	if errors.Is(err, state.ErrBootstrapLockHeld) {
		return NewFatal("Portal gave up waiting for another Portal to finish starting: "+err.Error(), err)
	}
	return NewFatal("Portal failed to take bootstrap.lock: "+err.Error(), err)
}
