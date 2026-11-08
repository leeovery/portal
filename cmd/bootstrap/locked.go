package bootstrap

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
)

// The bounds of a bootstrap's longest run, step by step, each wait at its
// bound and each backend call at its ceiling. zmx's own ceilings are pinned
// within saverhost's.
const (
	// checkRun is zmx version, a run of the binary bounded as Create is, with
	// its grace.
	checkRun = saverhost.CreateCeiling + saverhost.RunGrace
	// sweepRun is the sweep's reads of the saver session and of the listing
	// the flags are judged by.
	sweepRun = 2 * saverhost.ReadCeiling
	// saverRun is the read of whether a saver runs; the judgement of one
	// found, and the end of one Portal can't use; the listing of the sessions
	// to mark owed, whose write waits on commit.lock besides; then the wait
	// for saver.lock, which an uninstall may hold, then the ensure's own run,
	// which that wait outlasts.
	saverRun = saverhost.ReadCeiling + saverhost.JudgeRun + saverhost.ReadCeiling + 2*saverhost.LockWait
	// restoreRun is restore's listings: before its first session, after its
	// first pass, for a session made whose helper died, and once its retries
	// end, for one made after all or remade whose helper died too.
	restoreRun = restore.Listings * saverhost.ReadCeiling
	// lookup is restore's lookup of the session holding a name: a read, then,
	// when the name is held under a spelling the listing doesn't hold, a read
	// of whether that spelling answers.
	lookup = 2 * saverhost.ReadCeiling
	// restoreSessionRun is a session restore makes: its Create, a run of the
	// binary, with its grace, then the Create's read of whether a session it
	// read back no answer from speaks another protocol; then the longer of
	// the refusal of one that does (its zmx kill, a run of the binary) and a
	// second ask: of a Create that read back no creation, that lookup
	// finding none, or of one that failed otherwise, a lookup, then
	// backend.RecheckAfter's wait and a lookup again. Its record waits on
	// commit.lock besides.
	restoreSessionRun = saverhost.CreateCeiling + saverhost.RunGrace + saverhost.ReadCeiling +
		max(saverhost.KillCeiling+saverhost.RunGrace, 2*lookup+backend.RecheckAfter)
	// lockSlack covers what no bound does: pgrep and the proofs' ps calls,
	// and the flag, latch and index files.
	lockSlack = 15 * time.Second
)

// LockWait bounds the wait for bootstrap.lock while another bootstrap holds
// it, given the entries sessions.json holds: the holder's longest run, which
// looks each up and restores at most that many sessions, then retries those
// it couldn't make, with lockSlack over. Each entry may wait on commit.lock
// twice besides its session's restore: the last listing records a session
// it finds made after all, which the first pass recorded too when it made
// that session first, its helper since dead. The retries are one term
// however many there are: no try starts once restore.RetryBudget has run
// out, so the last starts within it and runs past it: the retry's lookup,
// then a session's restore. A holder still running past it all is stuck, and
// the wait gives up rather than queue behind it.
func LockWait(entries int) time.Duration {
	each := restoreSessionRun + commitlock.Bound()
	retries := restore.RetryBudget + lookup + each
	perEntry := lookup + each + commitlock.Bound()
	return checkRun + sweepRun + saverRun + commitlock.Bound() + restoreRun + time.Duration(entries)*perEntry + retries + lockSlack
}

// LockNotice is how long a wait for bootstrap.lock goes unremarked.
const LockNotice = time.Second

// Locked runs a bootstrap one at a time on its state directory, under
// bootstrap.lock, so two windows opening at once never restore a session
// twice. One that waited out another runs only if a bootstrap is still owed
// once it holds the lock. Hold takes the lock alone, as Run does, for other
// work that must not race a bootstrap's restore, reading only Lock, Notice
// and Logger.
type Locked struct {
	// Lock takes bootstrap.lock, waiting within its bound, and returns its
	// release.
	Lock func(ctx context.Context) (release func(), err error)
	// Owed is the gate, asked again once the lock is held.
	Owed      func(ctx context.Context) bool
	Bootstrap Runner
	// Notice is how long a wait for the lock lasts before the progress
	// emitter hears of it; zero is LockNotice.
	Notice time.Duration
	Logger *slog.Logger
}

var _ Runner = Locked{}

// Run takes the lock, then runs the bootstrap if one is still owed. A wait
// for the lock past its notice is told to ctx's progress emitter, once. A lock
// it can't take, another bootstrap holding it past its bound included, is
// fatal, and nothing runs.
func (l Locked) Run(ctx context.Context) ([]Warning, error) {
	start := time.Now()
	release, err := l.Hold(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if !l.Owed(ctx) {
		log.OrDiscard(l.Logger).Info("bootstrap skipped", "detail", "another bootstrap ran while this one waited", log.Took(start))
		return nil, nil
	}
	return l.Bootstrap.Run(ctx)
}

// Hold takes the lock and returns its release. A wait for it past its notice
// is told to ctx's progress emitter, once. A lock it can't take, another
// holder keeping it past its bound included, is fatal.
func (l Locked) Hold(ctx context.Context) (release func(), err error) {
	start := time.Now()
	stop := l.noticeWait(progressEmitterFromContext(ctx))
	release, err = l.Lock(ctx)
	stop()
	if err != nil {
		log.OrDiscard(l.Logger).Error("bootstrap.lock unavailable", "error", err, log.Took(start))
		return nil, lockFatal(err)
	}
	return release, nil
}

// noticeWait tells emit, once the notice has passed, that this bootstrap
// waits for another. The stop it returns ends the watch, waiting out a notice
// already under way, so none follows it.
func (l Locked) noticeWait(emit ProgressEmitter) (stop func()) {
	if emit == nil {
		return func() {}
	}
	notice := cmp.Or(l.Notice, LockNotice)
	sent := make(chan struct{})
	timer := time.AfterFunc(notice, func() {
		defer close(sent)
		emit(StepEvent{Waiting: true})
	})
	return func() {
		if !timer.Stop() {
			<-sent
		}
	}
}

func lockFatal(err error) *FatalError {
	if errors.Is(err, state.ErrBootstrapLockHeld) {
		return NewFatal("Portal gave up waiting for another Portal to finish starting: "+err.Error(), err)
	}
	return NewFatal("Portal failed to take bootstrap.lock: "+err.Error(), err)
}
