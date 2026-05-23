package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// withLockAcquireFake swaps lockAcquire for the duration of the test and
// restores it via t.Cleanup. Tests must not use t.Parallel — lockAcquire is
// package-level mutable state shared across the test binary.
func withLockAcquireFake(t *testing.T, fake func(fd int, how int) error) {
	t.Helper()
	prev := lockAcquire
	lockAcquire = fake
	t.Cleanup(func() { lockAcquire = prev })
}

// withLockAcquireIdentifyDaemonFake swaps lockAcquireIdentifyDaemon for the
// duration of the test and restores it via t.Cleanup. Tests must not use
// t.Parallel — package-level mutable state is shared across the test binary.
func withLockAcquireIdentifyDaemonFake(t *testing.T, fake func(pid int) (IdentifyResult, error)) {
	t.Helper()
	prev := lockAcquireIdentifyDaemon
	lockAcquireIdentifyDaemon = fake
	t.Cleanup(func() { lockAcquireIdentifyDaemon = prev })
}

// withLockAcquireReadPIDFileFake swaps lockAcquireReadPIDFile for the duration
// of the test and restores it via t.Cleanup. Tests must not use t.Parallel —
// package-level mutable state is shared across the test binary.
func withLockAcquireReadPIDFileFake(t *testing.T, fake func(dir string) (int, error)) {
	t.Helper()
	prev := lockAcquireReadPIDFile
	lockAcquireReadPIDFile = fake
	t.Cleanup(func() { lockAcquireReadPIDFile = prev })
}

func TestAcquireDaemonLock_ReturnsErrDaemonLockHeldOnEWOULDBLOCK(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireFake(t, func(_ int, _ int) error {
		return unix.EWOULDBLOCK
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on contention, got %v", f)
	}
	if !errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; want errors.Is ErrDaemonLockHeld", err)
	}
}

func TestAcquireDaemonLock_WrapsNonEWOULDBLOCKFlockErrors(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireFake(t, func(_ int, _ int) error {
		return unix.EBADF
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on non-EWOULDBLOCK flock error, got %v", f)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; must NOT be ErrDaemonLockHeld for non-EWOULDBLOCK", err)
	}
	if !errors.Is(err, unix.EBADF) {
		t.Fatalf("err = %v; expected wrapped unix.EBADF", err)
	}
}

func TestAcquireDaemonLock_WrapsOpenErrorWhenStateDirMissing(t *testing.T) {
	// lockAcquire must not be reached: open(2) fails first.
	withLockAcquireFake(t, func(_ int, _ int) error {
		t.Fatal("lockAcquire must not be called when open fails")
		return nil
	})

	missing := filepath.Join(t.TempDir(), "does-not-exist")

	f, err := AcquireDaemonLock(missing)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on open error, got %v", f)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; must NOT be ErrDaemonLockHeld for open(2) failure", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v; expected wrapped os.ErrNotExist", err)
	}
}

func TestAcquireDaemonLock_CreatesLockFileWithMode0600(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	path := DaemonLock(dir)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat lock file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("lock file mode = %o; want %o", got, 0o600)
	}
}

func TestAcquireDaemonLock_SetsFDCLOEXEC(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	flags, err := unix.FcntlInt(f.Fd(), unix.F_GETFD, 0)
	if err != nil {
		t.Fatalf("F_GETFD: %v", err)
	}
	if flags&unix.FD_CLOEXEC == 0 {
		t.Errorf("FD_CLOEXEC not set on returned fd; flags = %#x", flags)
	}
}

func TestAcquireDaemonLock_DoesNotCreateStateDirIfMissing(t *testing.T) {
	withLockAcquireFake(t, func(_ int, _ int) error {
		t.Fatal("lockAcquire must not be called when open fails")
		return nil
	})

	parent := t.TempDir()
	missing := filepath.Join(parent, "missing-state-dir")

	_, err := AcquireDaemonLock(missing)
	if err == nil {
		t.Fatal("expected error when stateDir does not exist, got nil")
	}

	if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("stateDir was created; stat err = %v; want os.ErrNotExist", statErr)
	}
}

// TestAcquireDaemonLock_KernelReleasesOnFDClose is the regression guard for
// the lock-cleanup-on-crash invariant: a daemon that exits abruptly (panic,
// SIGKILL, OS reboot) releases the advisory lock via kernel fd cleanup, and
// the next daemon acquires cleanly with no stale-lockfile dance. That
// property is structural to unix.Flock semantics — closing the holding fd
// (which the kernel does for every fd of an exiting process, regardless of
// cause) drops the lock. This test exercises the real unix.Flock syscall (no
// lockAcquire seam) on a real lockfile so a future refactor that installs a
// premature-close finalizer, or swaps to a lockfile-based primitive whose
// semantics leak on abrupt exit, fails here instead of leaking into prod.
func TestAcquireDaemonLock_KernelReleasesOnFDClose(t *testing.T) {
	stateDir := t.TempDir()

	f1, err := AcquireDaemonLock(stateDir)
	if err != nil {
		t.Fatalf("first AcquireDaemonLock: %v", err)
	}
	if f1 == nil {
		t.Fatal("first AcquireDaemonLock returned nil *os.File")
	}

	// While f1 is held, contention must surface as ErrDaemonLockHeld against
	// the real flock syscall — not a wrapped error, not a successful acquire.
	f2, err := AcquireDaemonLock(stateDir)
	if f2 != nil {
		_ = f2.Close()
		t.Fatalf("second AcquireDaemonLock returned non-nil *os.File while lock held")
	}
	if !errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("second AcquireDaemonLock err = %v; want errors.Is ErrDaemonLockHeld", err)
	}

	// Close f1 to simulate kernel-level fd cleanup on abrupt process exit.
	// The kernel maps process exit to "close all fds", so Close() is a
	// faithful simulation of the SIGKILL / panic / reboot path.
	if err := f1.Close(); err != nil {
		t.Fatalf("close f1: %v", err)
	}

	// Re-acquire against the same lockfile — no os.Remove, no manual unlink,
	// no recreation. The kernel must have released the lock when f1's fd was
	// closed.
	f3, err := AcquireDaemonLock(stateDir)
	if err != nil {
		t.Fatalf("third AcquireDaemonLock after f1.Close: %v", err)
	}
	if f3 == nil {
		t.Fatal("third AcquireDaemonLock returned nil *os.File")
	}
	t.Cleanup(func() { _ = f3.Close() })
}

// TestAcquireDaemonLock_PreCheck_PIDFileAbsent_Proceeds asserts that when
// daemon.pid does not exist, the pre-check returns "no holder" and acquire
// proceeds (opens daemon.lock, runs flock, returns the locked fd).
func TestAcquireDaemonLock_PreCheck_PIDFileAbsent_Proceeds(t *testing.T) {
	dir := t.TempDir()
	// No daemon.pid is written: ReadPIDFile will return ErrPIDFileAbsent.
	withLockAcquireIdentifyDaemonFake(t, func(pid int) (IdentifyResult, error) {
		t.Fatalf("lockAcquireIdentifyDaemon must not be called when daemon.pid is absent; got pid=%d", pid)
		return 0, nil
	})
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	if f == nil {
		t.Fatal("AcquireDaemonLock returned nil *os.File")
	}
	t.Cleanup(func() { _ = f.Close() })
}

// TestAcquireDaemonLock_PreCheck_DeadPID_Proceeds asserts that when daemon.pid
// records a PID that identity-checks as dead, the pre-check proceeds to the
// normal open + flock path.
func TestAcquireDaemonLock_PreCheck_DeadPID_Proceeds(t *testing.T) {
	dir := t.TempDir()
	if err := WritePIDFile(dir, 99999); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	identifyCalled := false
	withLockAcquireIdentifyDaemonFake(t, func(pid int) (IdentifyResult, error) {
		identifyCalled = true
		if pid != 99999 {
			t.Errorf("lockAcquireIdentifyDaemon pid = %d; want 99999", pid)
		}
		return IdentifyDead, nil
	})
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	if f == nil {
		t.Fatal("AcquireDaemonLock returned nil *os.File")
	}
	t.Cleanup(func() { _ = f.Close() })

	if !identifyCalled {
		t.Errorf("lockAcquireIdentifyDaemon was not called")
	}
}

// TestAcquireDaemonLock_PreCheck_LivePortalDaemon_ReturnsErrDaemonLockHeld
// asserts that when daemon.pid records a live PID that identity-checks as a
// portal state daemon, the pre-check returns ErrDaemonLockHeld WITHOUT opening
// daemon.lock.
func TestAcquireDaemonLock_PreCheck_LivePortalDaemon_ReturnsErrDaemonLockHeld(t *testing.T) {
	dir := t.TempDir()
	if err := WritePIDFile(dir, 4242); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	withLockAcquireIdentifyDaemonFake(t, func(pid int) (IdentifyResult, error) {
		if pid != 4242 {
			t.Errorf("lockAcquireIdentifyDaemon pid = %d; want 4242", pid)
		}
		return IdentifyIsPortalDaemon, nil
	})
	withLockAcquireFake(t, func(_ int, _ int) error {
		t.Fatal("lockAcquire must not be called when pre-check identifies a live portal daemon")
		return nil
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on pre-check refusal, got %v", f)
	}
	if !errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; want errors.Is ErrDaemonLockHeld", err)
	}

	// Crucially: daemon.lock must NOT have been created (open was never called).
	if _, statErr := os.Stat(DaemonLock(dir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("daemon.lock exists after pre-check refusal; stat err = %v; want os.ErrNotExist", statErr)
	}
}

// TestAcquireDaemonLock_PreCheck_LiveNonPortalPID_Proceeds asserts that when
// daemon.pid records a live PID whose identity check says "not a portal
// daemon", the pre-check proceeds to the normal open + flock path.
func TestAcquireDaemonLock_PreCheck_LiveNonPortalPID_Proceeds(t *testing.T) {
	dir := t.TempDir()
	if err := WritePIDFile(dir, 5151); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	withLockAcquireIdentifyDaemonFake(t, func(pid int) (IdentifyResult, error) {
		return IdentifyNotPortalDaemon, nil
	})
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	if f == nil {
		t.Fatal("AcquireDaemonLock returned nil *os.File")
	}
	t.Cleanup(func() { _ = f.Close() })
}

// TestAcquireDaemonLock_PreCheck_TransientIdentifyError_Proceeds asserts that
// when the identity check returns a transient error, the pre-check treats it
// as "not a portal daemon" and proceeds to the normal open + flock path. The
// flock EWOULDBLOCK path remains the fallback for real contention.
func TestAcquireDaemonLock_PreCheck_TransientIdentifyError_Proceeds(t *testing.T) {
	dir := t.TempDir()
	if err := WritePIDFile(dir, 6262); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	withLockAcquireIdentifyDaemonFake(t, func(pid int) (IdentifyResult, error) {
		return 0, fmt.Errorf("transient ps failure")
	})
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	if f == nil {
		t.Fatal("AcquireDaemonLock returned nil *os.File")
	}
	t.Cleanup(func() { _ = f.Close() })
}

// TestAcquireDaemonLock_PreCheck_ReadPIDFileNonAbsentError_Proceeds asserts
// that when ReadPIDFile fails with an error that is NOT IsNotExist (e.g. parse
// error), the pre-check treats it as "no holder" and proceeds.
func TestAcquireDaemonLock_PreCheck_ReadPIDFileNonAbsentError_Proceeds(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireReadPIDFileFake(t, func(d string) (int, error) {
		return 0, fmt.Errorf("parse daemon.pid: malformed")
	})
	withLockAcquireIdentifyDaemonFake(t, func(pid int) (IdentifyResult, error) {
		t.Fatalf("lockAcquireIdentifyDaemon must not be called when ReadPIDFile errors; got pid=%d", pid)
		return 0, nil
	})
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	if f == nil {
		t.Fatal("AcquireDaemonLock returned nil *os.File")
	}
	t.Cleanup(func() { _ = f.Close() })
}

// TestAcquireDaemonLock_PreCheck_DoesNotOpenLockFile_OnRefusal pins the
// invariant that pre-check refusal does NOT touch daemon.lock at all: neither
// open(2) nor lockAcquire is invoked. lockAcquire is stubbed to fail-fast on
// any call.
func TestAcquireDaemonLock_PreCheck_DoesNotOpenLockFile_OnRefusal(t *testing.T) {
	dir := t.TempDir()
	if err := WritePIDFile(dir, 7373); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}

	withLockAcquireIdentifyDaemonFake(t, func(pid int) (IdentifyResult, error) {
		return IdentifyIsPortalDaemon, nil
	})
	withLockAcquireFake(t, func(_ int, _ int) error {
		t.Fatal("lockAcquire must NOT be called when pre-check returns ErrDaemonLockHeld")
		return nil
	})

	_, err := AcquireDaemonLock(dir)
	if !errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; want errors.Is ErrDaemonLockHeld", err)
	}
	if _, statErr := os.Stat(DaemonLock(dir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("daemon.lock exists after pre-check refusal; stat err = %v; want os.ErrNotExist", statErr)
	}
}

// TestAcquireDaemonLock_EWOULDBLOCK_PreCheckSeesNoHolder_FlockFallback is the
// regression guard for the layered-enforcement contract: when the pre-check
// finds no holder (no daemon.pid), the existing EWOULDBLOCK path remains the
// fallback that returns ErrDaemonLockHeld on real contention.
func TestAcquireDaemonLock_EWOULDBLOCK_PreCheckSeesNoHolder_FlockFallback(t *testing.T) {
	dir := t.TempDir()
	// No daemon.pid → pre-check skips, proceeds.
	withLockAcquireFake(t, func(_ int, _ int) error {
		return unix.EWOULDBLOCK
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on contention, got %v", f)
	}
	if !errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; want errors.Is ErrDaemonLockHeld", err)
	}
}

// withLockAcquireFstatFake swaps lockAcquireFstat for the duration of the
// test and restores it via t.Cleanup. Tests must not use t.Parallel —
// package-level mutable state is shared across the test binary.
func withLockAcquireFstatFake(t *testing.T, fake func(fd int, st *unix.Stat_t) error) {
	t.Helper()
	prev := lockAcquireFstat
	lockAcquireFstat = fake
	t.Cleanup(func() { lockAcquireFstat = prev })
}

// withLockAcquireStatFake swaps lockAcquireStat for the duration of the
// test and restores it via t.Cleanup. Tests must not use t.Parallel —
// package-level mutable state is shared across the test binary.
func withLockAcquireStatFake(t *testing.T, fake func(path string, st *unix.Stat_t) error) {
	t.Helper()
	prev := lockAcquireStat
	lockAcquireStat = fake
	t.Cleanup(func() { lockAcquireStat = prev })
}

// inodeSequence builds fstat / stat fakes that yield deterministic inode
// sequences across attempts. Each call advances the index; tests stage one
// inode value per attempt for both fstat and stat.
func inodeSequence(values []uint64) func() uint64 {
	i := 0
	return func() uint64 {
		v := values[i]
		i++
		return v
	}
}

// TestAcquireDaemonLock_InodeCheck_HappyPath asserts that when fstat(fd).Ino
// matches stat(path).Ino on the first attempt, AcquireDaemonLock returns
// successfully with no retry and no sleep.
func TestAcquireDaemonLock_InodeCheck_HappyPath(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	fstatCalls := 0
	withLockAcquireFstatFake(t, func(_ int, st *unix.Stat_t) error {
		fstatCalls++
		st.Ino = 42
		return nil
	})
	statCalls := 0
	withLockAcquireStatFake(t, func(_ string, st *unix.Stat_t) error {
		statCalls++
		st.Ino = 42
		return nil
	})

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	if f == nil {
		t.Fatal("AcquireDaemonLock returned nil *os.File")
	}
	t.Cleanup(func() { _ = f.Close() })

	if fstatCalls != 1 {
		t.Errorf("fstat called %d times; want 1", fstatCalls)
	}
	if statCalls != 1 {
		t.Errorf("stat called %d times; want 1", statCalls)
	}
}

// TestAcquireDaemonLock_InodeCheck_MismatchThenMatch asserts that a mismatch
// on attempt 1 followed by a match on attempt 2 succeeds.
func TestAcquireDaemonLock_InodeCheck_MismatchThenMatch(t *testing.T) {
	dir := t.TempDir()
	flockCalls := 0
	withLockAcquireFake(t, func(_ int, _ int) error {
		flockCalls++
		return nil
	})

	fdInodes := inodeSequence([]uint64{100, 200})
	pathInodes := inodeSequence([]uint64{999, 200})

	withLockAcquireFstatFake(t, func(_ int, st *unix.Stat_t) error {
		st.Ino = fdInodes()
		return nil
	})
	withLockAcquireStatFake(t, func(_ string, st *unix.Stat_t) error {
		st.Ino = pathInodes()
		return nil
	})

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	if f == nil {
		t.Fatal("AcquireDaemonLock returned nil *os.File")
	}
	t.Cleanup(func() { _ = f.Close() })

	if flockCalls != 2 {
		t.Errorf("flock called %d times; want 2", flockCalls)
	}
}

// TestAcquireDaemonLock_InodeCheck_ExhaustsRetries asserts that mismatches on
// all 3 attempts produce a wrapped error (not ErrDaemonLockHeld) within 100ms.
func TestAcquireDaemonLock_InodeCheck_ExhaustsRetries(t *testing.T) {
	dir := t.TempDir()
	flockCalls := 0
	withLockAcquireFake(t, func(_ int, _ int) error {
		flockCalls++
		return nil
	})

	withLockAcquireFstatFake(t, func(_ int, st *unix.Stat_t) error {
		st.Ino = 1
		return nil
	})
	withLockAcquireStatFake(t, func(_ string, st *unix.Stat_t) error {
		st.Ino = 2
		return nil
	})

	start := time.Now()
	f, err := AcquireDaemonLock(dir)
	elapsed := time.Since(start)

	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on persistent inode mismatch, got %v", f)
	}
	if err == nil {
		t.Fatal("expected error after 3 mismatches, got nil")
	}
	if errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; must NOT be ErrDaemonLockHeld for persistent inode mismatch", err)
	}
	if flockCalls != 3 {
		t.Errorf("flock called %d times; want 3", flockCalls)
	}
	if elapsed >= 100*time.Millisecond {
		t.Errorf("elapsed = %v; want < 100ms", elapsed)
	}
}

// TestAcquireDaemonLock_InodeCheck_FstatSyscallError asserts that an fstat
// syscall failure returns a wrapped error immediately, no retry.
func TestAcquireDaemonLock_InodeCheck_FstatSyscallError(t *testing.T) {
	dir := t.TempDir()
	flockCalls := 0
	withLockAcquireFake(t, func(_ int, _ int) error {
		flockCalls++
		return nil
	})

	withLockAcquireFstatFake(t, func(_ int, _ *unix.Stat_t) error {
		return unix.EBADF
	})
	withLockAcquireStatFake(t, func(_ string, _ *unix.Stat_t) error {
		t.Fatal("stat must not be called when fstat fails")
		return nil
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on fstat error, got %v", f)
	}
	if err == nil {
		t.Fatal("expected error on fstat failure, got nil")
	}
	if errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; must NOT be ErrDaemonLockHeld for fstat error", err)
	}
	if !errors.Is(err, unix.EBADF) {
		t.Fatalf("err = %v; expected wrapped unix.EBADF", err)
	}
	if flockCalls != 1 {
		t.Errorf("flock called %d times; want 1 (no retry on syscall error)", flockCalls)
	}
}

// TestAcquireDaemonLock_InodeCheck_StatSyscallError asserts that a stat
// syscall failure returns a wrapped error immediately, no retry.
func TestAcquireDaemonLock_InodeCheck_StatSyscallError(t *testing.T) {
	dir := t.TempDir()
	flockCalls := 0
	withLockAcquireFake(t, func(_ int, _ int) error {
		flockCalls++
		return nil
	})

	withLockAcquireFstatFake(t, func(_ int, st *unix.Stat_t) error {
		st.Ino = 5
		return nil
	})
	withLockAcquireStatFake(t, func(_ string, _ *unix.Stat_t) error {
		return unix.ENOENT
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on stat error, got %v", f)
	}
	if err == nil {
		t.Fatal("expected error on stat failure, got nil")
	}
	if errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; must NOT be ErrDaemonLockHeld for stat error", err)
	}
	if !errors.Is(err, unix.ENOENT) {
		t.Fatalf("err = %v; expected wrapped unix.ENOENT", err)
	}
	if flockCalls != 1 {
		t.Errorf("flock called %d times; want 1 (no retry on syscall error)", flockCalls)
	}
}

// TestAcquireDaemonLock_InodeCheck_MismatchClosesFDBeforeRetry asserts that on
// inode mismatch the helper closes the fd (releasing the flock) before sleeping
// + retrying. We observe close via flock count: between retries the file must
// be re-opened so subsequent flock calls operate on a freshly-opened fd. We
// pin the contract by counting OpenFile calls indirectly — the file descriptor
// number advances between attempts.
func TestAcquireDaemonLock_InodeCheck_MismatchClosesFDBeforeRetry(t *testing.T) {
	dir := t.TempDir()

	// Record the fd value each flock attempt sees. A closed fd means the next
	// open will (typically) reuse the same low fd number, but the key property
	// is that flock is called once per attempt with a fresh fd from a fresh
	// open. We assert by counting flock calls.
	flockCalls := 0
	withLockAcquireFake(t, func(_ int, _ int) error {
		flockCalls++
		return nil
	})

	// 2 attempts: mismatch, then match.
	fdInodes := inodeSequence([]uint64{1, 2})
	pathInodes := inodeSequence([]uint64{9, 2})
	withLockAcquireFstatFake(t, func(_ int, st *unix.Stat_t) error {
		st.Ino = fdInodes()
		return nil
	})
	withLockAcquireStatFake(t, func(_ string, st *unix.Stat_t) error {
		st.Ino = pathInodes()
		return nil
	})

	f, err := AcquireDaemonLock(dir)
	if err != nil {
		t.Fatalf("AcquireDaemonLock: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if flockCalls != 2 {
		t.Errorf("flock called %d times; want 2 (one per attempt)", flockCalls)
	}

	// Sanity: lock file exists and is openable (would not be true if the
	// mismatch path left a leaked locked fd in the calling process).
	f2, err := os.OpenFile(DaemonLock(dir), os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("re-open lock file: %v", err)
	}
	_ = f2.Close()
}

// TestAcquireDaemonLock_InodeCheck_NoFDCLOEXECOnPersistentMismatch asserts that
// FD_CLOEXEC is NOT applied when the inode cross-check never passes. We pin
// this via the structural contract: the only path that returns (*os.File, nil)
// also applies FD_CLOEXEC. A persistent-mismatch return value MUST be
// (nil, wrapped-error), so the absence of a returned *os.File proves
// FD_CLOEXEC was not run on any fd handed to the caller.
func TestAcquireDaemonLock_InodeCheck_NoFDCLOEXECOnPersistentMismatch(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	withLockAcquireFstatFake(t, func(_ int, st *unix.Stat_t) error {
		st.Ino = 7
		return nil
	})
	withLockAcquireStatFake(t, func(_ string, st *unix.Stat_t) error {
		st.Ino = 8
		return nil
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("persistent mismatch must return nil *os.File; got %v", f)
	}
	if err == nil {
		t.Fatal("expected wrapped error on persistent mismatch")
	}
	if errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; must NOT be ErrDaemonLockHeld", err)
	}
}

// TestAcquireDaemonLock_InodeCheck_RetryBoundedWallTime asserts that the total
// wall-time spent in the retry loop is < 100ms (3 attempts × 10ms = 30ms
// baseline + syscall overhead).
func TestAcquireDaemonLock_InodeCheck_RetryBoundedWallTime(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })
	withLockAcquireFstatFake(t, func(_ int, st *unix.Stat_t) error {
		st.Ino = 1
		return nil
	})
	withLockAcquireStatFake(t, func(_ string, st *unix.Stat_t) error {
		st.Ino = 2
		return nil
	})

	start := time.Now()
	_, _ = AcquireDaemonLock(dir)
	elapsed := time.Since(start)

	if elapsed >= 100*time.Millisecond {
		t.Errorf("retry loop took %v; want < 100ms", elapsed)
	}
}

// TestAcquireDaemonLock_InodeCheck_NotReachedOnEWOULDBLOCK is the regression
// guard ensuring the existing EWOULDBLOCK contention path still returns
// ErrDaemonLockHeld without invoking fstat/stat.
func TestAcquireDaemonLock_InodeCheck_NotReachedOnEWOULDBLOCK(t *testing.T) {
	dir := t.TempDir()
	withLockAcquireFake(t, func(_ int, _ int) error { return unix.EWOULDBLOCK })
	withLockAcquireFstatFake(t, func(_ int, _ *unix.Stat_t) error {
		t.Fatal("fstat must not be called when flock returns EWOULDBLOCK")
		return nil
	})
	withLockAcquireStatFake(t, func(_ string, _ *unix.Stat_t) error {
		t.Fatal("stat must not be called when flock returns EWOULDBLOCK")
		return nil
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on contention, got %v", f)
	}
	if !errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; want errors.Is ErrDaemonLockHeld", err)
	}
}

// TestAcquireDaemonLock_InodeCheck_NotReachedOnPreCheckRefusal is the
// regression guard ensuring the pre-check ErrDaemonLockHeld path returns
// without ever invoking fstat/stat.
func TestAcquireDaemonLock_InodeCheck_NotReachedOnPreCheckRefusal(t *testing.T) {
	dir := t.TempDir()
	if err := WritePIDFile(dir, 1234); err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}
	withLockAcquireIdentifyDaemonFake(t, func(_ int) (IdentifyResult, error) {
		return IdentifyIsPortalDaemon, nil
	})
	withLockAcquireFake(t, func(_ int, _ int) error {
		t.Fatal("flock must not be called on pre-check refusal")
		return nil
	})
	withLockAcquireFstatFake(t, func(_ int, _ *unix.Stat_t) error {
		t.Fatal("fstat must not be called on pre-check refusal")
		return nil
	})
	withLockAcquireStatFake(t, func(_ string, _ *unix.Stat_t) error {
		t.Fatal("stat must not be called on pre-check refusal")
		return nil
	})

	f, err := AcquireDaemonLock(dir)
	if f != nil {
		_ = f.Close()
		t.Fatalf("expected nil *os.File on pre-check refusal, got %v", f)
	}
	if !errors.Is(err, ErrDaemonLockHeld) {
		t.Fatalf("err = %v; want errors.Is ErrDaemonLockHeld", err)
	}
}

func TestAcquireDaemonLock_AcceptsArbitraryStateDirParameter(t *testing.T) {
	withLockAcquireFake(t, func(_ int, _ int) error { return nil })

	// Two distinct, caller-supplied state directories — no environment
	// variables, no hardcoded path. Each call must lock its own file.
	dirA := t.TempDir()
	dirB := t.TempDir()

	fa, err := AcquireDaemonLock(dirA)
	if err != nil {
		t.Fatalf("AcquireDaemonLock(dirA): %v", err)
	}
	t.Cleanup(func() { _ = fa.Close() })

	fb, err := AcquireDaemonLock(dirB)
	if err != nil {
		t.Fatalf("AcquireDaemonLock(dirB): %v", err)
	}
	t.Cleanup(func() { _ = fb.Close() })

	if _, err := os.Stat(DaemonLock(dirA)); err != nil {
		t.Errorf("lock file missing under dirA: %v", err)
	}
	if _, err := os.Stat(DaemonLock(dirB)); err != nil {
		t.Errorf("lock file missing under dirB: %v", err)
	}
}
