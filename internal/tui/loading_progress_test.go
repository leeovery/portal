package tui_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/leeovery/portal/internal/tui"
)

const floatEps = 1e-9

// bootstrapSteps is how many steps the bootstrap reports, as cmd/bootstrap
// numbers them.
const bootstrapSteps = 6

func feed(acc tui.LoadingProgress, events ...tui.BootstrapProgressMsg) tui.LoadingProgress {
	for _, e := range events {
		acc = acc.Apply(e)
	}
	return acc
}

// through feeds steps 1 to last, each completing.
func through(last int) tui.LoadingProgress {
	var acc tui.LoadingProgress
	for step := 1; step <= last; step++ {
		acc = acc.Apply(tui.BootstrapProgressMsg{Index: step})
	}
	return acc
}

func activeLabelText(v tui.LoadingProgressView) string {
	for _, l := range v.Labels {
		if l.State == tui.LabelActive {
			return l.Text
		}
	}
	return ""
}

func TestStepMapsToFriendlyLabel(t *testing.T) {
	cases := []struct {
		name      string
		event     tui.BootstrapProgressMsg
		wantLabel string
	}{
		{"step 1 CheckZmx", tui.BootstrapProgressMsg{Index: 1}, tui.LabelCheckingZmx},
		{"step 2 RemoveLatch", tui.BootstrapProgressMsg{Index: 2}, tui.LabelStartingTheSaver},
		{"step 3 Sweep", tui.BootstrapProgressMsg{Index: 3}, tui.LabelStartingTheSaver},
		{"step 4 EnsureSaver", tui.BootstrapProgressMsg{Index: 4}, tui.LabelStartingTheSaver},
		{"step 5 Restore, a session (M>0)", tui.BootstrapProgressMsg{Index: 5, RestoreN: 1, RestoreM: 3}, tui.LabelRestoringSessions},
		{"step 5 Restore complete (M==0)", tui.BootstrapProgressMsg{Index: 5}, tui.LabelRestoringSessions},
		{"step 6 WriteLatch", tui.BootstrapProgressMsg{Index: 6}, tui.LabelRestoringSessions},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tui.LabelForStep(tc.event); got != tc.wantLabel {
				t.Errorf("LabelForStep(step %d) = %q; want %q", tc.event.Index, got, tc.wantLabel)
			}
		})
	}
}

func TestBarAdvancesEveryStep(t *testing.T) {
	acc := tui.LoadingProgress{}
	if f := acc.View().BarFraction; f != 0 {
		t.Fatalf("initial bar fraction = %v; want 0", f)
	}
	for step := 1; step <= bootstrapSteps; step++ {
		acc = acc.Apply(tui.BootstrapProgressMsg{Index: step})
		want := float64(step) / bootstrapSteps
		got := acc.View().BarFraction
		if math.Abs(got-want) > floatEps {
			t.Errorf("after step %d bar fraction = %v; want %v", step, got, want)
		}
		if step < bootstrapSteps && got >= 1.0 {
			t.Errorf("bar reached 100%% at step %d; must reach 100%% only after step %d", step, bootstrapSteps)
		}
	}
	if got := acc.View().BarFraction; math.Abs(got-1.0) > floatEps {
		t.Errorf("after the last step bar fraction = %v; want 1.0", got)
	}
}

func TestLabelStateTransitions(t *testing.T) {
	assertStates(t, tui.LoadingProgress{}.View(), map[string]tui.LabelState{
		tui.LabelCheckingZmx:       tui.LabelPending,
		tui.LabelStartingTheSaver:  tui.LabelPending,
		tui.LabelRestoringSessions: tui.LabelPending,
	})

	assertStates(t, through(1).View(), map[string]tui.LabelState{
		tui.LabelCheckingZmx:       tui.LabelDone,
		tui.LabelStartingTheSaver:  tui.LabelActive,
		tui.LabelRestoringSessions: tui.LabelPending,
	})

	assertStates(t, through(4).View(), map[string]tui.LabelState{
		tui.LabelCheckingZmx:       tui.LabelDone,
		tui.LabelStartingTheSaver:  tui.LabelDone,
		tui.LabelRestoringSessions: tui.LabelActive,
	})

	done := through(bootstrapSteps).View()
	assertStates(t, done, map[string]tui.LabelState{
		tui.LabelCheckingZmx:       tui.LabelDone,
		tui.LabelStartingTheSaver:  tui.LabelDone,
		tui.LabelRestoringSessions: tui.LabelDone,
	})
	if got := activeLabelText(done); got != "" {
		t.Errorf("after all steps active label = %q; want none", got)
	}
}

func TestWaitingLabel(t *testing.T) {
	waited := tui.BootstrapProgressMsg{Waiting: true}

	t.Run("absent until the bootstrap says it waits", func(t *testing.T) {
		if got := labelState(through(1).View(), tui.LabelWaiting); got != tui.LabelState(-1) {
			t.Errorf("waiting label state = %v; want no waiting label", got)
		}
	})

	t.Run("leads the labels, the only one active, until the first step", func(t *testing.T) {
		v := feed(tui.LoadingProgress{}, waited).View()
		if v.Labels[0].Text != tui.LabelWaiting {
			t.Errorf("first label = %q; want %q", v.Labels[0].Text, tui.LabelWaiting)
		}
		assertStates(t, v, map[string]tui.LabelState{
			tui.LabelWaiting:           tui.LabelActive,
			tui.LabelCheckingZmx:       tui.LabelPending,
			tui.LabelStartingTheSaver:  tui.LabelPending,
			tui.LabelRestoringSessions: tui.LabelPending,
		})
		if v.BarFraction != 0 {
			t.Errorf("bar fraction = %v; want 0: waiting is no step", v.BarFraction)
		}
	})

	t.Run("done once a step completes", func(t *testing.T) {
		v := feed(tui.LoadingProgress{}, waited, tui.BootstrapProgressMsg{Index: 1}).View()
		assertStates(t, v, map[string]tui.LabelState{
			tui.LabelWaiting:          tui.LabelDone,
			tui.LabelCheckingZmx:      tui.LabelDone,
			tui.LabelStartingTheSaver: tui.LabelActive,
		})
	})
}

func TestMultiStepLabelStaysActiveUntilLastStep(t *testing.T) {
	if got := labelState(through(3).View(), tui.LabelStartingTheSaver); got != tui.LabelActive {
		t.Errorf("Starting the saver after step 3 = %v; want active (its last step, 4, not done)", got)
	}
	if got := labelState(through(4).View(), tui.LabelStartingTheSaver); got != tui.LabelDone {
		t.Errorf("Starting the saver after step 4 = %v; want done", got)
	}
	if got := labelState(through(5).View(), tui.LabelRestoringSessions); got != tui.LabelActive {
		t.Errorf("Restoring sessions after step 5 = %v; want active (the latch, step 6, not written)", got)
	}
}

func TestRestoringSessionsCounter(t *testing.T) {
	acc := through(4).Apply(tui.BootstrapProgressMsg{Index: 5, RestoreN: 1, RestoreM: 3})
	v := acc.View()
	if got := counterText(v, tui.LabelRestoringSessions); got != "1/3" {
		t.Errorf("Restoring sessions counter = %q; want %q", got, "1/3")
	}
	if got := labelState(v, tui.LabelRestoringSessions); got != tui.LabelActive {
		t.Errorf("mid-flight Restoring sessions state = %v; want active (step 5 not yet complete)", got)
	}
	if want := 4.0 / bootstrapSteps; math.Abs(v.BarFraction-want) > floatEps {
		t.Errorf("mid-flight bar fraction = %v; want %v (a session's event must not complete step 5)", v.BarFraction, want)
	}
	for _, l := range v.Labels {
		if l.Text != tui.LabelRestoringSessions && l.Counter != "" {
			t.Errorf("label %q carries counter %q; only Restoring sessions may", l.Text, l.Counter)
		}
	}

	acc = acc.Apply(tui.BootstrapProgressMsg{Index: 5, RestoreN: 3, RestoreM: 3})
	if got := counterText(acc.View(), tui.LabelRestoringSessions); got != "3/3" {
		t.Errorf("Restoring sessions counter after N=3 = %q; want %q", got, "3/3")
	}

	acc = acc.Apply(tui.BootstrapProgressMsg{Index: 5})
	done := acc.View()
	if want := 5.0 / bootstrapSteps; math.Abs(done.BarFraction-want) > floatEps {
		t.Errorf("after the restore's completion bar fraction = %v; want %v", done.BarFraction, want)
	}
	if got := counterText(done, tui.LabelRestoringSessions); got != "3/3" {
		t.Errorf("after the restore's completion counter = %q; want %q (sticky last N/M)", got, "3/3")
	}
}

func TestEmptyRestoreSuppressesCounterAndTicksDone(t *testing.T) {
	v := through(bootstrapSteps).View()

	if got := counterText(v, tui.LabelRestoringSessions); got != "" {
		t.Errorf("M=0: Restoring sessions counter = %q; want empty (suppressed)", got)
	}
	if got := labelState(v, tui.LabelRestoringSessions); got != tui.LabelDone {
		t.Errorf("M=0: Restoring sessions state = %v; want done (not stalled)", got)
	}
}

func TestIdempotentPerStepIndex(t *testing.T) {
	acc := feed(tui.LoadingProgress{},
		tui.BootstrapProgressMsg{Index: 1},
		tui.BootstrapProgressMsg{Index: 1},
		tui.BootstrapProgressMsg{Index: 1},
	)
	if got := acc.View().BarFraction; math.Abs(got-1.0/bootstrapSteps) > floatEps {
		t.Errorf("3× step 1: bar = %v; want %v (no double-advance)", got, 1.0/bootstrapSteps)
	}

	acc = feed(tui.LoadingProgress{},
		tui.BootstrapProgressMsg{Index: 3},
		tui.BootstrapProgressMsg{Index: 2},
	)
	if got := acc.View().BarFraction; math.Abs(got-2.0/bootstrapSteps) > floatEps {
		t.Errorf("steps 3,2 out of order: bar = %v; want %v", got, 2.0/bootstrapSteps)
	}
}

func TestMappingCoversTheSixStepsNoGaps(t *testing.T) {
	valid := map[string]bool{
		tui.LabelCheckingZmx:       true,
		tui.LabelStartingTheSaver:  true,
		tui.LabelRestoringSessions: true,
	}
	for step := 1; step <= bootstrapSteps; step++ {
		got := tui.LabelForStep(tui.BootstrapProgressMsg{Index: step})
		if got == "" {
			t.Errorf("step %d resolved to no label (gap in the step→label mapping)", step)
			continue
		}
		if !valid[got] {
			t.Errorf("step %d resolved to unknown label %q", step, got)
		}
	}
	for _, bad := range []int{0, 7, 10, 99} {
		if got := tui.LabelForStep(tui.BootstrapProgressMsg{Index: bad}); got != "" {
			t.Errorf("out-of-range step %d mapped to label %q; want none", bad, got)
		}
		v := feed(tui.LoadingProgress{}, tui.BootstrapProgressMsg{Index: bad}).View()
		if f := v.BarFraction; f != 0 {
			t.Errorf("out-of-range step %d advanced the bar to %v; want 0", bad, f)
		}
	}

	v := tui.LoadingProgress{}.View()
	wantOrder := []string{tui.LabelCheckingZmx, tui.LabelStartingTheSaver, tui.LabelRestoringSessions}
	if len(v.Labels) != len(wantOrder) {
		t.Fatalf("View().Labels length = %d; want %d", len(v.Labels), len(wantOrder))
	}
	for i, want := range wantOrder {
		if v.Labels[i].Text != want {
			t.Errorf("label[%d] = %q; want %q", i, v.Labels[i].Text, want)
		}
	}
}

func TestFailedViewFailsTheStepsLabel(t *testing.T) {
	t.Run("a step's fatal fails its label", func(t *testing.T) {
		v := through(1).FailedView(4, "boom")
		if got := labelState(v, tui.LabelStartingTheSaver); got != tui.LabelFailed {
			t.Errorf("Starting the saver = %v; want failed", got)
		}
		if v.Message != "boom" {
			t.Errorf("Message = %q; want the fatal's", v.Message)
		}
	})

	t.Run("a fatal before any step fails no label", func(t *testing.T) {
		v := tui.LoadingProgress{}.FailedView(0, "boom")
		for _, l := range v.Labels {
			if l.State == tui.LabelFailed {
				t.Errorf("label %q failed, want none", l.Text)
			}
		}
	})
}

func TestBootstrapProgressMsgCarriesOnlyConsumedFields(t *testing.T) {
	want := map[string]bool{"Index": true, "RestoreN": true, "RestoreM": true, "Waiting": true}
	tp := reflect.TypeFor[tui.BootstrapProgressMsg]()
	got := map[string]bool{}
	for field := range tp.Fields() {
		got[field.Name] = true
	}
	for name := range got {
		if !want[name] {
			t.Errorf("BootstrapProgressMsg carries field %q — only Index/RestoreN/RestoreM/Waiting may ride the wire (the label mapping is loading_progress.go's sole authority)", name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("BootstrapProgressMsg is missing the consumed field %q", name)
		}
	}
}

func assertStates(t *testing.T, v tui.LoadingProgressView, want map[string]tui.LabelState) {
	t.Helper()
	for text, wantState := range want {
		if got := labelState(v, text); got != wantState {
			t.Errorf("label %q state = %v; want %v", text, got, wantState)
		}
	}
}

func labelState(v tui.LoadingProgressView, text string) tui.LabelState {
	for _, l := range v.Labels {
		if l.Text == text {
			return l.State
		}
	}
	return tui.LabelState(-1)
}

func counterText(v tui.LoadingProgressView, text string) string {
	for _, l := range v.Labels {
		if l.Text == text {
			return l.Counter
		}
	}
	return ""
}
