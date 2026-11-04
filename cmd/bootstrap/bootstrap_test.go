package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
)

// stepRecorder is every seam of an Orchestrator, recording each call in
// order and answering as its fields say. Its zero value's saver is absent and
// speaks protocol 0; newRecorder's is present and speaks the Orchestrator's.
type stepRecorder struct {
	calls []string

	ZmxErr, RaiseErr, OrphansErr, FlagsErr, LowerErr, LatchErr error
	SaverUp                                                    bool
	PresentErr, EnsureErr, ProtocolErr                         error
	Speaks                                                     int
	Result                                                     restore.Result
	// Sessions is how many sessions Restore reports taking up, through the
	// progress SetProgress installed.
	Sessions int

	noSaver  []bool
	progress func(n, m int)
}

func newRecorder() *stepRecorder { return &stepRecorder{SaverUp: true, Speaks: portalProtocol} }

// portalProtocol is the protocol the tests' Orchestrator speaks.
const portalProtocol = 1

func (r *stepRecorder) record(call string) { r.calls = append(r.calls, call) }

func (r *stepRecorder) CheckZmx(context.Context) error { r.record("CheckZmx"); return r.ZmxErr }
func (r *stepRecorder) Raise() error                   { r.record("Raise"); return r.RaiseErr }
func (r *stepRecorder) Lower() error                   { r.record("Lower"); return r.LowerErr }
func (r *stepRecorder) WriteLatch() error              { r.record("WriteLatch"); return r.LatchErr }

func (r *stepRecorder) SweepOrphanDaemons(context.Context) error {
	r.record("SweepOrphanDaemons")
	return r.OrphansErr
}

func (r *stepRecorder) SweepStaleFlags(context.Context) error {
	r.record("SweepStaleFlags")
	return r.FlagsErr
}

func (r *stepRecorder) Present(context.Context) (bool, error) {
	r.record("Present")
	return r.SaverUp, r.PresentErr
}

func (r *stepRecorder) EnsureSaver(context.Context) error {
	r.record("EnsureSaver")
	return r.EnsureErr
}

func (r *stepRecorder) Protocol(context.Context) (int, error) {
	r.record("Protocol")
	return r.Speaks, r.ProtocolErr
}

func (r *stepRecorder) SetProgress(fn func(n, m int)) { r.progress = fn }

func (r *stepRecorder) Restore(_ context.Context, noSaver bool) restore.Result {
	r.record("Restore")
	r.noSaver = append(r.noSaver, noSaver)
	if r.progress != nil {
		for n := 1; n <= r.Sessions; n++ {
			r.progress(n, r.Sessions)
		}
	}
	return r.Result
}

func newOrchestrator(r *stepRecorder, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		Zmx:           r,
		Restoring:     r,
		OrphanSweeper: r,
		FlagSweeper:   r,
		Saver:         r,
		Protocol:      portalProtocol,
		Restore:       r,
		Latch:         r,
		Logger:        logger,
	}
}

// wantAllCalls is every seam call a bootstrap makes, in the order Run makes
// them.
var wantAllCalls = []string{
	"CheckZmx", "Raise", "SweepOrphanDaemons", "SweepStaleFlags",
	"Present", "EnsureSaver", "Protocol", "Restore", "Lower", "WriteLatch",
}

// stepNames is the seven steps, in order.
var stepNames = []string{"CheckZmx", "RaiseRestoring", "Sweep", "EnsureSaver", "Restore", "LowerRestoring", "WriteLatch"}

var errBoom = errors.New("boom")

var _ Runner = (*Orchestrator)(nil)

func TestOrchestratorRun_RunsTheSevenStepsInOrder(t *testing.T) {
	r := newRecorder()

	warnings, err := newOrchestrator(r, nil).Run(context.Background())

	if err != nil || len(warnings) != 0 {
		t.Fatalf("Run = %v, %v; want a clean run", warnings, err)
	}
	if !slices.Equal(r.calls, wantAllCalls) {
		t.Errorf("calls = %v, want %v", r.calls, wantAllCalls)
	}
}

func TestOrchestratorRun_FatalSteps(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stage   func(*stepRecorder)
		step    int
		message string
		// calls is every call made, the failing one last.
		calls []string
	}{
		{
			name: "checking zmx",
			stage: func(r *stepRecorder) {
				r.ZmxErr = errors.New("zmx not found on PATH; install it with: brew install neurosnap/tap/zmx")
			},
			step:    indexCheckZmx,
			message: "zmx not found on PATH; install it with: brew install neurosnap/tap/zmx",
			calls:   wantAllCalls[:1],
		},
		{
			name:    "raising the restoring flag",
			stage:   func(r *stepRecorder) { r.RaiseErr = errBoom },
			step:    indexRaiseRestoring,
			message: "Portal failed to raise the restoring flag: boom",
			calls:   wantAllCalls[:2],
		},
		{
			name:    "a saver speaking another protocol, before restore creates anything",
			stage:   func(r *stepRecorder) { r.Speaks = 2 },
			step:    indexEnsureSaver,
			message: "Portal can't use this zmx: its sessions speak protocol 2, and Portal speaks 1. Upgrade Portal, or install the zmx release it supports.",
			calls:   wantAllCalls[:7],
		},
		{
			name:    "lowering the restoring flag",
			stage:   func(r *stepRecorder) { r.LowerErr = errBoom },
			step:    indexLowerRestoring,
			message: "Portal failed to lower the restoring flag: boom",
			calls:   wantAllCalls[:9],
		},
	} {
		t.Run(tc.name+" is fatal, and nothing runs after it, the latch least of all", func(t *testing.T) {
			r := newRecorder()
			tc.stage(r)
			sink := &logtest.Sink{}

			_, err := newOrchestrator(r, slog.New(sink)).Run(context.Background())

			fatal, ok := errors.AsType[*FatalError](err)
			if !ok {
				t.Fatalf("Run = %v, want a *FatalError", err)
			}
			if fatal.UserMessage != tc.message || fatal.Step != tc.step {
				t.Errorf("fatal = %q at step %d, want %q at step %d", fatal.UserMessage, fatal.Step, tc.message, tc.step)
			}
			if !slices.Equal(r.calls, tc.calls) {
				t.Errorf("calls = %v, want %v", r.calls, tc.calls)
			}
			rec := sink.Records().WithMessage("step failed").AtExactLevel(slog.LevelError).Only(t, "the fatal")
			if got := rec.AttrString(t, "step"); got != stepNames[tc.step-1] {
				t.Errorf("fatal logged for step %q, want %q", got, stepNames[tc.step-1])
			}
			if got := sink.Records().WithMessage("orchestration complete"); len(got) != 0 {
				t.Errorf("a fatal run logged %+v, want no summary", got)
			}
		})
	}
}

func TestOrchestratorRun_SoftFailuresRunEveryStep(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stage    func(*stepRecorder)
		warnings []Warning
	}{
		{"the orphan sweep failing", func(r *stepRecorder) { r.OrphansErr = errBoom }, nil},
		{"the flag sweep failing", func(r *stepRecorder) { r.FlagsErr = errBoom }, nil},
		{"the saver's presence unread", func(r *stepRecorder) { r.PresentErr = errBoom }, nil},
		{"the saver failing to start", func(r *stepRecorder) { r.EnsureErr = errBoom }, []Warning{SaverDownWarning()}},
		{"the saver's protocol unread", func(r *stepRecorder) { r.ProtocolErr = errBoom }, nil},
		{"the latch unwritten", func(r *stepRecorder) { r.LatchErr = errBoom }, nil},
	} {
		t.Run(tc.name+" logs a WARN, and every step runs", func(t *testing.T) {
			r := newRecorder()
			tc.stage(r)
			sink := &logtest.Sink{}

			warnings, err := newOrchestrator(r, slog.New(sink)).Run(context.Background())

			if err != nil {
				t.Fatalf("Run = %v, want no fatal", err)
			}
			if !slices.Equal(r.calls, wantAllCalls) {
				t.Errorf("calls = %v, want %v", r.calls, wantAllCalls)
			}
			if !equalWarnings(warnings, tc.warnings) {
				t.Errorf("warnings = %v, want %v", warnings, tc.warnings)
			}
			if !errors.Is(sink.Records().WithMessage("step failed").AtExactLevel(slog.LevelWarn).Only(t, "the failure").ErrorAttr(t, "error"), errBoom) {
				t.Error("the WARN does not carry the failure")
			}
		})
	}
}

func TestOrchestratorRun_RestoreWarnings(t *testing.T) {
	r := newRecorder()
	r.EnsureErr = errBoom
	r.Result = restore.Result{IndexErr: errBoom, ListErr: errBoom, Restored: 1, Failed: 2}

	warnings, err := newOrchestrator(r, nil).Run(context.Background())

	if err != nil {
		t.Fatalf("Run = %v, want no fatal", err)
	}
	want := []Warning{SaverDownWarning(), CorruptSessionsJSONWarning(), SessionsUnlistedWarning(), UnrestoredWarning(2)}
	if !equalWarnings(warnings, want) {
		t.Errorf("warnings = %v, want each step's in step order, %v", warnings, want)
	}
}

func TestOrchestratorRun_NoSaverRan(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage func(*stepRecorder)
		want  bool
	}{
		{"a saver found running", func(*stepRecorder) {}, false},
		{"no saver found", func(r *stepRecorder) { r.SaverUp = false }, true},
		{"a saver that couldn't be told, which may have watched sessions end", func(r *stepRecorder) { r.PresentErr = errBoom }, false},
		{"a saver found running that then failed to start again", func(r *stepRecorder) { r.EnsureErr = errBoom }, false},
	} {
		t.Run("restore is told no saver ran exactly when step 4 found none: "+tc.name, func(t *testing.T) {
			r := newRecorder()
			tc.stage(r)

			if _, err := newOrchestrator(r, nil).Run(context.Background()); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if !slices.Equal(r.noSaver, []bool{tc.want}) {
				t.Errorf("Restore's noSaver = %v, want [%v]", r.noSaver, tc.want)
			}
		})
	}
}

func TestOrchestratorRun_Logs(t *testing.T) {
	t.Run("it logs each step entering and complete, in order, then one summary", func(t *testing.T) {
		r := newRecorder()
		r.EnsureErr = errBoom
		sink := &logtest.Sink{}

		if _, err := newOrchestrator(r, slog.New(sink)).Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}

		for msg, level := range map[string]slog.Level{"step entering": slog.LevelDebug, "step complete": slog.LevelInfo} {
			var got []string
			for _, rec := range sink.Records().WithMessage(msg).AtExactLevel(level) {
				got = append(got, rec.AttrString(t, "step"))
			}
			if !slices.Equal(got, stepNames) {
				t.Errorf("%q steps = %v, want %v", msg, got, stepNames)
			}
		}
		summary := sink.Records().WithMessage("orchestration complete").AtExactLevel(slog.LevelInfo).Only(t, "the summary")
		if got := summary.IntAttr(t, "steps"); got != int64(len(stepNames)) {
			t.Errorf("steps = %d, want %d", got, len(stepNames))
		}
		if got := summary.IntAttr(t, "warnings"); got != 1 {
			t.Errorf("warnings = %d, want 1", got)
		}
		summary.RequireDuration(t, "took")
	})
}

func TestOrchestratorRun_Progress(t *testing.T) {
	t.Run("it emits each step as it completes, the restore's sessions between, numbered from 1", func(t *testing.T) {
		r := newRecorder()
		r.Sessions = 2
		var got []StepEvent
		ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) { got = append(got, ev) })

		if _, err := newOrchestrator(r, nil).Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}

		want := []StepEvent{
			{Index: 1, Name: "CheckZmx"},
			{Index: 2, Name: "RaiseRestoring"},
			{Index: 3, Name: "Sweep"},
			{Index: 4, Name: "EnsureSaver"},
			{Index: 5, Name: "Restore", RestoreN: 1, RestoreM: 2},
			{Index: 5, Name: "Restore", RestoreN: 2, RestoreM: 2},
			{Index: 5, Name: "Restore"},
			{Index: 6, Name: "LowerRestoring"},
			{Index: 7, Name: "WriteLatch"},
		}
		if !slices.Equal(got, want) {
			t.Errorf("events = %+v\nwant %+v", got, want)
		}
	})

	t.Run("a fatal step emits nothing for itself or after it", func(t *testing.T) {
		r := newRecorder()
		r.RaiseErr = errBoom
		var got []StepEvent
		ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) { got = append(got, ev) })

		if _, err := newOrchestrator(r, nil).Run(ctx); err == nil {
			t.Fatal("Run = nil, want the fatal")
		}

		if want := []StepEvent{{Index: 1, Name: "CheckZmx"}}; !slices.Equal(got, want) {
			t.Errorf("events = %+v, want %+v", got, want)
		}
	})

	t.Run("with no emitter, restore reports no progress", func(t *testing.T) {
		r := newRecorder()

		if _, err := newOrchestrator(r, nil).Run(context.Background()); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if r.progress != nil {
			t.Error("SetProgress was called on the synchronous route")
		}
	})
}

func equalWarnings(got, want []Warning) bool {
	return slices.EqualFunc(got, want, func(a, b Warning) bool { return strings.Join(a.Lines, "\n") == strings.Join(b.Lines, "\n") })
}
