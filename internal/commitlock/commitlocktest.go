package commitlock

import (
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
