package bootstrap

// Task skip-bootstrap-when-warm-1-2 — the version-stamped @portal-bootstrapped
// latch is written as the final pre-return action of a successful Orchestrator.Run.
//
// These tests use a recording LatchWriter double that captures every
// SetServerOption(name, value) call and can be primed to fail, plus the
// existing stepRecorder / newOrchestrator scaffolding from bootstrap_test.go.

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

// latchCall records one SetServerOption(name, value) invocation on the
// recording latch writer.
type latchCall struct {
	name  string
	value string
}

// recordingLatch is a test double for the LatchWriter seam. It captures every
// SetServerOption(name, value) call and can be primed (err) to return an
// error. When seq is non-nil each call also appends a "latch" marker so
// ordering against the orchestration-complete log line is observable.
type recordingLatch struct {
	calls []latchCall
	err   error
	seq   *[]string
}

func (l *recordingLatch) SetServerOption(name, value string) error {
	l.calls = append(l.calls, latchCall{name: name, value: value})
	if l.seq != nil {
		*l.seq = append(*l.seq, "latch")
	}
	return l.err
}

// Compile-time assertion the recording double satisfies the seam.
var _ LatchWriter = (*recordingLatch)(nil)

// orchestrationSeqHandler records a marker into a shared sequence when it
// observes the orchestration-complete INFO line, so a test can assert the
// latch write (which appends "latch" to the same sequence) precedes it.
type orchestrationSeqHandler struct {
	seq *[]string
}

func (h *orchestrationSeqHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *orchestrationSeqHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *orchestrationSeqHandler) WithGroup(string) slog.Handler            { return h }
func (h *orchestrationSeqHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "orchestration complete" {
		*h.seq = append(*h.seq, "log:orchestration complete")
	}
	return nil
}

// TestOrchestratorRun_stampsLatchWithVersionAfterSoftWarning proves a run that
// finishes with only a soft warning (SaverDownWarning here) still reaches the
// latch and stamps @portal-bootstrapped = Version exactly once — the
// soft-warnings-still-latch rule.
func TestOrchestratorRun_stampsLatchWithVersionAfterSoftWarning(t *testing.T) {
	r := &stepRecorder{EnsureSaverErr: errors.New("saver boom")}
	latch := &recordingLatch{}
	o := newOrchestrator(r, nil)
	o.Latch = latch
	o.Version = "v1.2.3"

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(latch.calls) != 1 {
		t.Fatalf("latch SetServerOption call count = %d, want 1; calls=%+v", len(latch.calls), latch.calls)
	}
	if latch.calls[0].name != state.BootstrappedMarkerName {
		t.Errorf("latch name = %q, want %q", latch.calls[0].name, state.BootstrappedMarkerName)
	}
	if latch.calls[0].value != "v1.2.3" {
		t.Errorf("latch value = %q, want %q", latch.calls[0].value, "v1.2.3")
	}

	// Soft warning still latches AND still surfaces: the SaverDownWarning
	// survives in the returned slice.
	if len(warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1; got %#v", len(warnings), warnings)
	}
	if warnings[0].Lines[0] != SaverDownWarning().Lines[0] {
		t.Errorf("warnings[0] = %q, want SaverDownWarning", warnings[0].Lines[0])
	}
}

// TestOrchestratorRun_doesNotStampLatchOnFatalAbort proves a run that aborts at
// a fatal step (SetRestoring here) returns before the stamp — the latch writer
// is never called, so the next command retries the full bootstrap.
func TestOrchestratorRun_doesNotStampLatchOnFatalAbort(t *testing.T) {
	r := &stepRecorder{SetErr: errors.New("set marker boom")}
	latch := &recordingLatch{}
	o := newOrchestrator(r, nil)
	o.Latch = latch
	o.Version = "v1.2.3"

	_, _, err := o.Run(context.Background())

	var fatal *FatalError
	if !errors.As(err, &fatal) {
		t.Fatalf("expected *FatalError from a fatal-step abort, got %T (%v)", err, err)
	}
	if len(latch.calls) != 0 {
		t.Errorf("latch must not be written on a fatal abort; got %+v", latch.calls)
	}
}

// TestOrchestratorRun_swallowsLatchWriteFailureAsWarn proves a stamp-write
// failure is a pure WARN under the bootstrap component: Run still returns
// (serverStarted, warnings, nil), the warnings slice is unchanged (no
// latch-write entry appended), and no progress StepEvent is emitted for the
// write.
func TestOrchestratorRun_swallowsLatchWriteFailureAsWarn(t *testing.T) {
	r := &stepRecorder{}
	latch := &recordingLatch{err: errors.New("latch write boom")}

	sink := &logtest.Sink{}
	logger := slog.New(sink).With("component", "bootstrap")

	// Record progress events to prove the latch write emits none.
	var events []StepEvent
	ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) {
		events = append(events, ev)
	})

	o := newOrchestrator(r, logger)
	o.Latch = latch
	o.Version = "v9.9.9"

	_, warnings, err := o.Run(ctx)
	if err != nil {
		t.Fatalf("latch-write failure must not be fatal; got %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("latch-write failure must NOT append a warning; got %#v", warnings)
	}

	// The latch write emits no StepEvent. Each executed step emits exactly one
	// "step complete" INFO and exactly one StepEvent; the latch write emits
	// neither — so the event count equals the step-complete count regardless of
	// the live step total (robust to the task 1-3 11→10 change).
	stepCompletes := 0
	for _, rec := range sink.Records() {
		if rec.Msg == "step complete" {
			stepCompletes++
		}
	}
	if stepCompletes == 0 {
		t.Fatal("expected at least one 'step complete' record")
	}
	if len(events) != stepCompletes {
		t.Errorf("emitted %d StepEvents but %d steps completed — the latch write must emit no event; events=%+v",
			len(events), stepCompletes, events)
	}

	// A WARN under component=bootstrap whose message names the specific latch
	// marker. The marker name is folded into the message text — there is no
	// non-vocabulary "marker" attr (the closed attr-key vocabulary permits only
	// "error" alongside the handler-injected baselines here).
	foundWarn := false
	for _, rec := range sink.Records() {
		if rec.Level != slog.LevelWarn {
			continue
		}
		comp, ok := rec.Attrs["component"]
		if !ok || comp.String() != "bootstrap" {
			continue
		}
		if !strings.Contains(rec.Msg, state.BootstrappedMarkerName) {
			continue
		}
		if _, hasMarker := rec.Attrs["marker"]; hasMarker {
			t.Errorf("latch-write WARN must not carry a non-vocabulary \"marker\" attr; rec=%+v", rec)
		}
		foundWarn = true
		break
	}
	if !foundWarn {
		t.Errorf("expected a WARN under component=bootstrap whose message names %q; records=%+v",
			state.BootstrappedMarkerName, sink.Records())
	}
}

// TestOrchestratorRun_stampsLatchBeforeOrchestrationComplete proves the latch
// is stamped exactly once and the write precedes the orchestration-complete
// summary line — the ordering that makes "latch present ⟺ a full bootstrap
// completed" hold before the terminal completion event on the concurrent path.
func TestOrchestratorRun_stampsLatchBeforeOrchestrationComplete(t *testing.T) {
	r := &stepRecorder{}
	var seq []string
	latch := &recordingLatch{seq: &seq}
	logger := slog.New(&orchestrationSeqHandler{seq: &seq})

	o := newOrchestrator(r, logger)
	o.Latch = latch
	o.Version = "v1.2.3"

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(latch.calls) != 1 {
		t.Fatalf("latch call count = %d, want exactly 1; calls=%+v", len(latch.calls), latch.calls)
	}

	latchIdx, completeIdx := -1, -1
	for i, ev := range seq {
		switch ev {
		case "latch":
			latchIdx = i
		case "log:orchestration complete":
			completeIdx = i
		}
	}
	if latchIdx == -1 {
		t.Fatalf("latch marker not recorded; seq=%v", seq)
	}
	if completeIdx == -1 {
		t.Fatalf("orchestration-complete marker not recorded; seq=%v", seq)
	}
	if latchIdx >= completeIdx {
		t.Errorf("latch write must precede the orchestration-complete summary; latchIdx=%d completeIdx=%d seq=%v",
			latchIdx, completeIdx, seq)
	}
}
