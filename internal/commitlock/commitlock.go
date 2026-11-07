// Package commitlock owns commit.lock, the sidecar every writer of the state
// directory's sessions.json holds exclusively. sessionindex takes it around
// every write, the bootstrap reads its Bound to bound a run that commits, and
// migrate its Path to name what a migration builds. It imports no Portal
// package but fileutil, so sessionindex, whose imports are held to leaves,
// can take it.
package commitlock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"golang.org/x/sys/unix"
)

const name = "commit.lock"

// ErrHeld separates contention on the commit lock from a genuine open(2)/flock
// failure.
var ErrHeld = errors.New("commit lock held by another committer")

// timeout bounds the acquire. An unbounded acquire would park the daemon's
// tick loop behind a holder that is alive but stuck.
var timeout = 5 * time.Second

var pollInterval = 5 * time.Millisecond

// Bound is how long Acquire waits for the lock before it gives up, for a
// caller bounding a run that commits.
func Bound() time.Duration { return timeout }

// Path is the commit lock in dir. It is not daemon.lock, which the daemon
// holds for its whole life.
func Path(dir string) string { return filepath.Join(dir, name) }

// Acquire takes the commit lock in dir exclusively, retrying until the bound.
// A lock not granted within it returns an error wrapping ErrHeld.
//
// The lock is released by closing the returned file, and by the kernel when
// the holding process exits, so a crashed holder never wedges the next one.
func Acquire(dir string) (*os.File, error) {
	return lock(Path(dir), timeout)
}

// lock opens path, creating it, and takes its lock exclusively, retrying until
// bound has passed. A lock not granted within it is an error wrapping ErrHeld.
func lock(path string, bound time.Duration) (*os.File, error) {
	f, err := fileutil.LockWithin(context.Background(), path, fileutil.LockSpec{
		Flag: os.O_RDWR | os.O_CREATE, How: unix.LOCK_EX, Wait: bound, Poll: pollInterval,
	})
	var lockErr *fileutil.LockError
	switch {
	case err == nil:
		return f, nil
	case !errors.As(err, &lockErr):
		return nil, err
	case lockErr.Step == fileutil.LockOpen:
		return nil, fmt.Errorf("open commit lock: %w", lockErr.Err)
	case lockErr.Step == fileutil.LockTimedOut:
		return nil, fmt.Errorf("%w: %s", ErrHeld, path)
	default:
		return nil, fmt.Errorf("flock commit lock %s: %w", path, lockErr.Err)
	}
}
