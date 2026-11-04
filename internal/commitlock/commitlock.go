// Package commitlock owns commit.lock, the sidecar every writer of the state
// directory's sessions.json holds exclusively. It is a leaf so that
// internal/state and internal/sessionindex take the one lock without either
// importing the other.
package commitlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

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

// Path is the commit lock in dir. It is not daemon.lock, which the daemon
// holds for its whole life.
func Path(dir string) string { return filepath.Join(dir, name) }

// Acquire takes the commit lock in dir exclusively, retrying until the bound.
// A lock not granted within it returns an error wrapping ErrHeld.
//
// The lock is released by closing the returned file, and by the kernel when
// the holding process exits, so a crashed holder never wedges the next one.
func Acquire(dir string) (*os.File, error) {
	path := Path(dir)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open commit lock: %w", err)
	}
	if err := lock(f, path, timeout); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// lock takes f's lock on path exclusively, retrying until bound has passed. A
// lock not granted within it is an error wrapping ErrHeld.
func lock(f *os.File, path string, bound time.Duration) error {
	deadline := time.Now().Add(bound)
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			return fmt.Errorf("flock commit lock %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: %s", ErrHeld, path)
		}
		time.Sleep(pollInterval)
	}
}
