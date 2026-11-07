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

// IndexOf is an index holding entries as read, for a test to stage one
// without a file.
func IndexOf(entries []Entry) Index {
	return Index{sessions: entries}
}
