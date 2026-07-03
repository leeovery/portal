package bootstrap

// Task spectrum-tui-design-5-2 — progress-emitter seam on the orchestrator.
//
// The §10.2 concurrent cold-boot route streams a per-step progress event to
// the loading-page TUI. The orchestrator gains a context-carried emitter seam
// (WithProgressEmitter) that each of the ten steps invokes at the same site
// it logs "step complete", in step order. On the synchronous warm/CLI route no
// emitter is in the context, so Run behaves exactly as today (the emitter read
// resolves to nil and every emit is a no-op).

import (
	"context"
	"testing"
)

// TestRun_EmitsProgressEventPerStepInOrder asserts the orchestrator emits one
// StepEvent per real bootstrap step, in spec order, with the canonical step
// index (1..10) and name, when an emitter is wired through the context.
func TestRun_EmitsProgressEventPerStepInOrder(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	var got []StepEvent
	ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) {
		got = append(got, ev)
	})

	if _, _, err := o.Run(ctx); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := []StepEvent{
		{Index: 1, Name: stepEnsureServer},
		{Index: 2, Name: stepRegisterHooks},
		{Index: 3, Name: stepSetRestoring},
		{Index: 4, Name: stepSweepOrphanDaemons},
		{Index: 5, Name: stepEnsureSaver},
		{Index: 6, Name: stepRestore},
		{Index: 7, Name: stepEagerSignalHydrate},
		{Index: 8, Name: stepClearRestoring},
		{Index: 9, Name: stepCleanStaleMarkers},
		{Index: 10, Name: stepSweepOrphanFIFOs},
	}
	if len(got) != len(want) {
		t.Fatalf("emitted %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestRun_NoEmitterIsNoOp asserts the synchronous route (no emitter in the
// context) runs every step and returns success without any progress plumbing —
// the warm/CLI path is unaffected by the seam.
func TestRun_NoEmitterIsNoOp(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	// context.Background() carries no emitter — the read must resolve to nil
	// and Run must behave exactly as the pre-5-2 synchronous path.
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := []string{
		"EnsureServer", "RegisterPortalHooks", "Set", "SweepOrphanDaemons",
		"EnsureSaver", "Restore", "EagerSignalHydrate", "Clear",
		"CleanStaleMarkers", "Sweep",
	}
	if !equalCalls(r.calls, want) {
		t.Errorf("call order = %v, want %v", r.calls, want)
	}
}

// TestRun_FatalStepStopsEmitting asserts a fatal step (EnsureServer here) emits
// no progress event for the aborting step — mirroring the "no step-complete log
// for an aborting step" contract — and emits nothing thereafter.
func TestRun_FatalStepStopsEmitting(t *testing.T) {
	r := &stepRecorder{EnsureServerErr: context.Canceled}
	o := newOrchestrator(r, nil)

	var got []StepEvent
	ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) {
		got = append(got, ev)
	})

	if _, _, err := o.Run(ctx); err == nil {
		t.Fatal("Run returned nil error, want fatal from EnsureServer")
	}
	if len(got) != 0 {
		t.Errorf("emitted %d events on fatal step 1, want 0: %+v", len(got), got)
	}
}
