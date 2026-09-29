package state

import (
	"testing"
	"time"
)

// SetCommitLockTimeoutForTest lowers the commit lock's acquire bound for the
// duration of the test, so a test can drive a timeout without waiting out the
// production figure.
func SetCommitLockTimeoutForTest(t *testing.T, d time.Duration) {
	t.Helper()
	prev := commitLockTimeout
	commitLockTimeout = d
	t.Cleanup(func() { commitLockTimeout = prev })
}
