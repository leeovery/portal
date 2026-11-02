package sessionindex

import (
	"testing"
	"time"
)

// SetClockForTest fixes the clock Update stamps saved_at from for the
// duration of the test.
func SetClockForTest(t *testing.T, at time.Time) {
	t.Helper()
	prev := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = prev })
}
