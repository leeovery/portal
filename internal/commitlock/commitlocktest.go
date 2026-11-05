package commitlock

import (
	"os"
	"testing"
	"time"
)

// SetTimeoutForTest lowers the acquire bound for the duration of the test, so
// a test can drive a timeout without waiting out the production figure.
func SetTimeoutForTest(t *testing.T, d time.Duration) {
	t.Helper()
	prev := timeout
	timeout = d
	t.Cleanup(func() { timeout = prev })
}

// holdBound is how long HoldForTest waits out a holder: longer than any
// commit holds the lock, a running daemon's included.
const holdBound = 15 * time.Second

// HoldForTest takes the commit lock in dir through an open file description
// of its own, as another process would, until the returned file is closed or
// the test ends. A process committing in dir, such as a saver daemon the test
// runs, holds the lock now and then, so it waits that out.
func HoldForTest(t *testing.T, dir string) *os.File {
	t.Helper()
	f, err := lock(Path(dir), holdBound)
	if err != nil {
		t.Fatalf("hold commit lock: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
