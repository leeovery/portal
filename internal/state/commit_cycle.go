package state

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

const commitLockName = "commit.lock"

// ErrCommitLockHeld separates contention on the commit lock from a genuine
// open(2)/flock failure.
var ErrCommitLockHeld = errors.New("commit lock held by another committer")

// commitLockTimeout bounds the acquire. An unbounded acquire would park the
// daemon's tick loop behind a holder that is alive but stuck.
var commitLockTimeout = 5 * time.Second

var commitLockPollInterval = 5 * time.Millisecond

// CommitLock is the sidecar every committing cycle holds exclusively. It is not
// daemon.lock, which the daemon holds for its whole life.
func CommitLock(dir string) string { return filepath.Join(dir, commitLockName) }

// CommitCycle is one committing cycle: capture, re-file, the caller's dump,
// then the commit and its housekeeping pass.
type CommitCycle struct {
	Client CaptureCycleClient
	Dir    string
	// LoadPrev supplies the previous index, and is called only once the commit
	// lock is held.
	LoadPrev func() *Index
	HashMap  HashMap
	// Dump writes the caller's scrollback for the capture and reports whether
	// any file changed. Nil dumps nothing. An error ends the cycle uncommitted.
	Dump   func(CaptureCycle) (bool, error)
	Logger *slog.Logger
}

// RunCommitCycle runs cycle under the exclusive commit lock, held from the
// skeleton-marker read to the end of the housekeeping pass, so no two
// committers' captures and collections interleave. A lock not granted within
// the bound returns an error wrapping ErrCommitLockHeld with nothing read or
// written. A failed capture returns before the dump, and a failed dump before
// the commit, its error returned as the dump gave it.
func RunCommitCycle(cycle CommitCycle) (CaptureCycle, error) {
	lock, err := acquireCommitLock(cycle.Dir)
	if err != nil {
		return CaptureCycle{}, err
	}
	defer func() { _ = lock.Close() }()

	capture, err := captureAndRefile(cycle.Client, cycle.Dir, cycle.LoadPrev(), cycle.HashMap, cycle.Logger)
	if err != nil {
		return capture, fmt.Errorf("capture: %w", err)
	}
	changed := false
	if cycle.Dump != nil {
		if changed, err = cycle.Dump(capture); err != nil {
			return capture, err
		}
	}
	if err := Commit(cycle.Dir, capture.Index, changed, cycle.Logger); err != nil {
		return capture, fmt.Errorf("commit: %w", err)
	}
	return capture, nil
}

// The lock is released by closing the returned file, and by the kernel when
// the holding process exits, so a crashed holder never wedges the next one.
func acquireCommitLock(dir string) (*os.File, error) {
	path := CommitLock(dir)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open commit lock: %w", err)
	}
	deadline := time.Now().Add(commitLockTimeout)
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("flock commit lock %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("%w: %s", ErrCommitLockHeld, path)
		}
		time.Sleep(commitLockPollInterval)
	}
}
