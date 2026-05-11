package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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
