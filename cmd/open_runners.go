package cmd

import (
	"context"
	"time"

	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/state"
)

// runnerRegistry is the registry of attach runners as opening a session uses
// it.
type runnerRegistry interface {
	// Listing keeps one attach loop's runner listed while it is attached,
	// marking each attach's client through b and confirming through b that
	// the client leads its session.
	Listing(b runnerreg.Backend) runnerListing
	// Switch asks the runner registered under token, whose terminal leads
	// session current, to switch to target, returning nil once it has
	// accepted.
	Switch(ctx context.Context, token, current, target string) error
	// Take asks every runner on session but this process's own to let it go
	// to by, returning how many did.
	Take(ctx context.Context, session, by string) (int, error)
	// RecordTake records that session is being taken to by, for the clients
	// only the backend's detach ends.
	RecordTake(session, by string) error
	// TakenBy is where the last take of session recorded no earlier than
	// since says it went: empty when there is none.
	TakenBy(session string, since time.Time) string
	// LockTake takes the lock takes of session hold, waiting up to wait, and
	// returns its release.
	LockTake(ctx context.Context, session string, wait time.Duration) (release func(), err error)
}

// runnerListing is runnerreg.Listing's surface.
type runnerListing interface {
	Attach(cfg *runner.Config)
	Ended()
	Close()
}

// inStateDir is what open makes of Portal's state directory, resolved at each
// use, so a directory that won't resolve fails the use rather than the
// command.
func inStateDir[T any](open func(stateDir string) T) (T, error) {
	dir, err := state.Dir()
	if err != nil {
		var none T
		return none, err
	}
	return open(dir), nil
}

// stateRunners is the registry under Portal's state directory.
type stateRunners struct{}

func (stateRunners) Listing(b runnerreg.Backend) runnerListing {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		attachLogger.Warn("register failed", "error", err)
		return unlisted{}
	}
	return r.Listing(b)
}

func (stateRunners) Switch(ctx context.Context, token, current, target string) error {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		return err
	}
	return r.Switch(ctx, token, current, target)
}

func (stateRunners) Take(ctx context.Context, session, by string) (int, error) {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		return 0, err
	}
	return r.Take(ctx, session, by)
}

func (stateRunners) RecordTake(session, by string) error {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		return err
	}
	return r.RecordTake(session, by)
}

func (stateRunners) TakenBy(session string, since time.Time) string {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		return ""
	}
	return r.TakenBy(session, since)
}

func (stateRunners) LockTake(ctx context.Context, session string, wait time.Duration) (func(), error) {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		return nil, err
	}
	return r.LockTake(ctx, session, wait)
}

func (stateRunners) DetachToPicker(ctx context.Context, session string) (int, error) {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		return 0, err
	}
	return r.DetachToPicker(ctx, session)
}

// ConfirmSession is the pid of the runner registered under token once it
// confirms it shows session: what spawn detection walks from to the
// terminal.
func (stateRunners) ConfirmSession(ctx context.Context, token, session string) (int, error) {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		return 0, err
	}
	return r.ConfirmSession(ctx, token, session)
}

// readOnlyRunners is the registry under Portal's state directory for a
// caller that must change nothing, as doctor: it asks as stateRunners does,
// but removes no entry it proves stale.
type readOnlyRunners struct{}

func (readOnlyRunners) ConfirmSession(ctx context.Context, token, session string) (int, error) {
	r, err := inStateDir(runnerreg.New)
	if err != nil {
		return 0, err
	}
	return r.ReadOnly().ConfirmSession(ctx, token, session)
}

// unlisted lists nothing, for a state directory that won't resolve: the
// runner attaches unlisted, and with no token to confirm its attach by.
type unlisted struct{}

func (unlisted) Attach(*runner.Config) {}
func (unlisted) Ended()                {}
func (unlisted) Close()                {}
