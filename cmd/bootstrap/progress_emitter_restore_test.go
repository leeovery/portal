package bootstrap

// Task spectrum-tui-design-5-3 — restore per-session N/M progress install.
//
// On the concurrent cold-boot route step 6 installs a per-session progress
// callback onto a Restorer that satisfies the optional RestoreProgressSink seam,
// forwarding each (n, m) onto the SAME ctx emitter the step events ride
// (flavoured as Index 6 / stepRestore with RestoreN/RestoreM populated). On the
// synchronous route (no emitter) SetProgress is never called and the restore
// loop's Progress stays nil — byte-for-byte unchanged.

import (
	"context"
	"testing"
)

// progressRecorder is a stepRecorder that also satisfies RestoreProgressSink and
// drives the installed per-session callback through a fixed M on Restore — so a
// test can assert the orchestrator forwards restore N/M onto the ctx emitter.
type progressRecorder struct {
	stepRecorder
	m          int
	progressFn func(n, m int)
}

func (r *progressRecorder) SetProgress(fn func(n, m int)) { r.progressFn = fn }

func (r *progressRecorder) Restore() (bool, error) {
	r.calls = append(r.calls, "Restore")
	if r.progressFn != nil {
		for n := 1; n <= r.m; n++ {
			r.progressFn(n, r.m)
		}
	}
	return r.RestoreCorrupt, r.RestoreErr
}

func newProgressOrchestrator(r *progressRecorder) *Orchestrator {
	o := newOrchestrator(&r.stepRecorder, nil)
	o.Restore = r // override with the progress-capable restorer
	return o
}

// TestRun_InstallsRestoreProgressAndForwardsNMOntoEmitter asserts that on the
// concurrent route the per-session restore callback streams (n, m) onto the ctx
// emitter as Index-6 restore-progress events, interleaved before the restore
// step-complete tick.
func TestRun_InstallsRestoreProgressAndForwardsNMOntoEmitter(t *testing.T) {
	r := &progressRecorder{m: 3}
	o := newProgressOrchestrator(r)

	var got []StepEvent
	ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) {
		got = append(got, ev)
	})
	if _, _, err := o.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Collect the restore-progress events (RestoreM > 0).
	var nm [][2]int
	for _, ev := range got {
		if ev.RestoreM > 0 {
			if ev.Index != 6 || ev.Name != stepRestore {
				t.Errorf("restore-progress event = %+v, want Index 6 / %q", ev, stepRestore)
			}
			nm = append(nm, [2]int{ev.RestoreN, ev.RestoreM})
		}
	}
	want := [][2]int{{1, 3}, {2, 3}, {3, 3}}
	if len(nm) != len(want) {
		t.Fatalf("restore-progress N/M = %v, want %v", nm, want)
	}
	for i := range want {
		if nm[i] != want[i] {
			t.Errorf("restore-progress[%d] = %v, want %v", i, nm[i], want[i])
		}
	}

	// The restore STEP tick (zero N/M, Index 6) must still fire after the
	// per-session events.
	var sawStepTick bool
	for _, ev := range got {
		if ev.Index == 6 && ev.RestoreM == 0 {
			sawStepTick = true
		}
	}
	if !sawStepTick {
		t.Error("restore step-complete tick (Index 6, zero N/M) never fired")
	}
}

// TestRun_NoEmitterDoesNotInstallRestoreProgress asserts the synchronous route
// never installs the per-session callback — SetProgress is not called, so the
// restorer's progressFn stays nil and the restore loop is byte-for-byte
// unchanged.
func TestRun_NoEmitterDoesNotInstallRestoreProgress(t *testing.T) {
	r := &progressRecorder{m: 3}
	o := newProgressOrchestrator(r)

	// context.Background() carries no emitter.
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if r.progressFn != nil {
		t.Error("SetProgress was called on the synchronous route — restore progress must only install when an emitter is wired")
	}
}
