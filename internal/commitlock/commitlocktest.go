package commitlock

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// SetTimeoutForTest lowers the acquire bound for the duration of the test, so
// a test can drive a timeout without waiting out the production figure.
func SetTimeoutForTest(t *testing.T, d time.Duration) {
	t.Helper()
	prev := timeout
	timeout = d
	t.Cleanup(func() { timeout = prev })
}

// HoldForTest takes the commit lock in dir through an open file description
// of its own, as another process would, until the returned file is closed or
// the test ends.
func HoldForTest(t *testing.T, dir string) *os.File {
	t.Helper()
	f, err := os.OpenFile(Path(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open commit lock: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		t.Fatalf("flock commit lock: %v", err)
	}
	return f
}
