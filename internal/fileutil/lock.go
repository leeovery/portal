package fileutil

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// LockSpec is how LockWithin opens and locks its file.
type LockSpec struct {
	// Flag is the open's flags: os.O_CREATE among them makes the file, mode
	// 0600, when it isn't there.
	Flag int
	// How is unix.LOCK_EX or unix.LOCK_SH.
	How int
	// Wait bounds the wait for another holder; Poll spaces the tries.
	Wait, Poll time.Duration
}

// LockStep is the step of LockWithin that failed.
type LockStep int

const (
	// LockOpen is the open of the lock file.
	LockOpen LockStep = iota
	// LockFlock is a flock refused for a reason other than another holder.
	LockFlock
	// LockCancelled is ctx ending while LockWithin waited.
	LockCancelled
	// LockTimedOut is the bound passing with the lock still held elsewhere.
	LockTimedOut
)

// ErrLockTimedOut is a LockTimedOut LockError's cause.
var ErrLockTimedOut = errors.New("lock held past the bound")

// LockError is a lock LockWithin didn't take: the step that failed, the lock
// file's path, and why: the open's or flock's error, ctx's cause, or
// ErrLockTimedOut.
type LockError struct {
	Step LockStep
	Path string
	Err  error
}

func (e *LockError) Error() string {
	switch e.Step {
	case LockOpen:
		return "open " + e.Path + ": " + e.Err.Error()
	case LockFlock:
		return "flock " + e.Path + ": " + e.Err.Error()
	case LockCancelled:
		return "wait for " + e.Path + ": " + e.Err.Error()
	default:
		return e.Path + ": " + e.Err.Error()
	}
}

func (e *LockError) Unwrap() error { return e.Err }

// LockWithin opens path and takes spec.How's lock on it, trying at once and
// then every spec.Poll, and returns the file, whose close releases the lock;
// the kernel releases it too when its holder exits, so a crashed holder never
// wedges the next. It gives up with a LockError: at once on a failed open or
// a flock refused for a reason other than another holder, EINTR included;
// with ctx's cause should ctx end while it waits between tries; and with
// ErrLockTimedOut at the first try at or after spec.Wait that finds the lock
// still held, so a lock reported held was held at the bound, and the wait
// outlasts the bound by under one poll. An uncontended lock is granted
// whatever the bound and whatever ctx.
func LockWithin(ctx context.Context, path string, spec LockSpec) (*os.File, error) {
	f, err := os.OpenFile(path, spec.Flag, 0o600)
	if err != nil {
		return nil, &LockError{Step: LockOpen, Path: path, Err: err}
	}
	if err := LockOpened(ctx, f, spec); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// LockOpened takes spec.How's lock on f, already open, as LockWithin does
// once it has opened its file, for a caller that opens the file itself;
// spec.Flag plays no part. Its LockError names f's path. f stays open either
// way.
func LockOpened(ctx context.Context, f *os.File, spec LockSpec) error {
	if step, err := awaitLock(ctx, f, spec); err != nil {
		return &LockError{Step: step, Path: f.Name(), Err: err}
	}
	return nil
}

// awaitLock is LockWithin's tries on the open f, reporting the step that
// ended them unlocked.
func awaitLock(ctx context.Context, f *os.File, spec LockSpec) (LockStep, error) {
	deadline := time.Now().Add(spec.Wait)
	for {
		err := unix.Flock(int(f.Fd()), spec.How|unix.LOCK_NB)
		switch {
		case err == nil:
			return 0, nil
		case !errors.Is(err, unix.EWOULDBLOCK):
			return LockFlock, err
		case !time.Now().Before(deadline):
			return LockTimedOut, ErrLockTimedOut
		}
		poll := time.NewTimer(spec.Poll)
		select {
		case <-ctx.Done():
			poll.Stop()
			return LockCancelled, context.Cause(ctx)
		case <-poll.C:
		}
	}
}
