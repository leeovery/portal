package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// ErrLockHeld separates expected contention on the sidecar lock from a genuine
// open(2)/flock failure, so a caller can tell a timeout from a save failure.
var ErrLockHeld = errors.New("hooks lock held by another process")

// lockTimeout bounds every mutation and ordinary read. An unbounded acquire
// would park the daemon's tick loop behind a holder that is alive but stuck, so
// waiting is bounded well above the sub-millisecond critical section and well
// inside the sweep's own cadence.
var lockTimeout = 2 * time.Second

var lockPollInterval = 5 * time.Millisecond

// snapshotLockFraction is the hundredth of lockTimeout the clean's advisory
// pre-read waits, which is 20ms at the 2s bound above: four lockPollInterval
// sleeps above the sub-millisecond critical section, so a holder that is merely
// mid-write is still waited out. A hundredth rather than a thousandth because a
// thousandth is 2ms at that bound — below the lockPollInterval floor, so it
// would resolve to the floor and the fraction would stop naming the wait at
// all.
const snapshotLockFraction = 100

// snapshotLockBound bounds the clean's advisory pre-read alone, which may
// degrade to an unlocked read at no cost to correctness, paying one DEBUG
// breadcrumb, so a clean held up by a stuck writer costs the daemon's tick one
// mutation bound rather than two.
// A bound this far under lockTimeout is safe because it can only shorten a wait
// that was already contended: acquireLock attempts its first Flock before it
// tests any deadline, so an uncontended acquire is granted whatever the bound
// and no bound, however short, can degrade one. What is left to trade is only
// how long a contended pre-read persists before falling back.
// It is derived from lockTimeout rather than declared beside it, so lowering the
// mutation bound lowers this one with it until the floor below takes over.
// The floor of one lockPollInterval is load-bearing: acquireLock re-tests its
// deadline only after a poll sleep, so under it the figure named stops being
// the figure waited.
func snapshotLockBound() time.Duration {
	return max(lockTimeout/snapshotLockFraction, lockPollInterval)
}

// lockPath is the sidecar the lock is taken on. It is never hooks.json itself:
// AtomicWrite renames a temp file over the target, so a lock on the target is a
// lock on an unlinked inode the moment anyone writes.
func (s *Store) lockPath() string {
	return s.path + ".lock"
}

// acquireLock opens path with openFlags and polls for the flockMode lock until
// it is granted or bound elapses, returning ErrLockHeld on expiry. The returned
// file must be closed by the caller, which releases the lock.
func acquireLock(path string, openFlags, flockMode int, bound time.Duration) (*os.File, error) {
	f, err := os.OpenFile(path, openFlags, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open hooks lock: %w", err)
	}

	deadline := time.Now().Add(bound)
	for {
		err := unix.Flock(int(f.Fd()), flockMode|unix.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			_ = f.Close()
			return nil, fmt.Errorf("flock hooks lock %s: %w", path, err)
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, fmt.Errorf("%w: %s", ErrLockHeld, path)
		}
		time.Sleep(lockPollInterval)
	}
}

// acquireMutationLock takes the exclusive hold a whole load-mutate-save runs
// under. The config directory is created first because the sidecar cannot be
// created inside a directory that does not exist; its error is deliberately
// discarded so an uncreatable directory fails through the acquire below, on the
// one branch that reports a lock failure.
func (s *Store) acquireMutationLock() (*os.File, error) {
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	return acquireLock(s.lockPath(), os.O_RDWR|os.O_CREATE, unix.LOCK_EX, lockTimeout)
}

// acquireSharedLock takes the shared hold a read runs under, at the bound its
// caller supplies. It passes no O_CREATE deliberately: a read must create
// nothing — not the config directory, not the sidecar — so a read-only `portal
// doctor` and a display-only `hook list` keep having no write side effect on a
// fresh install, and an absent sidecar degrades through the caller instead.
func (s *Store) acquireSharedLock(bound time.Duration) (*os.File, error) {
	return acquireLock(s.lockPath(), os.O_RDONLY, unix.LOCK_SH, bound)
}
