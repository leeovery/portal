package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
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

	ZmxErr, ZmxDirErr, RemoveErr, RaiseErr, OrphansErr, FlagsErr, LatchErr error
	// LowerErrs are the lowers' errors in turn, nil past the last.
	LowerErrs []error
	// Raised is the restoring flag, as a pass cut short may leave it.
	Raised bool
	// SaverUp is a saver found present, until End ends it; Recycles has
	// EnsureSaver make a new one in its place, as one that is absent always
	// is.
	SaverUp, Recycles bool
	// CreateErr fails EnsureSaver's create, so it makes no saver; EnsureErr
	// fails it after. EndErr fails End, and EndMadeErr EndMade.
	PresentErr, CreateErr, EnsureErr, MarkErr, EndErr, EndMadeErr error
	// Answers are the protocol's readings in turn, the last repeating.
	Answers []answer
	Result  restore.Result
	// Sessions is how many sessions Restore reports taking up, through the
	// progress SetProgress installed.
	Sessions int
	// DuringRestore runs inside Restore.
	DuringRestore func()

	protocolReads int
	lowers        int
	progress      func(n, m int)
}

func newRecorder() *stepRecorder {
	return &stepRecorder{SaverUp: true, Answers: []answer{{speaks: portalProtocol}}}
}

// portalProtocol is the protocol the tests' Orchestrator speaks.
const portalProtocol = 1

func (r *stepRecorder) record(call string) { r.calls = append(r.calls, call) }

func (r *stepRecorder) called(call string) bool { return slices.Contains(r.calls, call) }

func (r *stepRecorder) count(call string) int {
	n := 0
	for _, c := range r.calls {
		if c == call {
			n++
		}
	}
	return n
}

func (r *stepRecorder) CheckZmx(context.Context) error { r.record("CheckZmx"); return r.ZmxErr }
func (r *stepRecorder) Claim() error                   { r.record("ClaimZmxDir"); return r.ZmxDirErr }
func (r *stepRecorder) RemoveLatch() error             { r.record("RemoveLatch"); return r.RemoveErr }
func (r *stepRecorder) WriteLatch() error              { r.record("WriteLatch"); return r.LatchErr }

func (r *stepRecorder) Raise() error {
	r.record("Raise")
	if r.RaiseErr != nil {
		return r.RaiseErr
	}
	r.Raised = true
	return nil
}

func (r *stepRecorder) Lower() (bool, error) {
	r.record("Lower")
	r.lowers++
	if r.lowers <= len(r.LowerErrs) && r.LowerErrs[r.lowers-1] != nil {
		return false, r.LowerErrs[r.lowers-1]
	}
	raised := r.Raised
	r.Raised = false
	return raised, nil
}

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
	r.SaverUp = true
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

func (r *stepRecorder) End(context.Context) error {
	r.record("End")
	return r.end(r.EndErr)
}

func (r *stepRecorder) EndMade(context.Context) error {
	r.record("EndMade")
	return r.end(r.EndMadeErr)
}

func (r *stepRecorder) end(err error) error {
	if err == nil {
		r.SaverUp = false
	}
	return err
}

func (r *stepRecorder) MarkOwed(context.Context) error { r.record("MarkOwed"); return r.MarkErr }

func (r *stepRecorder) SetProgress(fn func(n, m int)) { r.progress = fn }

func (r *stepRecorder) Restore(context.Context) restore.Result {
	r.record("Restore")
	if !r.Raised {
		r.record("Restore with the restoring flag lowered")
	}
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
		ZmxDir:        r,
		Latch:         r,
		Restoring:     r,
		OrphanSweeper: r,
		FlagSweeper:   r,
		Saver:         r,
		Protocol:      portalProtocol,
		Restore:       r,
		Logger:        logger,
	}
}

// wantAllCalls is every seam call a bootstrap that finds its saver running
// makes, in the order Run makes them.
var wantAllCalls = []string{
	"CheckZmx", "ClaimZmxDir", "RemoveLatch", "Lower", "SweepOrphanDaemons", "SweepStaleFlags",
	"Present", "Protocol", "EnsureSaver", "Raise", "Restore", "Lower", "WriteLatch",
}

// stepNames is the six steps, in order.
var stepNames = []string{"CheckZmx", "RemoveLatch", "Sweep", "EnsureSaver", "Restore", "WriteLatch"}

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

func TestOrchestratorRun_RunsTheSixStepsInOrder(t *testing.T) {
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
			"CheckZmx", "ClaimZmxDir", "RemoveLatch", "Lower", "SweepOrphanDaemons", "SweepStaleFlags",
			"Present", "MarkOwed", "EnsureSaver", "Create", "Protocol", "Raise", "Restore", "Lower", "WriteLatch",
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

// Decision 67: the latch is the record of a finished bootstrap, and the
// restoring flag covers restore's creates alone.
func TestOrchestratorRun_LatchAndRestoringFlag(t *testing.T) {
	t.Run("it removes the latch before its first change", func(t *testing.T) {
		r := newRecorder()

		runPass(t, r, &logtest.Sink{})

		if got := r.calls[:3]; !slices.Equal(got, []string{"CheckZmx", "ClaimZmxDir", "RemoveLatch"}) {
			t.Errorf("calls begin %v, want the latch removed straight after zmx is checked", got)
		}
	})

	t.Run("it raises the restoring flag only for restore, and lowers it once restore returns", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false

		runPass(t, r, &logtest.Sink{})

		raise := slices.Index(r.calls, "Raise")
		if raise < 0 || r.calls[raise+1] != "Restore" || r.calls[raise+2] != "Lower" {
			t.Errorf("calls = %v, want Raise, Restore, Lower together", r.calls)
		}
		if raise < slices.Index(r.calls, "MarkOwed") || raise < slices.Index(r.calls, "Protocol") {
			t.Errorf("calls = %v, want the flag raised only after the owed marks and the judgement", r.calls)
		}
		if r.called("Restore with the restoring flag lowered") || r.Raised {
			t.Errorf("calls = %v, flag raised %v; want restore run under the flag, and the flag lowered after", r.calls, r.Raised)
		}
	})

	t.Run("a restoring flag left raised by a pass cut short is lowered before anything else, and logged", func(t *testing.T) {
		r := newRecorder()
		r.Raised = true
		sink := &logtest.Sink{}

		runPass(t, r, sink)

		if got := r.calls[:4]; !slices.Equal(got, []string{"CheckZmx", "ClaimZmxDir", "RemoveLatch", "Lower"}) {
			t.Errorf("calls begin %v, want the flag lowered with the latch removed", got)
		}
		rec := sink.Records().WithMessage("restoring flag lowered").AtExactLevel(slog.LevelInfo).Only(t, "the flag left raised")
		if got := rec.AttrString(t, "detail"); got != "a restore cut short left it raised" {
			t.Errorf("detail = %q", got)
		}
	})

	t.Run("a pass that left no flag raised logs no lowering", func(t *testing.T) {
		sink := &logtest.Sink{}

		runPass(t, newRecorder(), sink)

		if got := sink.Records().WithMessage("restoring flag lowered"); len(got) != 0 {
			t.Errorf("records = %+v, want none: restore's own lowering is no flag left raised", got)
		}
	})
}

// decision 62: only a saver's absence proves none saw the sessions end, and
// decision 68: a saver Portal can't use, ended, is as none.
func TestOrchestratorRun_MarksOwedOnlyWhenNoSaverRuns(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage func(*stepRecorder)
		want  bool
	}{
		{"a saver found running", func(*stepRecorder) {}, false},
		{"no saver found", func(r *stepRecorder) { r.SaverUp = false }, true},
		{"a saver that couldn't be told, which may have watched sessions end", func(r *stepRecorder) { r.PresentErr = errBoom }, false},
		{"a saver found that Portal can't use, ended", func(r *stepRecorder) { r.Answers = []answer{{speaks: 2}, {speaks: portalProtocol}} }, true},
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
			name: "a state directory of another zmx directory",
			stage: func(r *stepRecorder) {
				r.ZmxDirErr = &OtherZmxDirError{StateDir: "/srv/state", Recorded: "/srv/zmx", Using: "/tmp/scratch"}
			},
			step:    indexCheckZmx,
			message: "Portal's state in /srv/state belongs to the zmx sessions in /srv/zmx, but this Portal is using /tmp/scratch. Run it with ZMX_DIR=/srv/zmx, or give it a state of its own with PORTAL_STATE_DIR.",
			calls:   wantAllCalls[:2],
		},
		{
			name:    "a zmx directory record it can't read",
			stage:   func(r *stepRecorder) { r.ZmxDirErr = &ZmxDirRecordError{Path: "/srv/state/zmx-dir", Err: errBoom} },
			step:    indexCheckZmx,
			message: "Portal can't read /srv/state/zmx-dir, which records the zmx directory its state belongs to: boom. Delete it, and Portal records the directory in use at its next start.",
			calls:   wantAllCalls[:2],
		},
		{
			name: "a zmx directory record it can't write",
			stage: func(r *stepRecorder) {
				r.ZmxDirErr = &ZmxDirRecordError{Path: "/srv/state/zmx-dir", Write: true, Err: errBoom}
			},
			step:    indexCheckZmx,
			message: "Portal failed to record the zmx directory its state belongs to in /srv/state/zmx-dir: boom",
			calls:   wantAllCalls[:2],
		},
		{
			name:    "removing the latch",
			stage:   func(r *stepRecorder) { r.RemoveErr = errBoom },
			step:    indexRemoveLatch,
			message: "Portal failed to remove the bootstrapped latch: boom",
			calls:   wantAllCalls[:3],
		},
		{
			name:    "lowering a restoring flag left raised",
			stage:   func(r *stepRecorder) { r.LowerErrs = []error{errBoom} },
			step:    indexRemoveLatch,
			message: "Portal failed to lower the restoring flag: boom",
			calls:   wantAllCalls[:4],
		},
		{
			name:    "raising the restoring flag",
			stage:   func(r *stepRecorder) { r.RaiseErr = errBoom },
			step:    indexRestore,
			message: "Portal failed to raise the restoring flag: boom",
			calls:   wantAllCalls[:10],
		},
		{
			name:    "lowering the restoring flag after restore",
			stage:   func(r *stepRecorder) { r.LowerErrs = []error{nil, errBoom} },
			step:    indexRestore,
			message: "Portal failed to lower the restoring flag: boom",
			calls:   wantAllCalls[:12],
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

// anotherProtocol is the fatal of a saver speaking protocol 2.
const anotherProtocol = "Portal can't use this zmx: its sessions speak protocol 2, and Portal speaks 1. Upgrade Portal, or install the zmx release it supports."

var (
	older   = answer{err: fmt.Errorf("%w: no answer in 1s", ErrOlderZmx)}
	another = answer{speaks: 2}
	neither = answer{err: errBoom}
	portals = answer{speaks: portalProtocol}
)

// decision 68: a saver found that Portal can't use is ended, and the pass
// goes on as if no saver ran.
func TestOrchestratorRun_EndsASaverFoundThatPortalCantUse(t *testing.T) {
	for name, verdict := range map[string]answer{"answers zmx 0.8.1's framing": older, "speaks another protocol": another} {
		t.Run("one that "+name+" is ended, then sessions are marked owed, a saver made and judged, and restored", func(t *testing.T) {
			r := newRecorder()
			r.Answers = []answer{verdict, portals}
			sink := &logtest.Sink{}

			if warnings := runPass(t, r, sink); len(warnings) != 0 {
				t.Errorf("warnings = %v, want none", warnings)
			}

			want := []string{
				"CheckZmx", "ClaimZmxDir", "RemoveLatch", "Lower", "SweepOrphanDaemons", "SweepStaleFlags",
				"Present", "Protocol", "End", "MarkOwed", "EnsureSaver", "Create", "Protocol", "Raise", "Restore", "Lower", "WriteLatch",
			}
			if !slices.Equal(r.calls, want) {
				t.Errorf("calls = %v, want %v", r.calls, want)
			}
			rec := sink.Records().WithMessage("saver ended").AtExactLevel(slog.LevelInfo).Only(t, "the saver ended")
			if got := rec.AttrString(t, "detail"); !strings.Contains(got, "Portal ended the saver running on it") {
				t.Errorf("detail = %q, want the saver's verdict and its end", got)
			}
		})
	}

	t.Run("one ended, with the zmx on PATH still one Portal can't use, ends the one it makes too, and is fatal", func(t *testing.T) {
		r := newRecorder()
		r.Answers = []answer{older}

		fatal := fatalOf(t, r)

		if fatal.UserMessage != olderZmx || fatal.Step != indexEnsureSaver {
			t.Errorf("fatal = %q at step %d, want %q at step %d", fatal.UserMessage, fatal.Step, olderZmx, indexEnsureSaver)
		}
		if r.count("End") != 1 || r.count("EndMade") != 1 || !r.called("MarkOwed") || r.called("Raise") || r.called("Restore") || r.called("WriteLatch") {
			t.Errorf("calls = %v, want the saver found ended, then the one made, the owed marks written, and nothing restored", r.calls)
		}
		if !strings.Contains(fatal.Cause.Error(), "Portal ended the saver it made on it") {
			t.Errorf("cause = %v, want it to say the saver made was ended", fatal.Cause)
		}
	})

	// The judgement's own fatal names a zmx to install, which leaves this
	// saver running: every command after would find it, still unended.
	for name, verdict := range map[string]answer{"answers zmx 0.8.1's framing": older, "speaks another protocol": another} {
		t.Run("one that "+name+" it can't end is the fatal naming the way out, nothing made, and the cause names the saver left", func(t *testing.T) {
			r := newRecorder()
			r.Answers = []answer{verdict}
			r.EndErr = errors.New("no one process is proved the session's zmx daemon")

			fatal := fatalOf(t, r)

			if fatal.UserMessage != SaverLeftMessage || fatal.Step != indexEnsureSaver {
				t.Errorf("fatal = %q at step %d, want %q at step %d", fatal.UserMessage, fatal.Step, SaverLeftMessage, indexEnsureSaver)
			}
			if !strings.Contains(fatal.Cause.Error(), "Portal couldn't end the saver running on it, which is left: no one process") {
				t.Errorf("cause = %v, want it to name the saver left", fatal.Cause)
			}
			if verdict == older && !errors.Is(fatal, ErrOlderZmx) {
				t.Errorf("cause = %v, want it to carry the verdict", fatal.Cause)
			}
			for _, call := range []string{"MarkOwed", "EnsureSaver", "Create", "Raise", "Restore", "WriteLatch"} {
				if r.called(call) {
					t.Errorf("calls = %v, want no %s past a saver it couldn't end", r.calls, call)
				}
			}
		})
	}
}

// decision 63: the saver made is judged before restore creates anything.
func TestOrchestratorRun_JudgesTheSaverItMakes(t *testing.T) {
	for name, verdict := range map[string]answer{"answers zmx 0.8.1's framing": older, "speaks another protocol": another} {
		t.Run("one that "+name+" is ended, and fatal before restore creates anything", func(t *testing.T) {
			r := newRecorder()
			r.SaverUp = false
			r.Answers = []answer{verdict}

			fatal := fatalOf(t, r)

			if want := map[bool]string{true: olderZmx, false: anotherProtocol}[verdict == older]; fatal.UserMessage != want || fatal.Step != indexEnsureSaver {
				t.Errorf("fatal = %q at step %d, want %q at step %d", fatal.UserMessage, fatal.Step, want, indexEnsureSaver)
			}
			if !r.called("EndMade") || r.called("End") || r.called("Raise") || r.called("Restore") || r.called("WriteLatch") {
				t.Errorf("calls = %v, want the saver made ended as made, and no restore and no latch", r.calls)
			}
		})
	}

	t.Run("one it can't end is left, and the fatal's cause says so", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false
		r.Answers = []answer{older}
		r.EndMadeErr = errors.New("its socket outlived SIGTERM")

		fatal := fatalOf(t, r)

		if !strings.Contains(fatal.Cause.Error(), "Portal couldn't end the saver it made on it, which is left: its socket outlived SIGTERM") || fatal.UserMessage != olderZmx {
			t.Errorf("fatal = %q, cause %v; want the older-zmx fatal, its cause naming the saver left", fatal.UserMessage, fatal.Cause)
		}
	})

	// decision 75: the backend's create judges what it made, ending a session
	// of zmx 0.8.1's with that zmx's kill, or its daemon's proved signal,
	// before it refuses it, so the pass ends nothing again.
	t.Run("one the create refuses as zmx 0.8.1's is fatal, nothing ended again, and nothing restored", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false
		r.CreateErr = fmt.Errorf("create the saver session: %w; zmx kill ended the session", backend.ErrUnsupportedProtocol)

		fatal := fatalOf(t, r)

		if fatal.UserMessage != olderZmx || fatal.Step != indexEnsureSaver || !errors.Is(fatal, backend.ErrUnsupportedProtocol) {
			t.Errorf("fatal = %q at step %d (%v), want %q at step %d carrying the refusal", fatal.UserMessage, fatal.Step, fatal.Cause, olderZmx, indexEnsureSaver)
		}
		if r.called("End") || r.called("EndMade") || r.called("Raise") || r.called("Restore") || r.called("WriteLatch") {
			t.Errorf("calls = %v, want nothing ended after the create's own end, and no restore and no latch", r.calls)
		}
	})

	t.Run("one the create refuses and couldn't end is fatal, its cause the create's saying so", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false
		r.CreateErr = fmt.Errorf("create the saver session: %w; the session is left running: zmx kill failed; then: no one process is proved the session's zmx daemon", backend.ErrUnsupportedProtocol)

		fatal := fatalOf(t, r)

		if fatal.UserMessage != olderZmx || !errors.Is(fatal.Cause, r.CreateErr) {
			t.Errorf("fatal = %q, cause %v; want the older-zmx fatal carrying the create's refusal", fatal.UserMessage, fatal.Cause)
		}
	})

	t.Run("one it recycles that answers zmx 0.8.1's framing is fatal, and ended", func(t *testing.T) {
		r := newRecorder()
		r.Recycles = true
		r.Answers = []answer{portals, older}

		if fatal := fatalOf(t, r); fatal.UserMessage != olderZmx || !r.called("EndMade") || r.called("Restore") {
			t.Errorf("fatal = %q, calls %v; want the older-zmx fatal, the made saver ended, no restore", fatal.UserMessage, r.calls)
		}
	})

	t.Run("one gone at once is a saver down, and leaves the restore unfinished", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false
		r.Answers = []answer{{err: fmt.Errorf("%w: no such session", ErrSaverGone)}}
		sink := &logtest.Sink{}

		warnings := runPass(t, r, sink)

		if !equalWarnings(warnings, []Warning{SaverDownWarning(), UnjudgedWarning()}) {
			t.Errorf("warnings = %v, want the saver's and the can't-judge warning", warnings)
		}
		assertUnfinished(t, r, sink)
	})

	t.Run("none made, its create failing, leaves the restore unfinished", func(t *testing.T) {
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

	t.Run("one judged Portal's whose daemon then fails to come up still restores", func(t *testing.T) {
		r := newRecorder()
		r.SaverUp = false
		r.EnsureErr = fmt.Errorf("%w within 10s", saverhost.ErrNotReady)

		warnings := runPass(t, r, &logtest.Sink{})

		if !equalWarnings(warnings, []Warning{SaverDownWarning()}) || !r.called("Restore") || !r.called("WriteLatch") {
			t.Errorf("warnings = %v, calls %v; want the saver's warning, and the restore finished", warnings, r.calls)
		}
	})
}

func TestOrchestratorRun_LeavesASaverItCantJudgeBe(t *testing.T) {
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
			if r.called("End") || r.called("EndMade") {
				t.Errorf("calls = %v, want a saver Portal can't judge left be", r.calls)
			}
			if found && r.called("EnsureSaver") {
				t.Errorf("calls = %v, want nothing ensured over a saver it can't judge", r.calls)
			}
		})
	}

	t.Run("a saver found gone by its judgement is kept as one that may have watched sessions end", func(t *testing.T) {
		r := newRecorder()
		r.Answers = []answer{{err: fmt.Errorf("%w: no such session", ErrSaverGone)}}
		sink := &logtest.Sink{}

		warnings := runPass(t, r, sink)

		if !equalWarnings(warnings, []Warning{UnjudgedWarning()}) || r.called("MarkOwed") {
			t.Errorf("warnings = %v, calls %v; want the can't-judge warning alone, and no session marked owed", warnings, r.calls)
		}
		assertUnfinished(t, r, sink)
	})
}

// assertUnfinished checks a run left the restore unfinished: no latch
// written, the restoring flag lowered, and each step skipped logged.
func assertUnfinished(t *testing.T, r *stepRecorder, sink *logtest.Sink) {
	t.Helper()
	if r.called("WriteLatch") || r.Raised {
		t.Errorf("calls = %v, flag raised %v; want no latch written and the flag lowered", r.calls, r.Raised)
	}
	var skipped []string
	for _, rec := range sink.Records().WithMessage("step skipped").AtExactLevel(slog.LevelInfo) {
		skipped = append(skipped, rec.AttrString(t, "step"))
	}
	if want := []string{"WriteLatch"}; !slices.Equal(skipped[max(len(skipped)-1, 0):], want) {
		t.Errorf("steps skipped = %v, want them to end %v", skipped, want)
	}
}

// decision 67: only a bootstrap that finished its restore writes the latch,
// so the next command finishes one cut short.
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
			if r.called("Restore") != tc.restored || r.called("Raise") != tc.restored {
				t.Errorf("calls = %v, want Restore, and the flag raised for it, %v", r.calls, tc.restored)
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

// decision 75: a zmx 0.8.1 first on PATH makes every create fail alike, so a
// restore the backend refused a session to stops there, with the fatal.
func TestOrchestratorRun_RestoreStoppedByAnUnsupportedCreateIsFatal(t *testing.T) {
	r := newRecorder()
	r.Result = restore.Result{Failed: 1, Unsupported: fmt.Errorf("create session %q: %w", "work", backend.ErrUnsupportedProtocol)}

	fatal := fatalOf(t, r)

	if fatal.UserMessage != olderZmx || fatal.Step != indexRestore || !errors.Is(fatal, backend.ErrUnsupportedProtocol) {
		t.Errorf("fatal = %q at step %d (%v), want %q at step %d carrying the refusal", fatal.UserMessage, fatal.Step, fatal.Cause, olderZmx, indexRestore)
	}
	if want := wantAllCalls[:len(wantAllCalls)-1]; !slices.Equal(r.calls, want) || r.Raised {
		t.Errorf("calls = %v, want %v: the flag lowered, no latch", r.calls, want)
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
		r.Answers = []answer{neither}
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
			{Index: 2, Name: "RemoveLatch"},
			{Index: 3, Name: "Sweep"},
			{Index: 4, Name: "EnsureSaver"},
			{Index: 5, Name: "Restore", RestoreN: 1, RestoreM: 2},
			{Index: 5, Name: "Restore", RestoreN: 2, RestoreM: 2},
			{Index: 5, Name: "Restore"},
			{Index: 6, Name: "WriteLatch"},
		}
		if !slices.Equal(got, want) {
			t.Errorf("events = %+v\nwant %+v", got, want)
		}
	})

	t.Run("it emits each step it skips, so the loading page runs to its end", func(t *testing.T) {
		r := newRecorder()
		r.Answers = []answer{neither}
		var got []int
		ctx := WithProgressEmitter(context.Background(), func(ev StepEvent) { got = append(got, ev.Index) })

		if _, err := newOrchestrator(r, nil).Run(ctx); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if want := []int{1, 2, 3, 4, 5, 6}; !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}
	})

	t.Run("a fatal step emits nothing for itself or after it", func(t *testing.T) {
		r := newRecorder()
		r.RemoveErr = errBoom
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
