package tmuxtest

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestPollUntil_ReturnsTrueWhenCondBecomesTrueBeforeTimeout exercises
// the success path: cond flips to true after a handful of ticks and
// PollUntil returns true well inside the configured timeout.
func TestPollUntil_ReturnsTrueWhenCondBecomesTrueBeforeTimeout(t *testing.T) {
	var calls int32
	cond := func() bool {
		// Flip true on the third invocation so the loop must tick at
		// least twice before observing success — this rules out a
		// false positive where the helper happens to short-circuit on
		// the first iteration.
		return atomic.AddInt32(&calls, 1) >= 3
	}
	start := time.Now()
	got := PollUntil(t, 500*time.Millisecond, 10*time.Millisecond, cond)
	elapsed := time.Since(start)
	if !got {
		t.Fatalf("PollUntil returned false; want true (calls=%d, elapsed=%s)",
			atomic.LoadInt32(&calls), elapsed)
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("PollUntil took %s; want < timeout (500ms)", elapsed)
	}
}

// TestPollUntil_ReturnsFalseWhenTimeoutElapsesWithCondNeverTrue
// exercises the timeout path: cond is constant-false so PollUntil must
// return false after at least the configured timeout has elapsed.
func TestPollUntil_ReturnsFalseWhenTimeoutElapsesWithCondNeverTrue(t *testing.T) {
	cond := func() bool { return false }
	start := time.Now()
	got := PollUntil(t, 80*time.Millisecond, 10*time.Millisecond, cond)
	elapsed := time.Since(start)
	if got {
		t.Fatalf("PollUntil returned true; want false")
	}
	if elapsed < 80*time.Millisecond {
		t.Fatalf("PollUntil returned after %s; want >= timeout (80ms)", elapsed)
	}
}
