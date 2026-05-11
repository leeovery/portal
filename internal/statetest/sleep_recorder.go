// Package statetest holds shared test helpers for callers exercising the
// internal/state seam-bearing APIs (notably state.WriteFIFOSignal). It exists
// because Go does not share test-only symbols across package boundaries: two
// consumer test packages (internal/state itself and cmd) each duplicated the
// same "record every Sleep duration" fake before this package was introduced.
//
// Helpers here are production-equivalent in shape — they implement the same
// function signatures the production code expects — but they are intended
// strictly for test code. Production code MUST NOT import this package; the
// package name suffix and the file-name convention follow the precedent set
// by internal/restoretest and internal/tmuxtest.
//
// Concurrency: every recording helper in this package is single-goroutine only.
// Production callers drive the recorded seam serially (single-threaded retry
// loop / per-pane iteration), so the helpers omit synchronization to keep
// their internals trivially inspectable from test assertions.
package statetest

import "time"

// RecordingSleep records every duration handed to its Fn() seam so tests can
// assert the retry-ladder shape without timing-dependent waits. Replaces the
// duplicated fakeSleep helpers previously declared in
// internal/state/signal_hydrate_test.go and cmd/state_signal_hydrate_test.go.
//
// Usage (production seam expects func(time.Duration)):
//
//	r := &statetest.RecordingSleep{}
//	state.WriteFIFOSignal(path, openFIFO, r.Fn())
//	// r.Durations now holds every Sleep duration the production code passed.
//
// See the package doc for the single-goroutine concurrency posture.
type RecordingSleep struct {
	// Durations is the ordered list of every duration the production code
	// passed to the Fn() closure. Tests inspect this slice directly to assert
	// the retry-ladder shape (e.g. reflect.DeepEqual against
	// state.SignalHydrateRetryDelays).
	Durations []time.Duration
}

// Fn returns a closure that appends each invocation's duration to
// r.Durations. The closure captures r by pointer so a single RecordingSleep
// instance accumulates state across multiple Fn() handouts (rare in practice
// but cheap to support).
func (r *RecordingSleep) Fn() func(time.Duration) {
	return func(d time.Duration) { r.Durations = append(r.Durations, d) }
}
