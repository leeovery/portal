package statetest_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/statetest"
)

// TestRecordingFIFOSignaler_GlobalErrTakesPrecedence pins the priority order:
// when Err is non-nil it is returned for every call regardless of any
// per-path ErrOn entry. Recording is still unconditional so callers can count
// attempted writes.
func TestRecordingFIFOSignaler_GlobalErrTakesPrecedence(t *testing.T) {
	sentinel := errors.New("global boom")
	perPath := errors.New("per-path boom")
	r := &statetest.RecordingFIFOSignaler{
		Err:   sentinel,
		ErrOn: map[string]error{"/state/alpha.fifo": perPath},
	}

	err := r.SendSignal("/state/alpha.fifo")
	if !errors.Is(err, sentinel) {
		t.Errorf("SendSignal err = %v; want global sentinel %v (must dominate ErrOn)", err, sentinel)
	}

	// Calls a path that has no ErrOn entry — global Err still applies.
	err = r.SendSignal("/state/beta.fifo")
	if !errors.Is(err, sentinel) {
		t.Errorf("SendSignal err = %v; want global sentinel %v", err, sentinel)
	}

	want := []string{"/state/alpha.fifo", "/state/beta.fifo"}
	if !reflect.DeepEqual(r.Calls, want) {
		t.Errorf("Calls = %v; want %v (recording is unconditional)", r.Calls, want)
	}
}

// TestRecordingFIFOSignaler_PerPathErrOnReturnsConfiguredError pins the
// per-path branch: with global Err nil, an ErrOn[path] entry returns its
// configured error while paths without entries return nil. The path is still
// recorded either way.
func TestRecordingFIFOSignaler_PerPathErrOnReturnsConfiguredError(t *testing.T) {
	sentinel := errors.New("write fifo: i/o error")
	failPath := "/state/broken.fifo"
	r := &statetest.RecordingFIFOSignaler{
		ErrOn: map[string]error{failPath: sentinel},
	}

	if err := r.SendSignal(failPath); !errors.Is(err, sentinel) {
		t.Errorf("SendSignal(failPath) err = %v; want sentinel %v", err, sentinel)
	}
	if err := r.SendSignal("/state/healthy.fifo"); err != nil {
		t.Errorf("SendSignal(non-failing path) err = %v; want nil", err)
	}

	want := []string{failPath, "/state/healthy.fifo"}
	if !reflect.DeepEqual(r.Calls, want) {
		t.Errorf("Calls = %v; want %v", r.Calls, want)
	}
}

// TestRecordingFIFOSignaler_DefaultRecordsAndReturnsNil pins the zero-error
// happy path: with both Err nil and ErrOn nil/missing, every call records the
// path and returns nil.
func TestRecordingFIFOSignaler_DefaultRecordsAndReturnsNil(t *testing.T) {
	r := &statetest.RecordingFIFOSignaler{}

	paths := []string{"/state/a.fifo", "/state/b.fifo", "/state/c.fifo"}
	for _, p := range paths {
		if err := r.SendSignal(p); err != nil {
			t.Errorf("SendSignal(%q) err = %v; want nil", p, err)
		}
	}

	if !reflect.DeepEqual(r.Calls, paths) {
		t.Errorf("Calls = %v; want %v", r.Calls, paths)
	}
}

// TestRecordingFIFOSignaler_SatisfiesFIFOSignaler is a runtime echo of the
// compile-time assertion in fifo_signaler_recorder.go: the recording fake
// must remain assignable to state.FIFOSignaler so callers can drop it into
// the production seam without an adapter.
func TestRecordingFIFOSignaler_SatisfiesFIFOSignaler(t *testing.T) {
	var _ state.FIFOSignaler = (*statetest.RecordingFIFOSignaler)(nil)
}
