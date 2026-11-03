package runner

import (
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// SetGrace bounds each of the runner's waits at d for the rest of t, so tests
// of what happens past one need not wait out the real grace.
func SetGrace(t testing.TB, d time.Duration) {
	t.Helper()
	old := grace
	grace = d
	t.Cleanup(func() { grace = old })
}

// SameDevice is the check that a terminal opened by path is the one Run was
// given.
func SameDevice(a, b int) bool { return sameDevice(a, b) }

// SameNode is the comparison under SameDevice.
func SameNode(a, b *unix.Stat_t) bool { return sameNode(a, b) }

// AbandonCatching abandons an attach that caught sig before it failed to
// begin, as start does.
func AbandonCatching(sig os.Signal) {
	a := newAttach(Config{})
	_ = a.catchSignals()
	a.signals <- sig
	a.abandon()
}
