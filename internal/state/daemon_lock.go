package state

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// ErrDaemonLockHeld is returned by AcquireDaemonLock when another process
// already holds the advisory lock on <stateDir>/daemon.lock. Callers use
// errors.Is to distinguish this expected contention path (loser exits 0,
// single WARN line) from genuine open(2) / flock failures, which are wrapped
// plain errors and must be treated as fatal misconfiguration.
var ErrDaemonLockHeld = errors.New("daemon.lock held by another process")

// lockAcquire is the test seam over unix.Flock. Production code uses
// unix.Flock unchanged; tests in this package swap lockAcquire to simulate
// EWOULDBLOCK / other flock errors without contending for a real OS lock.
//
// The seam shape mirrors the existing daemonRunFunc / daemonShutdownFunc
// pattern documented in the spec (§ Fix Part 1: "should be seamed for testing
// via a package-level var lockAcquire = unix.Flock").
var lockAcquire = unix.Flock

// AcquireDaemonLock opens <stateDir>/daemon.lock and attempts to acquire an
// exclusive, non-blocking advisory lock on it via unix.Flock. It is the
// daemon-side singleton primitive: at most one process can hold the lock for
// a given state directory at any time.
//
// Behaviour:
//   - On success the returned *os.File holds the locked fd. The fd has
//     FD_CLOEXEC set so it does not leak into child processes the daemon
//     forks. The caller MUST retain the returned *os.File in a variable that
//     lives for the lifetime of the daemon process (e.g. a package-level
//     var). Letting the *os.File go out of scope allows Go's finalizer to
//     close the fd, which releases the kernel-side flock and silently
//     re-introduces the race the lock exists to close.
//   - On EWOULDBLOCK (another process holds the lock) the helper closes the
//     fd and returns ErrDaemonLockHeld. Callers distinguish this via
//     errors.Is so the daemon-startup path can log a single WARN line and
//     exit status 0.
//   - On any other flock error the helper closes the fd and returns a
//     wrapped error.
//   - On open(2) failure (e.g. ENOENT because stateDir does not exist,
//     EACCES, EMFILE) the helper returns a wrapped error. It deliberately
//     does NOT MkdirAll: state-directory existence is the caller's
//     pre-existing responsibility.
//   - On FD_CLOEXEC fcntl failure the helper closes the fd and returns a
//     wrapped error.
//
// The lock file is created with mode 0600 to match the file mode of other
// portal state files.
func AcquireDaemonLock(stateDir string) (*os.File, error) {
	path := DaemonLock(stateDir)

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon.lock %s: %w", path, err)
	}

	if err := lockAcquire(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrDaemonLockHeld
		}
		return nil, fmt.Errorf("flock daemon.lock %s: %w", path, err)
	}

	if _, err := unix.FcntlInt(f.Fd(), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("set FD_CLOEXEC on daemon.lock %s: %w", path, err)
	}

	return f, nil
}
