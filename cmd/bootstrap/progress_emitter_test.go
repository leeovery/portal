package bootstrap

import (
	"context"
	"testing"
)

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

	// The indices are the loading page's step numbering, which internal/tui
	// keys its labels to.
	want := []StepEvent{
		{Index: 3, Name: stepSetRestoring},
		{Index: 4, Name: stepSweepOrphanDaemons},
		{Index: 5, Name: stepEnsureSaver},
		{Index: 6, Name: stepRestore},
		{Index: 8, Name: stepClearRestoring},
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

func TestRun_NoEmitterIsNoOp(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !equalCalls(r.calls, wantAllCalls) {
		t.Errorf("call order = %v, want %v", r.calls, wantAllCalls)
	}
}

func TestRun_FatalStepStopsEmitting(t *testing.T) {
	r := &stepRecorder{SetErr: context.Canceled}
	o := newOrchestrator(r, nil)

	var got []StepEvent
	ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) {
		got = append(got, ev)
	})

	if _, _, err := o.Run(ctx); err == nil {
		t.Fatal("Run returned nil error, want fatal from SetRestoring")
	}
	if len(got) != 0 {
		t.Errorf("emitted %d events on a fatal first step, want 0: %+v", len(got), got)
	}
}
