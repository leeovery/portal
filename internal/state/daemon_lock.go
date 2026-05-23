package state

import (
	"errors"
	"fmt"
	"os"
	"time"

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

// lockAcquireReadPIDFile is the test seam over ReadPIDFile used by the
// pre-acquire daemon.pid liveness check. Production code uses ReadPIDFile
// unchanged; tests swap this seam to stage canned daemon.pid read shapes
// (absent, parse error, etc.) without touching the filesystem.
var lockAcquireReadPIDFile = ReadPIDFile

// lockAcquireIdentifyDaemon is the test seam over IdentifyDaemon used by the
// pre-acquire daemon.pid liveness check. Production code uses IdentifyDaemon
// unchanged; tests swap this seam to stage canned identity-check outcomes
// (live portal daemon, dead, not portal, transient error) without forking
// real processes.
var lockAcquireIdentifyDaemon = IdentifyDaemon

// lockAcquireFstat is the test seam over unix.Fstat used by the post-flock
// inode cross-check. Production code uses unix.Fstat unchanged; tests swap
// this seam to drive deterministic inode sequences across retry attempts.
var lockAcquireFstat = unix.Fstat

// lockAcquireStat is the test seam over unix.Stat used by the post-flock
// inode cross-check. Production code uses unix.Stat unchanged; tests swap
// this seam to drive deterministic inode sequences across retry attempts.
var lockAcquireStat = unix.Stat

// lockAcquireInodeRetryAttempts bounds the post-flock inode cross-check at 3
// attempts. After 3 mismatches the helper returns a wrapped error (NOT
// ErrDaemonLockHeld) which the daemon treats as ERROR-and-exit-status-1.
const lockAcquireInodeRetryAttempts = 3

// lockAcquireInodeRetrySleep is the fixed sleep between inode-mismatch retry
// attempts. 3 attempts × 10ms = ≤30ms baseline + syscall overhead < 100ms
// total. Fixed sleep; no jitter.
const lockAcquireInodeRetrySleep = 10 * time.Millisecond

// AcquireDaemonLock opens <stateDir>/daemon.lock and attempts to acquire an
// exclusive, non-blocking advisory lock on it via unix.Flock. It is the
// daemon-side singleton primitive: at most one process can hold the lock for
// a given state directory at any time.
//
// Behaviour:
//   - Pre-acquire daemon.pid liveness check (primary singleton enforcer).
//     Before opening daemon.lock, the helper reads <stateDir>/daemon.pid via
//     ReadPIDFile and, if a valid PID is recorded, identity-checks it via
//     IdentifyDaemon. If the recorded PID is alive AND identifies as a
//     `portal state daemon` (IdentifyIsPortalDaemon), the helper returns
//     ErrDaemonLockHeld IMMEDIATELY without opening daemon.lock — closing
//     the per-inode gap in flock semantics so an unlinked + recreated
//     daemon.lock cannot allow two daemons to flock different inodes
//     simultaneously. If daemon.pid is absent, the recorded PID is dead, the
//     PID is alive but is NOT a portal state daemon, or ReadPIDFile fails
//     with any error (including non-IsNotExist I/O or parse errors), the
//     pre-check treats this as "no holder" and proceeds to open + flock. If
//     the identity check returns a transient error (e.g. ps exec failure),
//     the helper also proceeds; rationale: the flock EWOULDBLOCK fallback
//     still catches real contention, and biasing toward "let legitimate
//     succession proceed" is safer than spuriously blocking startup.
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
//     exit status 0. ErrDaemonLockHeld is returned via either the pre-check
//     (when daemon.pid identifies a live portal daemon — no fd is opened) or
//     the flock EWOULDBLOCK fallback (when the pre-check finds no holder but
//     a concurrent daemon already holds the flock on the same inode). Both
//     paths are semantically equivalent for callers using errors.Is.
//   - On any other flock error the helper closes the fd and returns a
//     wrapped error.
//   - On open(2) failure (e.g. ENOENT because stateDir does not exist,
//     EACCES, EMFILE) the helper returns a wrapped error. It deliberately
//     does NOT MkdirAll: state-directory existence is the caller's
//     pre-existing responsibility.
//   - On FD_CLOEXEC fcntl failure the helper closes the fd and returns a
//     wrapped error.
//
// Layered enforcement: the pre-check is the primary singleton enforcer for
// steady-state contention (and the only enforcer that survives daemon.lock
// inode replacement). The flock EWOULDBLOCK path is the fallback enforcer
// covering the small startup window between AcquireDaemonLock returning and
// the caller's subsequent daemon.pid write.
//
// The lock file is created with mode 0600 to match the file mode of other
// portal state files.
func AcquireDaemonLock(stateDir string) (*os.File, error) {
	path := DaemonLock(stateDir)

	for attempt := 1; attempt <= lockAcquireInodeRetryAttempts; attempt++ {
		// Pre-check runs at the head of every attempt so a slow daemon that
		// wins the lock mid-retry is still detected on the next iteration.
		if preAcquirePIDIdentifiesLiveDaemon(stateDir) {
			return nil, ErrDaemonLockHeld
		}

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

		// Post-flock inode cross-check: ensure the inode we flock'd is still
		// the inode at the path. If the file was unlinked + recreated between
		// our open and our flock, the two diverge and a second daemon can
		// flock a different inode for the same path. Bounded retry handles
		// transient turbulence; persistent mismatch returns a wrapped error.
		var fdStat unix.Stat_t
		if err := lockAcquireFstat(int(f.Fd()), &fdStat); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("fstat daemon.lock %s: %w", path, err)
		}
		var pathStat unix.Stat_t
		if err := lockAcquireStat(path, &pathStat); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("stat daemon.lock %s: %w", path, err)
		}

		if fdStat.Ino != pathStat.Ino {
			// Release the flock by closing the fd, sleep, and retry the whole
			// acquire (pre-check + open + flock + inode check).
			_ = f.Close()
			if attempt < lockAcquireInodeRetryAttempts {
				time.Sleep(lockAcquireInodeRetrySleep)
				continue
			}
			return nil, fmt.Errorf("daemon.lock %s inode mismatch after %d attempts: fd inode != path inode", path, lockAcquireInodeRetryAttempts)
		}

		if _, err := unix.FcntlInt(f.Fd(), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("set FD_CLOEXEC on daemon.lock %s: %w", path, err)
		}

		return f, nil
	}

	// Unreachable: the loop body either returns or continues; the final
	// iteration's mismatch branch returns the bounded-retry error.
	return nil, fmt.Errorf("daemon.lock %s inode mismatch after %d attempts: fd inode != path inode", path, lockAcquireInodeRetryAttempts)
}

// preAcquirePIDIdentifiesLiveDaemon reports whether <stateDir>/daemon.pid
// records a PID that is alive AND identifies as a `portal state daemon`.
// This is the primary singleton enforcer in AcquireDaemonLock — see the
// AcquireDaemonLock docstring for the full layered-enforcement contract.
//
// All non-affirmative outcomes — absent daemon.pid, ReadPIDFile errors of any
// shape (including parse errors), recorded PID dead, recorded PID alive but
// not a portal daemon, and transient identity-check errors — return false so
// the caller proceeds to the existing open + flock path. Only an unambiguous
// "live portal daemon" identification (IdentifyIsPortalDaemon with nil error)
// returns true.
func preAcquirePIDIdentifiesLiveDaemon(stateDir string) bool {
	pid, err := lockAcquireReadPIDFile(stateDir)
	if err != nil {
		// Includes ErrPIDFileAbsent (no holder), parse errors, and any other
		// I/O error. The spec contract is "treat as no holder, proceed" for
		// every read failure shape — the flock EWOULDBLOCK fallback still
		// catches real contention.
		return false
	}

	result, idErr := lockAcquireIdentifyDaemon(pid)
	if idErr != nil {
		// Transient identity-check error: bias toward letting legitimate
		// succession proceed; the flock EWOULDBLOCK fallback is the safety
		// net for real contention.
		return false
	}
	return result == IdentifyIsPortalDaemon
}
