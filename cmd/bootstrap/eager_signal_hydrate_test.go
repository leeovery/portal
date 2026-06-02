package bootstrap

import (
	"errors"
	"log/slog"
	"sort"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/statetest"
)

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
// happy path: every marker paneKey gets one Signaler.SendSignal call at
// state.FIFOPath(stateDir, paneKey), the loop visits every entry, and the
// method returns nil.
func TestEagerSignalHydrate_WritesSignalToEveryMarkerFIFO(t *testing.T) {
	stateDir := "/var/state"
	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"alpha__0.0": {},
		"beta__1.2":  {},
		"gamma__2.0": {},
	}}
	signaler := &statetest.RecordingFIFOSignaler{}

	c := &EagerSignalCore{
		Markers:  lister,
		StateDir: stateDir,
		Signaler: signaler,
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
	got := append([]string{}, signaler.Calls...)
	sort.Strings(want)
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("Signaler.SendSignal call count = %d (%v); want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Signaler.SendSignal call[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// TestEagerSignalHydrate_ZeroMarkersIsNoOp pins the zero-marker case: the
// step short-circuits before any FIFO write attempt and returns nil. This is
// the post-Restore steady state on a fresh bootstrap with no saved sessions
// and must not produce a spurious Signaler.SendSignal call.
func TestEagerSignalHydrate_ZeroMarkersIsNoOp(t *testing.T) {
	lister := &fakeMarkerLister{markers: map[string]struct{}{}}
	signaler := &statetest.RecordingFIFOSignaler{}

	c := &EagerSignalCore{
		Markers:  lister,
		StateDir: "/var/state",
		Signaler: signaler,
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate returned error: %v", err)
	}
	if len(signaler.Calls) != 0 {
		t.Errorf("Signaler.SendSignal called %d times under zero-marker no-op; want 0 (calls=%v)", len(signaler.Calls), signaler.Calls)
	}
}

// TestEagerSignalHydrate_PerFIFOWriteFailureLogsAndContinues pins the
// soft-warning posture: a single failing FIFO write must not abort the loop;
// every other marker still receives its signal, the failure is logged via
// the package-level signalLogger under component=signal with the
// "eager-signal write fifo failed" message, path/error/error_class=unexpected
// attrs (error is the WRAPPED err, passed directly), and the method still
// returns nil. The re-attribution (Phase 5 Task 5-11) homes the FIFO-signaling
// mechanism under the signal component, NOT hydrate.
func TestEagerSignalHydrate_PerFIFOWriteFailureLogsAndContinues(t *testing.T) {
	sink := installCleanSummarySink(t)
	stateDir := "/var/state"
	failPath := state.FIFOPath(stateDir, "broken__0.0")
	sentinel := errors.New("write fifo: i/o error")

	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"broken__0.0":   {},
		"healthy__1.0":  {},
		"healthy2__2.0": {},
	}}
	signaler := &statetest.RecordingFIFOSignaler{
		ErrOn: map[string]error{failPath: sentinel},
	}

	c := &EagerSignalCore{
		Markers:  lister,
		StateDir: stateDir,
		Signaler: signaler,
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate must return nil after per-FIFO write failure; got %v", err)
	}
	if len(signaler.Calls) != 3 {
		t.Errorf("Signaler.SendSignal call count = %d; want 3 (loop must continue past the failing write); calls=%v", len(signaler.Calls), signaler.Calls)
	}

	// Exactly one write-failure WARN under component=signal, carrying the
	// failing FIFO path, the WRAPPED error, and error_class=unexpected.
	warns := sink.matching(slog.LevelWarn, "signal", "eager-signal write fifo failed")
	if len(warns) != 1 {
		t.Fatalf("expected 1 WARN under component=signal for the failing FIFO, got %d: %+v", len(warns), sink.all())
	}
	rec := warns[0]
	if p, ok := rec.attrs["path"]; !ok || p.String() != failPath {
		t.Errorf("WARN path attr = %v; want %q", rec.attrs["path"], failPath)
	}
	if ec, ok := rec.attrs["error_class"]; !ok || ec.String() != "unexpected" {
		t.Errorf("WARN error_class attr = %v; want %q", rec.attrs["error_class"], "unexpected")
	}
	errAttr, ok := rec.attrs["error"]
	if !ok {
		t.Fatalf("WARN missing error attr: %+v", rec.attrs)
	}
	// The error attr must carry the wrapped err passed directly (not .Error()),
	// so a slog.AnyValue holding the error value renders the sentinel message.
	if errAttr.Kind() != slog.KindAny {
		t.Errorf("error attr kind = %v; want Any (wrapped err passed directly, not .Error())", errAttr.Kind())
	}
	if gotErr, ok := errAttr.Any().(error); !ok || !errors.Is(gotErr, sentinel) {
		t.Errorf("error attr = %v; want errors.Is(err, sentinel)=true", errAttr.Any())
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
	signaler := &statetest.RecordingFIFOSignaler{}

	c := &EagerSignalCore{
		Markers:  lister,
		StateDir: "/var/state",
		Signaler: signaler,
	}

	err := c.EagerSignalHydrate()
	if err == nil {
		t.Fatal("EagerSignalHydrate returned nil; want wrapped error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("EagerSignalHydrate err = %v; want errors.Is(err, sentinel)=true", err)
	}
	if len(signaler.Calls) != 0 {
		t.Errorf("Signaler.SendSignal called %d times after enumeration failure; want 0", len(signaler.Calls))
	}
}

// TestEagerSignalHydrate_NilLoggerTolerated pins the nil-Logger contract: a
// nil Logger field must not panic on the per-FIFO failure path. Since Phase 5
// Task 5-11 re-homed the write-failure WARN onto the package-level signalLogger
// (always non-nil), the field is no longer read by the WARN, but the
// nil-tolerance contract for the DI-wired field is preserved.
func TestEagerSignalHydrate_NilLoggerTolerated(t *testing.T) {
	stateDir := "/var/state"
	failPath := state.FIFOPath(stateDir, "broken__0.0")

	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"broken__0.0":  {},
		"healthy__1.0": {},
	}}
	signaler := &statetest.RecordingFIFOSignaler{
		ErrOn: map[string]error{failPath: errors.New("boom")},
	}

	c := &EagerSignalCore{
		Markers:  lister,
		StateDir: stateDir,
		Signaler: signaler,
		Logger:   nil, // contract: must not panic on the failure path.
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate with nil Logger returned error: %v", err)
	}
	if len(signaler.Calls) != 2 {
		t.Errorf("Signaler.SendSignal call count = %d; want 2 (loop must continue past failing write under nil Logger); calls=%v", len(signaler.Calls), signaler.Calls)
	}
}

// TestEagerSignalHydrate_SuccessEmitsSignalledDebugBreadcrumb pins the
// per-FIFO success breadcrumb (Phase 5 Task 5-11): every successful
// SendSignal emits a DEBUG "fifo signalled" under component=signal carrying
// the FIFO path. The breadcrumb is silent at INFO and present at DEBUG —
// the call-site transition breadcrumb that lets `grep signal:` reconstruct
// the FIFO-signaling behaviour.
func TestEagerSignalHydrate_SuccessEmitsSignalledDebugBreadcrumb(t *testing.T) {
	sink := installCleanSummarySink(t)
	stateDir := "/var/state"

	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"alpha__0.0": {},
		"beta__1.2":  {},
	}}
	signaler := &statetest.RecordingFIFOSignaler{}

	c := &EagerSignalCore{
		Markers:  lister,
		StateDir: stateDir,
		Signaler: signaler,
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate returned error: %v", err)
	}

	// One DEBUG "fifo signalled" under component=signal per successful FIFO.
	dbg := sink.matching(slog.LevelDebug, "signal", "fifo signalled")
	if len(dbg) != 2 {
		t.Fatalf("expected 2 DEBUG 'fifo signalled' under component=signal, got %d: %+v", len(dbg), sink.all())
	}
	gotPaths := map[string]bool{}
	for _, r := range dbg {
		p, ok := r.attrs["path"]
		if !ok {
			t.Fatalf("DEBUG 'fifo signalled' missing path attr: %+v", r.attrs)
		}
		gotPaths[p.String()] = true
	}
	for _, key := range []string{"alpha__0.0", "beta__1.2"} {
		wantPath := state.FIFOPath(stateDir, key)
		if !gotPaths[wantPath] {
			t.Errorf("missing DEBUG 'fifo signalled' for path %q; got %v", wantPath, gotPaths)
		}
	}

	// The breadcrumb is DEBUG-only: nothing rendered at INFO for it.
	if got := sink.matching(slog.LevelInfo, "signal", "fifo signalled"); len(got) != 0 {
		t.Errorf("'fifo signalled' must be DEBUG, not INFO; got %d INFO entries: %+v", len(got), got)
	}
}

// TestEagerSignalHydrate_NoSignalingLineUnderHydrateOrBootstrap pins the
// re-attribution (Phase 5 Task 5-11): after homing the FIFO-signaling
// mechanism under signal, NO signaling-mechanism line renders under
// component=hydrate or component=bootstrap. A mixed success+failure run
// must produce only signal-component lines for the mechanism.
func TestEagerSignalHydrate_NoSignalingLineUnderHydrateOrBootstrap(t *testing.T) {
	sink := installCleanSummarySink(t)
	stateDir := "/var/state"
	failPath := state.FIFOPath(stateDir, "broken__0.0")

	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"broken__0.0":  {},
		"healthy__1.0": {},
	}}
	signaler := &statetest.RecordingFIFOSignaler{
		ErrOn: map[string]error{failPath: errors.New("write fifo: i/o error")},
	}

	c := &EagerSignalCore{
		Markers:  lister,
		StateDir: stateDir,
		Signaler: signaler,
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate returned error: %v", err)
	}

	for _, r := range sink.all() {
		comp, ok := r.attrs["component"]
		if !ok {
			continue
		}
		if comp.String() == "hydrate" || comp.String() == "bootstrap" {
			t.Errorf("no signaling-mechanism line may render under %q; got %+v", comp.String(), r)
		}
	}
}

// TestEagerSignalHydrate_NoCycleSummaryNorNewAttrKeys pins the closed-attr +
// no-summary contract (Phase 5 Task 5-11): the EagerSignalHydrate run emits
// only the per-FIFO WARN/DEBUG under signal — no INFO cycle summary (owned by
// Task 5-2's bootstrap step-complete line, not a signal-sweep summary), and no
// attr keys beyond path/error/error_class.
func TestEagerSignalHydrate_NoCycleSummaryNorNewAttrKeys(t *testing.T) {
	sink := installCleanSummarySink(t)
	stateDir := "/var/state"
	failPath := state.FIFOPath(stateDir, "broken__0.0")

	lister := &fakeMarkerLister{markers: map[string]struct{}{
		"broken__0.0":  {},
		"healthy__1.0": {},
	}}
	signaler := &statetest.RecordingFIFOSignaler{
		ErrOn: map[string]error{failPath: errors.New("write fifo: i/o error")},
	}

	c := &EagerSignalCore{
		Markers:  lister,
		StateDir: stateDir,
		Signaler: signaler,
	}

	if err := c.EagerSignalHydrate(); err != nil {
		t.Fatalf("EagerSignalHydrate returned error: %v", err)
	}

	// No INFO line at all from the signal step (no cycle summary).
	for _, r := range sink.all() {
		if r.level == slog.LevelInfo {
			t.Errorf("EagerSignalHydrate must not emit an INFO cycle summary; got %+v", r)
		}
	}

	// The only attr keys the signal lines may carry are component + the closed
	// set path/error/error_class.
	allowed := map[string]bool{"component": true, "path": true, "error": true, "error_class": true}
	for _, r := range sink.all() {
		for key := range r.attrs {
			if !allowed[key] {
				t.Errorf("unexpected attr key %q on signal line %q; closed set is path/error/error_class", key, r.msg)
			}
		}
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
