package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/saverhost"
)

// answer is one reading of the saver's protocol.
type answer struct {
	speaks int
	err    error
}

// stepRecorder is every seam of an Orchestrator, recording each call in
// order and answering as its fields say. newRecorder's saver is present, and
// kept, and speaks the Orchestrator's protocol.
type stepRecorder struct {
	calls []string

	ZmxErr, RaiseErr, OrphansErr, FlagsErr, LowerErr, LatchErr error
	// SaverUp is a saver found present; Recycles has EnsureSaver make a new
	// one in its place, as one that is absent always is.
	SaverUp, Recycles bool
	// CreateErr fails EnsureSaver's create, so it makes no saver; EnsureErr
	// fails it after.
	PresentErr, CreateErr, EnsureErr, MarkErr, EndErr error
	// Answers are the protocol's readings in turn, the last repeating.
	Answers []answer
	Result  restore.Result
	// Sessions is how many sessions Restore reports taking up, through the
	// progress SetProgress installed.
	Sessions int
	// DuringRestore runs inside Restore.
	DuringRestore func()

	protocolReads int
	progress      func(n, m int)
}

func newRecorder() *stepRecorder {
	return &stepRecorder{SaverUp: true, Answers: []answer{{speaks: portalProtocol}}}
}

// portalProtocol is the protocol the tests' Orchestrator speaks.
const portalProtocol = 1

func (r *stepRecorder) record(call string) { r.calls = append(r.calls, call) }

func (r *stepRecorder) called(call string) bool { return slices.Contains(r.calls, call) }

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

func (r *stepRecorder) EnsureSaver(ctx context.Context, made func(context.Context) error) error {
	r.record("EnsureSaver")
	if r.SaverUp && !r.Recycles {
		return r.EnsureErr
	}
	if r.CreateErr != nil {
		return r.CreateErr
	}
	r.record("Create")
	if err := made(ctx); err != nil {
		return err
	}
	return r.EnsureErr
}

func (r *stepRecorder) Protocol(context.Context) (int, error) {
	r.record("Protocol")
	a := r.Answers[min(r.protocolReads, len(r.Answers)-1)]
	r.protocolReads++
	return a.speaks, a.err
}

func (r *stepRecorder) End(context.Context) error { r.record("End"); return r.EndErr }

func (r *stepRecorder) MarkOwed(context.Context) error { r.record("MarkOwed"); return r.MarkErr }

func (r *stepRecorder) SetProgress(fn func(n, m int)) { r.progress = fn }

func (r *stepRecorder) Restore(context.Context) restore.Result {
	r.record("Restore")
	if r.progress != nil {
		for n := 1; n <= r.Sessions; n++ {
			r.progress(n, r.Sessions)
		}
	}
	if r.DuringRestore != nil {
		r.DuringRestore()
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

// wantAllCalls is every seam call a bootstrap that finds its saver running
// makes, in the order Run makes them.
var wantAllCalls = []string{
	"CheckZmx", "Raise", "SweepOrphanDaemons", "SweepStaleFlags",
	"Present", "Protocol", "EnsureSaver", "Restore", "Lower", "WriteLatch",
}

// stepNames is the seven steps, in order.
var stepNames = []string{"CheckZmx", "RaiseRestoring", "Sweep", "EnsureSaver", "Restore", "LowerRestoring", "WriteLatch"}

var errBoom = errors.New("boom")

var _ Runner = (*Orchestrator)(nil)

// runPass runs r's Orchestrator, failing the test on a fatal.
func runPass(t *testing.T, r *stepRecorder, sink *logtest.Sink) []Warning {
	t.Helper()
	warnings, err := newOrchestrator(r, slog.New(sink)).Run(context.Background())
	if err != nil {
		t.Fatalf("Run = %v, want no fatal", err)
	}
	return warnings
}

// fatalOf runs r's Orchestrator, failing the test unless it ends in a fatal.
func fatalOf(t *testing.T, r *stepRecorder) *FatalError {
	t.Helper()
	_, err := newOrchestrator(r, nil).Run(context.Background())
	fatal, ok := errors.AsType[*FatalError](err)
	if !ok {
		t.Fatalf("Run = %v, want a *FatalError", err)
	}
	return fatal
}

func TestOrchestratorRun_RunsTheSevenStepsInOrder(t *testing.T) {
	t.Run("over a saver found running, it judges the saver before it ensures it", func(t *testing.T) {
		r := newRecorder()

		if warnings := runPass(t, r, &logtest.Sink{}); len(warnings) != 0 {
			t.Errorf("warnings = %v, want none", warnings)
		}
		if !slices.Equal(r.calls, wantAllCalls) {
			t.Errorf("calls = %v, want %v", r.calls, wantAllCalls)
		}
	})

	// decision 62: none saw the sessions end, so they are marked owed before
	// any saver can commit, and decision 63: the saver made is judged before
	// restore creates anything.
	t.Run("finding no saver, it marks the sessions owed before it makes one, and judges the one it makes", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false

		runPass(t, r, &logtest.Sink{})

		want := []string{
			"CheckZmx", "Raise", "SweepOrphanDaemons", "SweepStaleFlags",
			"Present", "MarkOwed", "EnsureSaver", "Create", "Protocol", "Restore", "Lower", "WriteLatch",
		}
		if !slices.Equal(r.calls, want) {
			t.Errorf("calls = %v, want %v", r.calls, want)
		}
	})

	t.Run("a saver it recycles is judged again, as made", func(t *testing.T) {
		r := newRecorder()
		r.Recycles = true

		runPass(t, r, &logtest.Sink{})

		if got := r.protocolReads; got != 2 {
			t.Errorf("protocol read %d times, want the saver found and the one made", got)
		}
	})
}

// decision 62: only a saver's absence proves none saw the sessions end.
func TestOrchestratorRun_MarksOwedOnlyWhenItFindsNoSaver(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage func(*stepRecorder)
		want  bool
	}{
		{"a saver found running", func(*stepRecorder) {}, false},
		{"no saver found", func(r *stepRecorder) { r.SaverUp = false }, true},
		{"a saver that couldn't be told, which may have watched sessions end", func(r *stepRecorder) { r.PresentErr = errBoom }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRecorder()
			tc.stage(r)

			runPass(t, r, &logtest.Sink{})

			if got := r.called("MarkOwed"); got != tc.want {
				t.Errorf("MarkOwed called = %v, want %v", got, tc.want)
			}
		})
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
			name:    "checking zmx",
			stage:   func(r *stepRecorder) { r.ZmxErr = errors.New("zmx not found on PATH") },
			step:    indexCheckZmx,
			message: "zmx not found on PATH",
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
			name:    "a saver speaking another protocol, before anything is made",
			stage:   func(r *stepRecorder) { r.Answers = []answer{{speaks: 2}} },
			step:    indexEnsureSaver,
			message: "Portal can't use this zmx: its sessions speak protocol 2, and Portal speaks 1. Upgrade Portal, or install the zmx release it supports.",
			calls:   wantAllCalls[:6],
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

// olderZmx is the fatal of a saver answering zmx 0.8.1's framing.
const olderZmx = "Portal can't use this zmx: it speaks zmx 0.8.1's protocol, which Portal doesn't support. Install a zmx build newer than 0.8.1."

// decision 63: the saver is judged before restore creates anything, with four
// outcomes.
func TestOrchestratorRun_JudgesTheSaversProtocol(t *testing.T) {
	older := answer{err: fmt.Errorf("%w: no answer in 1s", ErrOlderZmx)}
	another := answer{speaks: 2}
	neither := answer{err: errBoom}

	for _, tc := range []struct {
		name    string
		found   bool
		verdict answer
		message string
	}{
		{"a saver found that speaks another protocol", true, another, "Portal can't use this zmx: its sessions speak protocol 2, and Portal speaks 1. Upgrade Portal, or install the zmx release it supports."},
		{"a saver found that answers zmx 0.8.1's framing", true, older, olderZmx},
		{"a saver made that speaks another protocol", false, another, "Portal can't use this zmx: its sessions speak protocol 2, and Portal speaks 1. Upgrade Portal, or install the zmx release it supports."},
		{"a saver made that answers zmx 0.8.1's framing", false, older, olderZmx},
	} {
		t.Run(tc.name+" is fatal before restore creates anything", func(t *testing.T) {
			r := newRecorder()
			r.SaverUp = tc.found
			r.Answers = []answer{tc.verdict}

			fatal := fatalOf(t, r)

			if fatal.UserMessage != tc.message || fatal.Step != indexEnsureSaver {
				t.Errorf("fatal = %q at step %d, want %q at step %d", fatal.UserMessage, fatal.Step, tc.message, indexEnsureSaver)
			}
			if r.called("Restore") || r.called("Lower") || r.called("WriteLatch") {
				t.Errorf("calls = %v, want no restore, the flag left raised and no latch", r.calls)
			}
			if tc.found && (r.called("Create") || r.called("EnsureSaver") || r.called("End")) {
				t.Errorf("calls = %v, want nothing made, and the saver found left as it is", r.calls)
			}
			if !tc.found && !r.called("End") {
				t.Errorf("calls = %v, want the saver it made ended", r.calls)
			}
		})
	}

	t.Run("a saver made it can't end is left, and the fatal's cause says so", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false
		r.Answers = []answer{older}
		r.EndErr = errors.New("zmx kill: no answer")

		fatal := fatalOf(t, r)

		if !strings.Contains(fatal.Cause.Error(), "left: zmx kill: no answer") || fatal.UserMessage != olderZmx {
			t.Errorf("fatal = %q, cause %v; want the older-zmx fatal, its cause naming the saver left", fatal.UserMessage, fatal.Cause)
		}
	})

	t.Run("a saver it recycles that answers zmx 0.8.1's framing is fatal, and ended", func(t *testing.T) {
		r := newRecorder()
		r.Recycles = true
		r.Answers = []answer{{speaks: portalProtocol}, older}

		if fatal := fatalOf(t, r); fatal.UserMessage != olderZmx || !r.called("End") || r.called("Restore") {
			t.Errorf("fatal = %q, calls %v; want the older-zmx fatal, the made saver ended, no restore", fatal.UserMessage, r.calls)
		}
	})

	for name, found := range map[string]bool{"found": true, "made": false} {
		t.Run("a saver "+name+" that answers neither leaves the restore unfinished, with nothing more made", func(t *testing.T) {
			r := newRecorder()
			r.SaverUp = found
			r.Answers = []answer{neither}
			sink := &logtest.Sink{}

			warnings := runPass(t, r, sink)

			if !equalWarnings(warnings, []Warning{UnjudgedWarning()}) {
				t.Errorf("warnings = %v, want the can't-judge warning", warnings)
			}
			assertUnfinished(t, r, sink)
			if r.called("End") {
				t.Errorf("calls = %v, want a saver Portal can't judge left be", r.calls)
			}
			if found && r.called("EnsureSaver") {
				t.Errorf("calls = %v, want nothing ensured over a saver it can't judge", r.calls)
			}
		})
	}

	t.Run("no saver to judge, its create failing, leaves the restore unfinished", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false
		r.CreateErr = errBoom
		sink := &logtest.Sink{}

		warnings := runPass(t, r, sink)

		if !equalWarnings(warnings, []Warning{SaverDownWarning(), UnjudgedWarning()}) {
			t.Errorf("warnings = %v, want the saver's and the can't-judge warning", warnings)
		}
		assertUnfinished(t, r, sink)
	})

	t.Run("a saver judged Portal's whose daemon then fails to come up still restores", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false
		r.EnsureErr = fmt.Errorf("%w within 10s", saverhost.ErrNotReady)

		warnings := runPass(t, r, &logtest.Sink{})

		if !equalWarnings(warnings, []Warning{SaverDownWarning()}) || !r.called("Restore") || !r.called("WriteLatch") {
			t.Errorf("warnings = %v, calls %v; want the saver's warning, and the restore finished", warnings, r.calls)
		}
	})
}

// assertUnfinished checks a run left the restore unfinished: nothing
// restored, the restoring flag raised, no latch, and each step skipped logged.
func assertUnfinished(t *testing.T, r *stepRecorder, sink *logtest.Sink) {
	t.Helper()
	if r.called("Lower") || r.called("WriteLatch") {
		t.Errorf("calls = %v, want the restoring flag left raised and no latch written", r.calls)
	}
	var skipped []string
	for _, rec := range sink.Records().WithMessage("step skipped").AtExactLevel(slog.LevelInfo) {
		skipped = append(skipped, rec.AttrString(t, "step"))
	}
	if want := []string{"LowerRestoring", "WriteLatch"}; !slices.Equal(skipped[max(len(skipped)-2, 0):], want) {
		t.Errorf("steps skipped = %v, want them to end %v", skipped, want)
	}
}

// decision 61: only a bootstrap that finished its restore lowers the flag, so
// the next command finishes one cut short.
func TestOrchestratorRun_LeavesAnUnfinishedRestoreForTheNextCommand(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stage    func(*stepRecorder, context.CancelFunc)
		warnings []Warning
		restored bool
	}{
		{"sessions it can't mark owed", func(r *stepRecorder, _ context.CancelFunc) { r.SaverUp, r.MarkErr = false, errBoom }, []Warning{OwedUnmarkedWarning()}, false},
		{"a listing restore can't read", func(r *stepRecorder, _ context.CancelFunc) {
			r.Result = restore.Result{ListErr: errBoom}
		}, []Warning{SessionsUnlistedWarning()}, true},
		{"its context ending while restore runs", func(r *stepRecorder, cancel context.CancelFunc) { r.DuringRestore = cancel }, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRecorder()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tc.stage(r, cancel)
			sink := &logtest.Sink{}

			warnings, err := newOrchestrator(r, slog.New(sink)).Run(ctx)

			if err != nil {
				t.Fatalf("Run = %v, want no fatal", err)
			}
			if !equalWarnings(warnings, tc.warnings) {
				t.Errorf("warnings = %v, want %v", warnings, tc.warnings)
			}
			if r.called("Restore") != tc.restored {
				t.Errorf("calls = %v, want Restore called %v", r.calls, tc.restored)
			}
			if !tc.restored && (r.called("EnsureSaver") || r.called("Create")) {
				t.Errorf("calls = %v, want no saver made over sessions it couldn't mark owed", r.calls)
			}
			assertUnfinished(t, r, sink)
		})
	}
}

func TestOrchestratorRun_SoftFailuresStillFinish(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stage    func(*stepRecorder)
		warnings []Warning
	}{
		{"the orphan sweep failing", func(r *stepRecorder) { r.OrphansErr = errBoom }, nil},
		{"the flag sweep failing", func(r *stepRecorder) { r.FlagsErr = errBoom }, nil},
		{"the saver's presence unread", func(r *stepRecorder) { r.PresentErr = errBoom }, nil},
		{"the saver failing to start", func(r *stepRecorder) { r.EnsureErr = errBoom }, []Warning{SaverDownWarning()}},
		{"the saver unread", func(r *stepRecorder) { r.EnsureErr = fmt.Errorf("%w: %w", saverhost.ErrUnread, errBoom) }, []Warning{SaverUnreadWarning()}},
		{"the old saver not replaced", func(r *stepRecorder) {
			r.EnsureErr = fmt.Errorf("recycle the saver: %w: %w", saverhost.ErrNotReplaced, errBoom)
		}, []Warning{SaverNotReplacedWarning()}},
		{"the latch unwritten", func(r *stepRecorder) { r.LatchErr = errBoom }, []Warning{LatchUnwrittenWarning()}},
		{"this boot's id unread", func(r *stepRecorder) { r.LatchErr = fmt.Errorf("%w: %w", ErrNoBootID, errBoom) }, []Warning{BootIDUnreadWarning()}},
	} {
		t.Run(tc.name+" logs a WARN, and every step runs", func(t *testing.T) {
			r := newRecorder()
			tc.stage(r)
			sink := &logtest.Sink{}

			warnings := runPass(t, r, sink)

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
	for _, tc := range []struct {
		name   string
		result restore.Result
		want   []Warning
	}{
		{"an index it can't read, which waits on the user, finishes", restore.Result{IndexErr: errBoom}, []Warning{CorruptSessionsJSONWarning()}},
		{"sessions it couldn't make, kept owed, finish", restore.Result{Restored: 1, Failed: 2}, []Warning{UnrestoredWarning(2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRecorder()
			r.Result = tc.result

			warnings := runPass(t, r, &logtest.Sink{})

			if !equalWarnings(warnings, tc.want) {
				t.Errorf("warnings = %v, want %v", warnings, tc.want)
			}
			if !slices.Equal(r.calls, wantAllCalls) {
				t.Errorf("calls = %v, want every step run, the flag lowered and the latch written", r.calls)
			}
		})
	}
}

func TestOrchestratorRun_Logs(t *testing.T) {
	t.Run("it logs each step entering and complete, in order, then one summary", func(t *testing.T) {
		r := newRecorder()
		r.EnsureErr = errBoom
		sink := &logtest.Sink{}

		runPass(t, r, sink)

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

	t.Run("a pass that leaves the restore unfinished logs each step it skips instead", func(t *testing.T) {
		r := newRecorder()
		r.Answers = []answer{{err: errBoom}}
		sink := &logtest.Sink{}

		runPass(t, r, sink)

		var skipped []string
		for _, rec := range sink.Records().WithMessage("step skipped").AtExactLevel(slog.LevelInfo) {
			skipped = append(skipped, rec.AttrString(t, "step"))
		}
		if want := stepNames[4:]; !slices.Equal(skipped, want) {
			t.Errorf("steps skipped = %v, want %v", skipped, want)
		}
		var completed []string
		for _, rec := range sink.Records().WithMessage("step complete") {
			completed = append(completed, rec.AttrString(t, "step"))
		}
		if want := stepNames[:4]; !slices.Equal(completed, want) {
			t.Errorf("steps complete = %v, want %v", completed, want)
		}
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

	t.Run("it emits each step it skips, so the loading page runs to its end", func(t *testing.T) {
		r := newRecorder()
		r.Answers = []answer{{err: errBoom}}
		var got []int
		ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) { got = append(got, ev.Index) })

		if _, err := newOrchestrator(r, nil).Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if want := []int{1, 2, 3, 4, 5, 6, 7}; !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
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
