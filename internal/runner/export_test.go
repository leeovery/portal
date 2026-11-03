package runner

import (
	"testing"
	"time"
)

// SetGrace bounds each of the runner's waits at d for the rest of t, so tests
// of what happens past one need not wait out the real grace.
func SetGrace(t testing.TB, d time.Duration) {
	t.Helper()
	old := grace
	grace = d
	t.Cleanup(func() { grace = old })
}
