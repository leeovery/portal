package tui_test

// Task spectrum-tui-design-5-4 — step mapping (11 real bootstrap steps → 5
// friendly labels). The mapping is the single-source-of-truth contract between
// the streamed per-step progress (task 5-2) and the rendered tick-list (task
// 5-5). These tests pin the §10.4 table, the bar advance (11 increments,
// reaching 100% only after step 11), the done/active/pending label states, the
// (N/M) counter on Restoring sessions only, and the M=0 / zero-resume-command
// degenerate cases. Non-visual / vhs-exempt — verification is behavioural.

import (
	"math"
	"testing"

	"github.com/leeovery/portal/internal/tui"
)

const floatEps = 1e-9

// progress feeds a BootstrapProgressMsg into a fresh accumulator chain. Helper
// for terse table-driven assertions.
func feed(acc tui.LoadingProgress, events ...tui.BootstrapProgressMsg) tui.LoadingProgress {
	for _, e := range events {
		acc = acc.Apply(e)
	}
	return acc
}

// activeLabelText returns the Text of the single label currently in the active
// state, or "" if none is active.
func activeLabelText(v tui.LoadingProgressView) string {
	for _, l := range v.Labels {
		if l.State == tui.LabelActive {
			return l.Text
		}
	}
	return ""
}

// TestStepMapsToFriendlyLabel asserts each of the 11 real steps resolves to its
// §10.4 friendly label via the pure LabelForStep mapping, including step 6's
// RestoreM>0 → "Restoring sessions" vs RestoreM==0 → "Replaying scrollback"
// discrimination. This is the literal "maps each step to its label" contract,
// independent of the active/done lifecycle.
func TestStepMapsToFriendlyLabel(t *testing.T) {
	cases := []struct {
		name      string
		event     tui.BootstrapProgressMsg
		wantLabel string
	}{
		{"step 1 EnsureServer", tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"}, tui.LabelStartedTmuxServer},
		{"step 2 RegisterPortalHooks", tui.BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"}, tui.LabelRegisteredHooks},
		{"step 3 SetRestoring", tui.BootstrapProgressMsg{Index: 3, Name: "SetRestoring"}, tui.LabelRegisteredHooks},
		{"step 4 SweepOrphanDaemons", tui.BootstrapProgressMsg{Index: 4, Name: "SweepOrphanDaemons"}, tui.LabelRegisteredHooks},
		{"step 5 EnsureSaver", tui.BootstrapProgressMsg{Index: 5, Name: "EnsureSaver"}, tui.LabelRegisteredHooks},
		{"step 6 Restore skeleton (M>0)", tui.BootstrapProgressMsg{Index: 6, Name: "Restore", RestoreN: 1, RestoreM: 3}, tui.LabelRestoringSessions},
		{"step 6 Restore complete (M==0)", tui.BootstrapProgressMsg{Index: 6, Name: "Restore"}, tui.LabelReplayingScrollback},
		{"step 7 EagerSignalHydrate", tui.BootstrapProgressMsg{Index: 7, Name: "EagerSignalHydrate"}, tui.LabelReplayingScrollback},
		{"step 8 ClearRestoring", tui.BootstrapProgressMsg{Index: 8, Name: "ClearRestoring"}, tui.LabelRunningResumeCommands},
		{"step 9 CleanStaleMarkers", tui.BootstrapProgressMsg{Index: 9, Name: "CleanStaleMarkers"}, tui.LabelRunningResumeCommands},
		{"step 10 SweepOrphanFIFOs", tui.BootstrapProgressMsg{Index: 10, Name: "SweepOrphanFIFOs"}, tui.LabelRunningResumeCommands},
		{"step 11 CleanStale", tui.BootstrapProgressMsg{Index: 11, Name: "CleanStale"}, tui.LabelRunningResumeCommands},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tui.LabelForStep(tc.event); got != tc.wantLabel {
				t.Errorf("LabelForStep(step %d) = %q; want %q", tc.event.Index, got, tc.wantLabel)
			}
		})
	}
}

// TestBarAdvancesEveryStep asserts the bar advances on every real step (11
// increments of 1/11) and reaches exactly 1.0 only after step 11 — not 5
// increments.
func TestBarAdvancesEveryStep(t *testing.T) {
	acc := tui.LoadingProgress{}
	if f := acc.View().BarFraction; f != 0 {
		t.Fatalf("initial bar fraction = %v; want 0", f)
	}
	for step := 1; step <= 11; step++ {
		acc = acc.Apply(tui.BootstrapProgressMsg{Index: step, Name: "step"})
		want := float64(step) / 11.0
		got := acc.View().BarFraction
		if math.Abs(got-want) > floatEps {
			t.Errorf("after step %d bar fraction = %v; want %v", step, got, want)
		}
		if step < 11 && got >= 1.0 {
			t.Errorf("bar reached 100%% at step %d; must reach 100%% only after step 11", step)
		}
	}
	if got := acc.View().BarFraction; math.Abs(got-1.0) > floatEps {
		t.Errorf("after step 11 bar fraction = %v; want 1.0", got)
	}
}

// TestLabelStateTransitions asserts that as the stream progresses (events signal
// step COMPLETION) completed labels are done, the current frontier label is
// active, and future labels are pending.
func TestLabelStateTransitions(t *testing.T) {
	acc := tui.LoadingProgress{}

	// Before any step: every label pending.
	assertStates(t, acc.View(), map[string]tui.LabelState{
		tui.LabelStartedTmuxServer:     tui.LabelPending,
		tui.LabelRegisteredHooks:       tui.LabelPending,
		tui.LabelRestoringSessions:     tui.LabelPending,
		tui.LabelReplayingScrollback:   tui.LabelPending,
		tui.LabelRunningResumeCommands: tui.LabelPending,
	})

	// After step 1 COMPLETES: "Started tmux server" done; the frontier advances
	// to the now-executing "Registered hooks" group (active); rest pending.
	acc = acc.Apply(tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"})
	assertStates(t, acc.View(), map[string]tui.LabelState{
		tui.LabelStartedTmuxServer:     tui.LabelDone,
		tui.LabelRegisteredHooks:       tui.LabelActive,
		tui.LabelRestoringSessions:     tui.LabelPending,
		tui.LabelReplayingScrollback:   tui.LabelPending,
		tui.LabelRunningResumeCommands: tui.LabelPending,
	})

	// After steps 2-5 complete: "Registered hooks" done; frontier at
	// "Restoring sessions" (step 6 not yet complete).
	for step := 2; step <= 5; step++ {
		acc = acc.Apply(tui.BootstrapProgressMsg{Index: step, Name: "step"})
	}
	assertStates(t, acc.View(), map[string]tui.LabelState{
		tui.LabelStartedTmuxServer:     tui.LabelDone,
		tui.LabelRegisteredHooks:       tui.LabelDone,
		tui.LabelRestoringSessions:     tui.LabelActive,
		tui.LabelReplayingScrollback:   tui.LabelPending,
		tui.LabelRunningResumeCommands: tui.LabelPending,
	})

	// After step 11 (the last real step): every label is done; no active frontier.
	for step := 6; step <= 11; step++ {
		acc = acc.Apply(tui.BootstrapProgressMsg{Index: step, Name: "step"})
	}
	assertStates(t, acc.View(), map[string]tui.LabelState{
		tui.LabelStartedTmuxServer:     tui.LabelDone,
		tui.LabelRegisteredHooks:       tui.LabelDone,
		tui.LabelRestoringSessions:     tui.LabelDone,
		tui.LabelReplayingScrollback:   tui.LabelDone,
		tui.LabelRunningResumeCommands: tui.LabelDone,
	})
	if got := activeLabelText(acc.View()); got != "" {
		t.Errorf("after all steps active label = %q; want none", got)
	}
}

// TestMultiStepLabelStaysActiveUntilLastStep asserts a label spanning multiple
// steps ("Registered hooks": steps 2-5) stays active until its LAST constituent
// step (5) completes — at step 4 it is still active, not yet done.
func TestMultiStepLabelStaysActiveUntilLastStep(t *testing.T) {
	acc := feed(tui.LoadingProgress{},
		tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"},
		tui.BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"},
		tui.BootstrapProgressMsg{Index: 3, Name: "SetRestoring"},
		tui.BootstrapProgressMsg{Index: 4, Name: "SweepOrphanDaemons"},
	)
	// Through step 4 (steps 2-4 of the 2-5 group done), "Registered hooks" is
	// still active because step 5 has not completed.
	if got := labelState(acc.View(), tui.LabelRegisteredHooks); got != tui.LabelActive {
		t.Errorf("Registered hooks state after step 4 = %v; want active (last step 5 not done)", got)
	}

	acc = acc.Apply(tui.BootstrapProgressMsg{Index: 5, Name: "EnsureSaver"})
	if got := labelState(acc.View(), tui.LabelRegisteredHooks); got != tui.LabelDone {
		t.Errorf("Registered hooks state after step 5 = %v; want done", got)
	}
}

// TestRestoringSessionsCounter asserts the (N/M) counter is rendered ONLY on
// "Restoring sessions" and advances N against M from the restore skeleton
// events; no other label carries a counter.
func TestRestoringSessionsCounter(t *testing.T) {
	acc := feed(tui.LoadingProgress{},
		tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"},
		tui.BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"},
		tui.BootstrapProgressMsg{Index: 3, Name: "SetRestoring"},
		tui.BootstrapProgressMsg{Index: 4, Name: "SweepOrphanDaemons"},
		tui.BootstrapProgressMsg{Index: 5, Name: "EnsureSaver"},
		tui.BootstrapProgressMsg{Index: 6, Name: "Restore", RestoreN: 1, RestoreM: 3},
	)
	v := acc.View()
	if got := counterText(v, tui.LabelRestoringSessions); got != "1/3" {
		t.Errorf("Restoring sessions counter = %q; want %q", got, "1/3")
	}
	// Mid-flight (skeleton event, M>0): step 6 is NOT yet complete, so
	// "Restoring sessions" stays the active frontier and "Replaying scrollback"
	// is still pending — the counter ticks but the step has not advanced.
	if got := labelState(v, tui.LabelRestoringSessions); got != tui.LabelActive {
		t.Errorf("mid-flight Restoring sessions state = %v; want active (step 6 not yet complete)", got)
	}
	if got := labelState(v, tui.LabelReplayingScrollback); got != tui.LabelPending {
		t.Errorf("mid-flight Replaying scrollback state = %v; want pending", got)
	}
	// Bar still at 1/11 (only step 1 completed; the skeleton event advances no step).
	if want := 5.0 / 11.0; math.Abs(v.BarFraction-want) > floatEps {
		t.Errorf("mid-flight bar fraction = %v; want %v (skeleton event must not advance step 6)", v.BarFraction, want)
	}
	// No other label carries a counter.
	for _, l := range v.Labels {
		if l.Text == tui.LabelRestoringSessions {
			continue
		}
		if l.Counter != "" {
			t.Errorf("label %q carries counter %q; only Restoring sessions may", l.Text, l.Counter)
		}
	}

	// N advances against M as later skeleton events arrive.
	acc = acc.Apply(tui.BootstrapProgressMsg{Index: 6, Name: "Restore", RestoreN: 3, RestoreM: 3})
	if got := counterText(acc.View(), tui.LabelRestoringSessions); got != "3/3" {
		t.Errorf("Restoring sessions counter after N=3 = %q; want %q", got, "3/3")
	}

	// The trailing completion tick (RestoreM==0) is what marks step 6 done:
	// "Restoring sessions" flips to done and the bar advances to 6/11. The
	// counter stays sticky at the last N/M.
	acc = acc.Apply(tui.BootstrapProgressMsg{Index: 6, Name: "Restore"})
	done := acc.View()
	if got := labelState(done, tui.LabelRestoringSessions); got != tui.LabelDone {
		t.Errorf("after completion tick Restoring sessions state = %v; want done", got)
	}
	if want := 6.0 / 11.0; math.Abs(done.BarFraction-want) > floatEps {
		t.Errorf("after completion tick bar fraction = %v; want %v", done.BarFraction, want)
	}
	if got := counterText(done, tui.LabelRestoringSessions); got != "3/3" {
		t.Errorf("after completion tick counter = %q; want %q (sticky last N/M)", got, "3/3")
	}
}

// TestEmptyRestoreSuppressesCounterAndTicksDone asserts the M=0 degenerate case:
// the restore step completes immediately with RestoreM==0, so "Restoring
// sessions" renders no (N/M) and ticks done immediately (done, not stalled),
// while the bar still advances through step 6.
func TestEmptyRestoreSuppressesCounterAndTicksDone(t *testing.T) {
	acc := feed(tui.LoadingProgress{},
		tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"},
		tui.BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"},
		tui.BootstrapProgressMsg{Index: 3, Name: "SetRestoring"},
		tui.BootstrapProgressMsg{Index: 4, Name: "SweepOrphanDaemons"},
		tui.BootstrapProgressMsg{Index: 5, Name: "EnsureSaver"},
		// Restore completes immediately, no per-session events: RestoreM==0.
		tui.BootstrapProgressMsg{Index: 6, Name: "Restore"},
	)
	v := acc.View()

	if got := counterText(v, tui.LabelRestoringSessions); got != "" {
		t.Errorf("M=0: Restoring sessions counter = %q; want empty (suppressed)", got)
	}
	if got := labelState(v, tui.LabelRestoringSessions); got != tui.LabelDone {
		t.Errorf("M=0: Restoring sessions state = %v; want done (not stalled)", got)
	}
	// Bar advanced through step 6 (6/11).
	want := 6.0 / 11.0
	if math.Abs(v.BarFraction-want) > floatEps {
		t.Errorf("M=0: bar fraction after step 6 = %v; want %v", v.BarFraction, want)
	}
}

// TestRunningResumeCommandsTicksDoneWithNoItems asserts "Running resume
// commands" (steps 8-11) ticks done once its constituent steps complete, with no
// per-item counter and no stall — the cleanup steps 8-11 fold under it.
func TestRunningResumeCommandsTicksDoneWithNoItems(t *testing.T) {
	var acc tui.LoadingProgress
	// Before step 6 even completes, "Running resume commands" is pending.
	for step := 1; step <= 5; step++ {
		acc = acc.Apply(tui.BootstrapProgressMsg{Index: step, Name: "step"})
	}
	if got := labelState(acc.View(), tui.LabelRunningResumeCommands); got != tui.LabelPending {
		t.Fatalf("Running resume commands before its group = %v; want pending", got)
	}
	// After steps 6-10 complete (the "Replaying scrollback" group done, step 11
	// pending), "Running resume commands" is the frontier — active, no per-item
	// counter, not stalled.
	for step := 6; step <= 10; step++ {
		acc = acc.Apply(tui.BootstrapProgressMsg{Index: step, Name: "step"})
	}
	if got := labelState(acc.View(), tui.LabelRunningResumeCommands); got != tui.LabelActive {
		t.Errorf("Running resume commands at step 10 = %v; want active (last step 11 not done)", got)
	}
	// Step 11: done. No counter ever.
	acc = acc.Apply(tui.BootstrapProgressMsg{Index: 11, Name: "CleanStale"})
	v := acc.View()
	if got := labelState(v, tui.LabelRunningResumeCommands); got != tui.LabelDone {
		t.Errorf("Running resume commands at step 11 = %v; want done (no per-item work)", got)
	}
	if got := counterText(v, tui.LabelRunningResumeCommands); got != "" {
		t.Errorf("Running resume commands counter = %q; want empty (no per-item counter)", got)
	}
}

// TestIdempotentPerStepIndex asserts duplicate / out-of-order step events do not
// double-advance the bar — the bar tracks distinct completed step indices.
func TestIdempotentPerStepIndex(t *testing.T) {
	// Duplicates: step 1 three times advances the bar to exactly 1/11.
	acc := feed(tui.LoadingProgress{},
		tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"},
		tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"},
		tui.BootstrapProgressMsg{Index: 1, Name: "EnsureServer"},
	)
	if got := acc.View().BarFraction; math.Abs(got-1.0/11.0) > floatEps {
		t.Errorf("3× step 1: bar = %v; want %v (no double-advance)", got, 1.0/11.0)
	}

	// Out-of-order: receiving step 3 then step 2 advances by exactly 2 distinct
	// indices (2/11), never 3/11.
	acc = feed(tui.LoadingProgress{},
		tui.BootstrapProgressMsg{Index: 3, Name: "SetRestoring"},
		tui.BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"},
	)
	if got := acc.View().BarFraction; math.Abs(got-2.0/11.0) > floatEps {
		t.Errorf("steps 3,2 out of order: bar = %v; want %v", got, 2.0/11.0)
	}
}

// TestMappingCoversAllElevenStepsNoGaps is the coverage/drift guard: the §10.4
// table must cover exactly step indices 1..11 with no gaps and no duplicate
// index, so a future bootstrap-step change cannot silently leave a step
// unmapped. Each index must resolve to one of the five canonical labels.
func TestMappingCoversAllElevenStepsNoGaps(t *testing.T) {
	valid := map[string]bool{
		tui.LabelStartedTmuxServer:     true,
		tui.LabelRegisteredHooks:       true,
		tui.LabelRestoringSessions:     true,
		tui.LabelReplayingScrollback:   true,
		tui.LabelRunningResumeCommands: true,
	}
	for step := 1; step <= 11; step++ {
		// Every step index must map to a valid §10.4 label (no gap).
		got := tui.LabelForStep(tui.BootstrapProgressMsg{Index: step, Name: "step"})
		if got == "" {
			t.Errorf("step %d resolved to no label (gap in the §10.4 mapping)", step)
			continue
		}
		if !valid[got] {
			t.Errorf("step %d resolved to unknown label %q", step, got)
		}
	}
	// Out-of-range indices must not map and must not advance the bar (defensive —
	// no phantom steps).
	for _, bad := range []int{0, 12, 99} {
		if got := tui.LabelForStep(tui.BootstrapProgressMsg{Index: bad, Name: "x"}); got != "" {
			t.Errorf("out-of-range step %d mapped to label %q; want none", bad, got)
		}
		v := feed(tui.LoadingProgress{}, tui.BootstrapProgressMsg{Index: bad, Name: "x"}).View()
		if f := v.BarFraction; f != 0 {
			t.Errorf("out-of-range step %d advanced the bar to %v; want 0", bad, f)
		}
	}

	// Exactly five labels, in stable order, every time.
	v := tui.LoadingProgress{}.View()
	if len(v.Labels) != 5 {
		t.Fatalf("View().Labels length = %d; want 5", len(v.Labels))
	}
	wantOrder := []string{
		tui.LabelStartedTmuxServer,
		tui.LabelRegisteredHooks,
		tui.LabelRestoringSessions,
		tui.LabelReplayingScrollback,
		tui.LabelRunningResumeCommands,
	}
	for i, want := range wantOrder {
		if v.Labels[i].Text != want {
			t.Errorf("label[%d] = %q; want %q", i, v.Labels[i].Text, want)
		}
	}
}

// assertStates checks each named label's state in the view against want.
func assertStates(t *testing.T, v tui.LoadingProgressView, want map[string]tui.LabelState) {
	t.Helper()
	for text, wantState := range want {
		if got := labelState(v, text); got != wantState {
			t.Errorf("label %q state = %v; want %v", text, got, wantState)
		}
	}
}

// labelState returns the state of the label with the given text, or -1 if absent.
func labelState(v tui.LoadingProgressView, text string) tui.LabelState {
	for _, l := range v.Labels {
		if l.Text == text {
			return l.State
		}
	}
	return tui.LabelState(-1)
}

// counterText returns the Counter string of the label with the given text.
func counterText(v tui.LoadingProgressView, text string) string {
	for _, l := range v.Labels {
		if l.Text == text {
			return l.Counter
		}
	}
	return ""
}
