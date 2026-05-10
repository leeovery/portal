package statetest

import "github.com/leeovery/portal/internal/state"

// RecordingFIFOSignaler records each fifoPath the production code sends a
// hydrate signal to via the state.FIFOSignaler seam, with optional per-path or
// global error injection so tests can simulate per-FIFO write failures and
// assert loop-continuation / soft-fail behaviour. It exists because two
// consumer test packages (cmd and cmd/bootstrap) each duplicated the same
// recording fake before this helper was promoted; the shared definition keeps
// the recording semantics uniform across both call sites.
//
// Concurrent invocation from multiple goroutines is NOT supported; the
// production callers drive SendSignal serially under a single-goroutine loop.
type RecordingFIFOSignaler struct {
	// Calls is the ordered list of every path the production code passed to
	// SendSignal. Tests inspect this slice directly to assert which paneKey
	// FIFO paths were signaled. Every call is recorded — including the calls
	// that subsequently return an injected error — so loop-continuation
	// assertions can count attempted writes.
	Calls []string
	// ErrOn returns the configured error on calls whose path equals the key.
	// Useful for "this pane fails, others succeed" scenarios.
	ErrOn map[string]error
	// Err, when non-nil, is returned for every call. Useful for "every signal
	// fails" scenarios (e.g. retry-exhaustion soft-fail).
	Err error
}

// SendSignal satisfies state.FIFOSignaler. It records path verbatim into Calls
// and then returns the configured error (global Err takes precedence over the
// per-path ErrOn[path] lookup). Recording happens unconditionally so callers
// can assert that the production code attempted every write even when each
// attempt surfaced a soft-fail error.
func (r *RecordingFIFOSignaler) SendSignal(path string) error {
	r.Calls = append(r.Calls, path)
	if r.Err != nil {
		return r.Err
	}
	if e, ok := r.ErrOn[path]; ok {
		return e
	}
	return nil
}

// Compile-time assertion: the recording fake must satisfy state.FIFOSignaler.
// A drift in the production interface signature surfaces here.
var _ state.FIFOSignaler = (*RecordingFIFOSignaler)(nil)
