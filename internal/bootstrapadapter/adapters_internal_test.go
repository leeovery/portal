package bootstrapadapter

// White-box tests that need access to unexported fields of
// bootstrapadapter.StaleMarkerCleaner. Kept separate from the
// bootstrapadapter_test package so the public-API tests in adapters_test.go
// remain black-box.

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// innerStableStub satisfies the staleMarkerClient seam so we can drive
// CleanStaleMarkers without standing up a real tmux server. The stub returns
// an empty marker set so the cleanup short-circuits cleanly without invoking
// the live-pane lister or unsetter — the test only needs proof that the
// adapter's inner *bootstrap.MarkerCleanupCore is constructed once at
// adapter construction and re-used across CleanStaleMarkers invocations.
type innerStableStub struct{}

func (innerStableStub) ShowAllServerOptions() (string, error)         { return "", nil }
func (innerStableStub) ListAllPanesWithFormat(string) (string, error) { return "", nil }
func (innerStableStub) UnsetServerOption(string) error                { return nil }

// TestStaleMarkerCleaner_InnerCleanerStableAcrossCalls proves that
// CleanStaleMarkers does NOT construct a fresh inner *bootstrap.MarkerCleanupCore
// on every invocation. Pre-fix the adapter rebuilt the inner literal (and a
// closure-based MarkerLister) per call; the field-on-the-adapter design
// constructs it once at NewStaleMarkerCleaner time. We assert pointer
// equality between the inner captured before any call and the inner observed
// after two consecutive CleanStaleMarkers invocations — a regression that
// re-introduces per-call construction would either drop the field
// initialisation or overwrite it, both of which this test catches.
func TestStaleMarkerCleaner_InnerCleanerStableAcrossCalls(t *testing.T) {
	a := NewStaleMarkerCleaner(innerStableStub{}, nil)

	// Capture the inner pointer at construction.
	first := a.inner
	if first == nil {
		t.Fatal("NewStaleMarkerCleaner did not initialise inner *bootstrap.MarkerCleanupCore; got nil")
	}

	// Drive two consecutive cleanups. Both must succeed (stub returns empty
	// markers so the cleanup short-circuits to nil) and must NOT mutate the
	// adapter's inner pointer.
	if err := a.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers (first call): %v", err)
	}
	if a.inner != first {
		t.Errorf("inner pointer mutated after first CleanStaleMarkers; want stable")
	}
	if err := a.CleanStaleMarkers(); err != nil {
		t.Fatalf("CleanStaleMarkers (second call): %v", err)
	}
	if a.inner != first {
		t.Errorf("inner pointer mutated after second CleanStaleMarkers; want stable")
	}
}

// TestNewStaleMarkerCleaner_WiresMarkerCleanupCoreSeams proves that the
// constructor wires the staleMarkerClient seams to the inner
// *bootstrap.MarkerCleanupCore's Markers/Panes/Unsetter contracts so a
// downstream regression that drops one of the seams (or wires it to the
// wrong client method) surfaces here rather than at a live-tmux integration
// test. We exercise the wiring by driving a non-empty marker set through to
// a sentinel error from the live-pane lister — proving Markers and Panes
// were both invoked on the constructor-supplied stub.
func TestNewStaleMarkerCleaner_WiresMarkerCleanupCoreSeams(t *testing.T) {
	sentinel := errors.New("list-panes boom")
	stub := &staleClientStub{
		showOut: state.SkeletonMarkerPrefix + "stale__0.0 \"1\"\n",
		listErr: sentinel,
	}
	a := NewStaleMarkerCleaner(stub, nil)

	err := a.CleanStaleMarkers()
	if err == nil {
		t.Fatal("CleanStaleMarkers returned nil; want wrapped sentinel")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("CleanStaleMarkers err = %v; want errors.Is(err, sentinel)=true", err)
	}
}

// staleClientStub mirrors the stub in adapters_test.go but is duplicated here
// so the white-box test file stays self-contained (Go test files in the same
// package share symbols, but the external _test.go package's symbols are not
// visible from this internal-package test).
type staleClientStub struct {
	showOut string
	showErr error

	listOut    string
	listErr    error
	listFormat string

	unsetCalls []string
	unsetErr   error
}

func (s *staleClientStub) ShowAllServerOptions() (string, error) {
	return s.showOut, s.showErr
}

func (s *staleClientStub) ListAllPanesWithFormat(format string) (string, error) {
	s.listFormat = format
	return s.listOut, s.listErr
}

func (s *staleClientStub) UnsetServerOption(name string) error {
	s.unsetCalls = append(s.unsetCalls, name)
	return s.unsetErr
}
