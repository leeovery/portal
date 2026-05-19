package tmuxtest

import (
	"testing"
	"time"
)

// PollUntil repeatedly invokes cond at the given tick cadence until it
// returns true or the timeout elapses. Returns true on observed
// success, false on timeout.
//
// PollUntil does NOT call t.Fatal itself — the caller owns the
// failure-message shape (and any success-side return-value extraction)
// so this helper composes cleanly with the existing integration-test
// helpers that carry rich diagnostics on timeout. t.Helper is invoked
// so the test failure surface attributes to the caller's line.
func PollUntil(t *testing.T, timeout, tick time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(tick)
	}
}
