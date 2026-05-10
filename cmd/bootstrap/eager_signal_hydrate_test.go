package bootstrap

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// recordingFIFOWriter records each fifoPath EagerSignalCore writes to and
// (optionally) returns a per-path or global error so tests can simulate
// per-FIFO write failures and assert loop-continuation behaviour.
type recordingFIFOWriter struct {
	calls []string
	// errOn returns the configured error on the first call whose path equals
	// the key. Used for "second pane fails" style scenarios.
	errOn map[string]error
	// err, when non-nil, is returned for every call. Useful for "all writes
	// fail" scenarios.
	err error
}

func (w *recordingFIFOWriter) Write(path string) error {
	w.calls = append(w.calls, path)
	if w.err != nil {
		return w.err
	}
	if e, ok := w.errOn[path]; ok {
		return e
	}
	return nil
}

// TestEagerHydrateSignalerInterfaceContract pins the seam shape: the
// orchestrator step relies on a single-method interface so adding a new
// step seam stays one-line uniform with siblings (MarkerCleaner, FIFOSweeper).
func TestEagerHydrateSignalerInterfaceContract(t *testing.T) {
	// Compile-time assertion: NoOp must satisfy the interface. If the
	// interface signature drifts, this assignment fails to compile.
	var _ EagerHydrateSignaler = NoOpEagerHydrateSignaler{}
	var _ EagerHydrateSignaler = (*EagerSignalCore)(nil)
}

// TestNoOpEagerHydrateSignaler_ReturnsNil pins the canonical no-op fallback:
// the zero-value struct's method always returns nil so production wiring can
// drop it in when dependency resolution fails (mirroring NoOpFIFOSweeper).
func TestNoOpEagerHydrateSignaler_ReturnsNil(t *testing.T) {
	if err := (NoOpEagerHydrateSignaler{}).EagerSignalHydrate(); err != nil {
		t.Errorf("NoOpEagerHydrateSignaler.EagerSignalHydrate = %v; want nil", err)
	}
}

// TestEagerSignalHydrate_WritesSignalToEveryMarkerFIFO pins the N-marker
// happy path: every marker paneKey gets one WriteFIFOSignal call at
// state.FIFOPath(stateDir, paneKey), the loop visits every entry, and the
// method returns nil.
func TestEagerSignalHydrate_WritesSignalToEveryMarkerFIFO(t *testing.T) {
	stateDir := "/var/state"
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"alpha__0.0": {},
		"beta__1.2":  {},
		"gamma__2.0": {},
	}}
	writer := &recordingFIFOWriter{}

	c := &EagerSignalCore{
		Markers:         lister,
		StateDir:        stateDir,
		WriteFIFOSignal: writer.Write,
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate returned error: %v", err)
	}

	// Marker map iteration order is non-deterministic so sort both sides
	// before comparing the visited paths.
	want := []string{
		state.FIFOPath(stateDir, "alpha__0.0"),
		state.FIFOPath(stateDir, "beta__1.2"),
		state.FIFOPath(stateDir, "gamma__2.0"),
	}
	got := append([]string{}, writer.calls...)
	sort.Strings(want)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("WriteFIFOSignal call count = %d (%v); want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("WriteFIFOSignal call[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// TestEagerSignalHydrate_ZeroMarkersIsNoOp pins the zero-marker case: the
// step short-circuits before any FIFO write attempt and returns nil. This is
// the post-Restore steady state on a fresh bootstrap with no saved sessions
// and must not produce a spurious WriteFIFOSignal call.
func TestEagerSignalHydrate_ZeroMarkersIsNoOp(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{}}
	writer := &recordingFIFOWriter{}

	c := &EagerSignalCore{
		Markers:         lister,
		StateDir:        "/var/state",
		WriteFIFOSignal: writer.Write,
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate returned error: %v", err)
	}
	if len(writer.calls) != 0 {
		t.Errorf("WriteFIFOSignal called %d times under zero-marker no-op; want 0 (calls=%v)", len(writer.calls), writer.calls)
	}
}

// TestEagerSignalHydrate_PerFIFOWriteFailureLogsAndContinues pins the
// soft-warning posture: a single failing FIFO write must not abort the loop;
// every other marker still receives its signal, the failure is logged via
// Logger.Warn under ComponentHydrate with the spec's "eager-signal: write
// fifo" prefix, and the method still returns nil.
func TestEagerSignalHydrate_PerFIFOWriteFailureLogsAndContinues(t *testing.T) {
	stateDir := "/var/state"
	failPath := state.FIFOPath(stateDir, "broken__0.0")
	sentinel := errors.New("write fifo: i/o error")

	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"broken__0.0":   {},
		"healthy__1.0":  {},
		"healthy2__2.0": {},
	}}
	writer := &recordingFIFOWriter{
		errOn: map[string]error{failPath: sentinel},
	}
	logger := &recordingLogger{}

	c := &EagerSignalCore{
		Markers:         lister,
		StateDir:        stateDir,
		WriteFIFOSignal: writer.Write,
		Logger:          logger,
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate must return nil after per-FIFO write failure; got %v", err)
	}
	if len(writer.calls) != 3 {
		t.Errorf("WriteFIFOSignal call count = %d; want 3 (loop must continue past the failing write); calls=%v", len(writer.calls), writer.calls)
	}

	// Locate the warning entry and pin its component routing (ComponentHydrate)
	// + the spec-mandated "eager-signal: write fifo" prefix + the failing path
	// in the formatted message body.
	found := false
	for i, msg := range logger.warnings {
		if strings.Contains(msg, "eager-signal: write fifo") && strings.Contains(msg, failPath) {
			if logger.warnComponents[i] != state.ComponentHydrate {
				t.Errorf("warning component[%d] = %q; want %q", i, logger.warnComponents[i], state.ComponentHydrate)
			}
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a Warn entry containing %q and the failing FIFO path %q; got warnings=%v", "eager-signal: write fifo", failPath, logger.warnings)
	}
}

// TestEagerSignalHydrate_ReturnsErrorWhenListSkeletonMarkersFails pins the
// orchestrator-soft-warn path: a ShowAllServerOptions failure surfaces as a
// non-nil return so the orchestrator's Warn-and-swallow site logs it
// uniformly with siblings (FIFOSweeper, CleanStaleMarkers). No FIFO writes
// must be attempted because the marker set is unknown.
func TestEagerSignalHydrate_ReturnsErrorWhenListSkeletonMarkersFails(t *testing.T) {
	sentinel := errors.New("show-options boom")
	lister := &fakeMarkerLister{err: sentinel}
	writer := &recordingFIFOWriter{}

	c := &EagerSignalCore{
		Markers:         lister,
		StateDir:        "/var/state",
		WriteFIFOSignal: writer.Write,
	}

	err := c.EagerSignalHydrate()
	if err == nil {
		t.Fatal("EagerSignalHydrate returned nil; want wrapped error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("EagerSignalHydrate err = %v; want errors.Is(err, sentinel)=true", err)
	}
	if len(writer.calls) != 0 {
		t.Errorf("WriteFIFOSignal called %d times after enumeration failure; want 0", len(writer.calls))
	}
}

// TestEagerSignalHydrate_NilLoggerTolerated pins the local noopLogger
// substitution: a nil Logger field must not panic when the failure path
// exercises Logger.Warn. Mirrors MarkerCleanupCore's nil-Logger contract.
func TestEagerSignalHydrate_NilLoggerTolerated(t *testing.T) {
	stateDir := "/var/state"
	failPath := state.FIFOPath(stateDir, "broken__0.0")

	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"broken__0.0":  {},
		"healthy__1.0": {},
	}}
	writer := &recordingFIFOWriter{
		errOn: map[string]error{failPath: errors.New("boom")},
	}

	c := &EagerSignalCore{
		Markers:         lister,
		StateDir:        stateDir,
		WriteFIFOSignal: writer.Write,
		Logger:          nil, // contract: must not panic when Logger.Warn fires.
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate with nil Logger returned error: %v", err)
	}
	if len(writer.calls) != 2 {
		t.Errorf("WriteFIFOSignal call count = %d; want 2 (loop must continue past failing write under nil Logger); calls=%v", len(writer.calls), writer.calls)
	}
}

// TestOrchestrator_HasEagerSignalerField pins task 1-3's structural acceptance
// criterion: *Orchestrator gains an EagerSignaler EagerHydrateSignaler field.
// Task 1-4 wires the field into Bootstrap()'s execution flow; this test only
// proves the field exists and is interface-typed so the wiring task can
// compile.
func TestOrchestrator_HasEagerSignalerField(t *testing.T) {
	o := &Orchestrator{
		EagerSignaler: NoOpEagerHydrateSignaler{},
	}
	// Compile-time + runtime: the assignment above proves the field is
	// declared and typed as EagerHydrateSignaler. A nil dereference here
	// would also catch a future refactor that drops the field.
	if o.EagerSignaler == nil {
		t.Fatal("Orchestrator.EagerSignaler unexpectedly nil after explicit assignment")
	}
	if err := o.EagerSignaler.EagerSignalHydrate(); err != nil {
		t.Errorf("NoOp injected via Orchestrator.EagerSignaler returned %v; want nil", err)
	}
}
