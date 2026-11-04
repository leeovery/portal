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
	"github.com/leeovery/portal/internal/tmux"
)

type stepRecorder struct {
	calls []string

	SetErr                error
	SweepOrphanDaemonsErr error
	EnsureSaverErr        error
	RestoreCorrupt        bool
	RestoreErr            error
	ClearErr              error
}

func (r *stepRecorder) Set() error {
	r.calls = append(r.calls, "Set")
	return r.SetErr
}

func (r *stepRecorder) Clear() error {
	r.calls = append(r.calls, "Clear")
	return r.ClearErr
}

func (r *stepRecorder) SweepOrphanDaemons() error {
	r.calls = append(r.calls, "SweepOrphanDaemons")
	return r.SweepOrphanDaemonsErr
}

func (r *stepRecorder) EnsureSaver() error {
	r.calls = append(r.calls, "EnsureSaver")
	return r.EnsureSaverErr
}

func (r *stepRecorder) Restore() (bool, error) {
	r.calls = append(r.calls, "Restore")
	return r.RestoreCorrupt, r.RestoreErr
}

func newOrchestrator(r *stepRecorder, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		Restoring:     r,
		OrphanSweeper: r,
		Saver:         r,
		Restore:       r,
		Logger:        logger,
	}
}

// wantAllCalls is every step's recorder call, in the order Run makes them.
var wantAllCalls = []string{"Set", "SweepOrphanDaemons", "EnsureSaver", "Restore", "Clear"}

// errCorruptIndex stands in for a Restorer's corrupt-index cause.
var errCorruptIndex = errors.New("sessions.json corrupt")

func equalCalls(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestOrchestratorRun_executesStepsInSpecOrder(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	want := wantAllCalls
	if !equalCalls(r.calls, want) {
		t.Errorf("call order = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_propagatesSetRestoringErrorAndSkipsLaterSteps(t *testing.T) {
	sentinel := errors.New("set marker boom")
	r := &stepRecorder{SetErr: sentinel}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, _, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
	}
	var fatal *FatalError
	if !errors.As(err, &fatal) {
		t.Fatalf("expected *FatalError, got %T (%v)", err, err)
	}
	wantPrefix := "Portal failed to set @portal-restoring marker: "
	if !strings.HasPrefix(fatal.UserMessage, wantPrefix) {
		t.Errorf("UserMessage = %q, want prefix %q", fatal.UserMessage, wantPrefix)
	}
	if !strings.Contains(fatal.UserMessage, "set marker boom") {
		t.Errorf("UserMessage = %q, want to contain underlying %q", fatal.UserMessage, "set marker boom")
	}
	if sink.Records().AtExactLevel(slog.LevelError) == nil {
		t.Error("expected an ERROR record before the fatal return")
	}
	for _, c := range r.calls {
		if c == "EnsureSaver" {
			t.Errorf("EnsureSaver must not run when Set fails; calls = %v", r.calls)
		}
		if c == "Restore" {
			t.Errorf("Restore must not run when Set fails; calls = %v", r.calls)
		}
	}
	found := slices.Contains(r.calls, "Set")
	if !found {
		t.Errorf("expected Set in calls, got %v", r.calls)
	}
}

func TestOrchestratorRun_continuesPastEnsureSaverFailureAndAppendsWarning(t *testing.T) {
	sentinel := errors.New("saver boom")
	r := &stepRecorder{EnsureSaverErr: sentinel}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not return saver failures; got %v", err)
	}
	wantWarning := SaverDownWarning()
	if len(warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1; got %#v", len(warnings), warnings)
	}
	if len(warnings[0].Lines) != len(wantWarning.Lines) {
		t.Fatalf("warning Lines len = %d, want %d", len(warnings[0].Lines), len(wantWarning.Lines))
	}
	for i := range wantWarning.Lines {
		if warnings[0].Lines[i] != wantWarning.Lines[i] {
			t.Errorf("warning Lines[%d] = %q, want %q", i, warnings[0].Lines[i], wantWarning.Lines[i])
		}
	}
	want := wantAllCalls
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
	if sink.Records().AtExactLevel(slog.LevelWarn) == nil {
		t.Error("expected at least one WARN record")
	}
}

func TestOrchestratorRun_clearsRestoringEvenWhenRestoreFails(t *testing.T) {
	corruptErr := fmt.Errorf("restore: %w", errCorruptIndex)
	r := &stepRecorder{RestoreCorrupt: true, RestoreErr: corruptErr}
	o := newOrchestrator(r, nil)

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must treat corrupt-index restore as soft; got %v", err)
	}

	restoreIdx, clearIdx := -1, -1
	for i, c := range r.calls {
		if c == "Restore" {
			restoreIdx = i
		}
		if c == "Clear" {
			clearIdx = i
		}
	}
	if restoreIdx == -1 {
		t.Fatalf("Restore not called; calls = %v", r.calls)
	}
	if clearIdx == -1 {
		t.Fatalf("Clear not called; calls = %v", r.calls)
	}
	if clearIdx < restoreIdx {
		t.Errorf("Clear must run after Restore; calls = %v", r.calls)
	}
}

func TestOrchestratorRun_reportsClearRestoringFailureAsFatal(t *testing.T) {
	sentinel := errors.New("clear boom")
	r := &stepRecorder{ClearErr: sentinel}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, _, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
	}
	var fatal *FatalError
	if !errors.As(err, &fatal) {
		t.Fatalf("expected *FatalError, got %T (%v)", err, err)
	}
	wantPrefix := "Portal failed to clear @portal-restoring marker: "
	if !strings.HasPrefix(fatal.UserMessage, wantPrefix) {
		t.Errorf("UserMessage = %q, want prefix %q", fatal.UserMessage, wantPrefix)
	}
	if !strings.Contains(fatal.UserMessage, "clear boom") {
		t.Errorf("UserMessage = %q, want to contain underlying %q", fatal.UserMessage, "clear boom")
	}
	if sink.Records().AtExactLevel(slog.LevelError) == nil {
		t.Error("expected an ERROR record before the fatal return")
	}
}

func TestOrchestratorRun_isIdempotentAcrossInvocations(t *testing.T) {
	r1 := &stepRecorder{}
	o := newOrchestrator(r1, nil)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("first Run errored: %v", err)
	}
	want := wantAllCalls
	if !equalCalls(r1.calls, want) {
		t.Errorf("first calls = %v, want %v", r1.calls, want)
	}

	r2 := &stepRecorder{}
	o.Restoring = r2
	o.OrphanSweeper = r2
	o.Saver = r2
	o.Restore = r2

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("second Run errored: %v", err)
	}
	if !equalCalls(r2.calls, want) {
		t.Errorf("second calls = %v, want %v", r2.calls, want)
	}
}

func TestOrchestratorRun_reportsNoServerStarted(t *testing.T) {
	o := newOrchestrator(&stepRecorder{}, nil)

	started, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run errored: %v", err)
	}
	if started {
		t.Error("serverStarted = true; Run starts no server")
	}
}

func TestOrchestratorRun_doesNotCallEnsureSaverWhenSetFails(t *testing.T) {
	r := &stepRecorder{SetErr: errors.New("set boom")}
	o := newOrchestrator(r, nil)

	_, _, _ = o.Run(context.Background())

	for _, c := range r.calls {
		if c == "EnsureSaver" {
			t.Fatalf("EnsureSaver must not run when Set fails; calls = %v", r.calls)
		}
	}
}

func TestOrchestratorRun_ensureSaverFailureDoesNotProduceFatalError(t *testing.T) {
	sentinel := errors.New("saver boom")
	r := &stepRecorder{EnsureSaverErr: sentinel}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not surface saver failures; got %v", err)
	}

	if _, ok := errors.AsType[*FatalError](err); ok {
		t.Errorf("Run unexpectedly returned *FatalError on soft saver failure: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1; got %#v", len(warnings), warnings)
	}
	if warnings[0].Lines[0] != SaverDownWarning().Lines[0] {
		t.Errorf("warnings[0] = %q, want SaverDownWarning", warnings[0].Lines[0])
	}
	if errs := sink.Records().AtExactLevel(slog.LevelError); errs != nil {
		t.Errorf("no ERROR record may be emitted for a soft saver failure; got %+v", errs)
	}
	if sink.Records().AtExactLevel(slog.LevelWarn) == nil {
		t.Error("expected a WARN record for the soft saver failure")
	}
}

func TestOrchestratorRun_appendsSaverDownWarningOnEnsureSaverFailure(t *testing.T) {
	r := &stepRecorder{EnsureSaverErr: errors.New("saver boom")}
	o := newOrchestrator(r, nil)

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not return saver failures; got %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1; got %#v", len(warnings), warnings)
	}
	want := SaverDownWarning()
	if len(warnings[0].Lines) != len(want.Lines) {
		t.Fatalf("warning Lines len = %d, want %d", len(warnings[0].Lines), len(want.Lines))
	}
	for i := range want.Lines {
		if warnings[0].Lines[i] != want.Lines[i] {
			t.Errorf("warning Lines[%d] = %q, want %q", i, warnings[0].Lines[i], want.Lines[i])
		}
	}
}

func TestOrchestratorRun_appendsCorruptSessionsJSONWarningOnRestoreErrCorruptIndex(t *testing.T) {
	corruptErr := fmt.Errorf("restore: %w", errCorruptIndex)
	r := &stepRecorder{RestoreCorrupt: true, RestoreErr: corruptErr}
	o := newOrchestrator(r, nil)

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must treat ErrCorruptIndex as soft; got err=%v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1; got %#v", len(warnings), warnings)
	}
	want := CorruptSessionsJSONWarning()
	for i := range want.Lines {
		if warnings[0].Lines[i] != want.Lines[i] {
			t.Errorf("warning Lines[%d] = %q, want %q", i, warnings[0].Lines[i], want.Lines[i])
		}
	}
}

func TestOrchestratorRun_accumulatesMultipleSoftWarnings(t *testing.T) {
	r := &stepRecorder{
		EnsureSaverErr: errors.New("saver boom"),
		RestoreCorrupt: true,
		RestoreErr:     fmt.Errorf("restore: %w", errCorruptIndex),
	}
	o := newOrchestrator(r, nil)

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must treat both as soft; got err=%v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings len = %d, want 2; got %#v", len(warnings), warnings)
	}
	if warnings[0].Lines[0] != SaverDownWarning().Lines[0] {
		t.Errorf("warnings[0] = %q, want SaverDownWarning first", warnings[0].Lines[0])
	}
	if warnings[1].Lines[0] != CorruptSessionsJSONWarning().Lines[0] {
		t.Errorf("warnings[1] = %q, want CorruptSessionsJSONWarning second", warnings[1].Lines[0])
	}
}

func TestOrchestratorRun_doesNotReturnFatalErrorForCorruptIndex(t *testing.T) {
	r := &stepRecorder{
		RestoreCorrupt: true,
		RestoreErr:     fmt.Errorf("restore: %w", errCorruptIndex),
	}
	o := newOrchestrator(r, nil)

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("expected nil err for soft corrupt-index path; got %v", err)
	}
}

func TestOrchestratorRun_doesNotEscalateNonCorruptRestoreError(t *testing.T) {
	sentinel := errors.New("restore boom")
	r := &stepRecorder{RestoreCorrupt: false, RestoreErr: sentinel}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must NOT escalate a non-corrupt soft restore error; got %v", err)
	}
	if sink.Records().AtExactLevel(slog.LevelWarn) == nil {
		t.Error("expected a WARN record for the contract-violating soft failure")
	}
	if errs := sink.Records().AtExactLevel(slog.LevelError); errs != nil {
		t.Errorf("no ERROR record may be emitted for a soft restore failure; got %+v", errs)
	}
}

func TestOrchestratorRun_doesNotEmitCorruptWarningWhenCorruptFalse(t *testing.T) {
	r := &stepRecorder{RestoreCorrupt: false, RestoreErr: errors.New("soft boom")}
	o := newOrchestrator(r, nil)

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run errored: %v", err)
	}
	for _, w := range warnings {
		if len(w.Lines) > 0 && w.Lines[0] == CorruptSessionsJSONWarning().Lines[0] {
			t.Errorf("must not emit CorruptSessionsJSONWarning when corrupt=false; got %#v", warnings)
		}
	}
}

func TestOrchestratorRun_emptyWarningsOnHappyPath(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run errored: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected zero warnings on happy path; got %#v", warnings)
	}
}

var _ Runner = (*Orchestrator)(nil)

func TestOrchestratorRun_emitsDebugLinePerExecutedStep(t *testing.T) {
	r := &stepRecorder{}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	steps := wantAllCalls
	debugs := sink.Records().AtExactLevel(slog.LevelDebug)
	for _, step := range steps {
		matches := 0
		for _, rec := range debugs {
			if strings.Contains(rec.AttrOrEmpty("step"), step) {
				matches++
			}
		}
		if matches < 1 {
			t.Errorf("step %q: expected ≥1 DEBUG record referencing it; got debugs=%+v", step, debugs)
		}
	}
}

func TestOrchestratorRun_stepEntryLinesEmitUnderBootstrapComponent(t *testing.T) {
	r := &stepRecorder{}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink).With("component", "bootstrap"))

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	entries := sink.Records().WithMessage("step entering").AtExactLevel(slog.LevelDebug)
	for i, rec := range entries {
		if comp := rec.AttrString(t, "component"); comp != "bootstrap" {
			t.Errorf("step-entry DEBUG[%d] component = %q, want %q (record=%+v)", i, comp, "bootstrap", rec)
		}
	}
	if len(entries) == 0 {
		t.Errorf("expected ≥1 'step entering' DEBUG record; got debugs=%+v", sink.Records().AtExactLevel(slog.LevelDebug))
	}
}

var closedStepNames = []string{
	"SetRestoring",
	"SweepOrphanDaemons",
	"EnsureSaver",
	"Restore",
	"ClearRestoring",
}

// stepCompleteNames returns the step named by each "step complete" INFO, in
// emission order.
func stepCompleteNames(t *testing.T, sink *logtest.Sink) []string {
	t.Helper()
	var out []string
	for _, rec := range stepCompleteRecords(sink) {
		out = append(out, rec.AttrString(t, "step"))
	}
	return out
}

func stepCompleteRecords(sink *logtest.Sink) logtest.Records {
	return sink.Records().WithMessage("step complete").AtExactLevel(slog.LevelInfo)
}

func orchestrationCompleteRecords(sink *logtest.Sink) logtest.Records {
	return sink.Records().WithMessage("orchestration complete").AtExactLevel(slog.LevelInfo)
}

func TestOrchestratorRun_emitsStepCompletePerStepInOrder(t *testing.T) {
	r := &stepRecorder{}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	got := stepCompleteNames(t, sink)
	if !equalCalls(got, closedStepNames) {
		t.Errorf("step complete names = %v, want %v", got, closedStepNames)
	}
}

func TestOrchestratorRun_emitsStepCompleteUnderBootstrapComponent(t *testing.T) {
	r := &stepRecorder{}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink).With("component", "bootstrap"))

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	completes := stepCompleteRecords(sink)
	for i, rec := range completes {
		if comp := rec.AttrString(t, "component"); comp != "bootstrap" {
			t.Errorf("step-complete INFO[%d] component = %q, want %q (record=%+v)", i, comp, "bootstrap", rec)
		}
	}
	if len(completes) != len(closedStepNames) {
		t.Errorf("step complete INFO count = %d, want %d; got %+v", len(completes), len(closedStepNames), completes)
	}
}

func TestOrchestratorRun_emitsOrchestrationCompleteOnCleanBootstrap(t *testing.T) {
	r := &stepRecorder{}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	rec := orchestrationCompleteRecords(sink).Only(t, "orchestration complete INFO")
	if got := rec.IntAttr(t, "steps"); got != int64(len(closedStepNames)) {
		t.Errorf("orchestration complete steps = %d, want %d", got, len(closedStepNames))
	}
	if got := rec.IntAttr(t, "warnings"); got != 0 {
		t.Errorf("orchestration complete warnings = %d, want 0", got)
	}
	rec.RequireDuration(t, "took")
}

func TestOrchestratorRun_orchestrationCompleteReportsAccumulatedWarnings(t *testing.T) {
	r := &stepRecorder{
		EnsureSaverErr: errors.New("saver boom"),
		RestoreCorrupt: true,
		RestoreErr:     fmt.Errorf("restore: %w", errCorruptIndex),
	}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must treat both as soft; got %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings len = %d, want 2; got %#v", len(warnings), warnings)
	}

	rec := orchestrationCompleteRecords(sink).Only(t, "orchestration complete INFO")
	if got := rec.IntAttr(t, "warnings"); got != 2 {
		t.Errorf("orchestration complete warnings = %d, want 2", got)
	}
	if got := rec.IntAttr(t, "steps"); got != int64(len(closedStepNames)) {
		t.Errorf("orchestration complete steps = %d, want %d", got, len(closedStepNames))
	}
}

func TestOrchestratorRun_fatalFirstStepShortCircuitsBeforeSummaries(t *testing.T) {
	r := &stepRecorder{SetErr: errors.New("set boom")}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, _, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected fatal error, got nil")
	}

	if completes := stepCompleteRecords(sink); completes != nil {
		t.Errorf("no step complete must fire on a fatal first-step abort; got %+v", completes)
	}
	if summaries := orchestrationCompleteRecords(sink); summaries != nil {
		t.Errorf("orchestration summary must not fire on a fatal first-step abort; got %+v", summaries)
	}
	if sink.Records().AtExactLevel(slog.LevelError) == nil {
		t.Error("expected the fatal ERROR line as the terminal record")
	}
}

func TestOrchestratorRun_fatalClearRestoringShortCircuitsBeforeSummary(t *testing.T) {
	r := &stepRecorder{ClearErr: errors.New("clear boom")}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, _, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected fatal error, got nil")
	}

	if summaries := orchestrationCompleteRecords(sink); summaries != nil {
		t.Errorf("orchestration summary must not fire on a fatal ClearRestoring abort; got %+v", summaries)
	}

	got := stepCompleteNames(t, sink)
	for _, name := range got {
		if name == "ClearRestoring" {
			t.Errorf("no step complete must fire for the aborting ClearRestoring step; got %v", got)
		}
	}
	wantBefore := closedStepNames[:len(closedStepNames)-1]
	if !equalCalls(got, wantBefore) {
		t.Errorf("step complete names before the ClearRestoring abort = %v, want %v", got, wantBefore)
	}
	if sink.Records().AtExactLevel(slog.LevelError) == nil {
		t.Error("expected the fatal ERROR line as the terminal record")
	}
}

func TestOrchestratorRun_retainsEnteringDebugWithNormalizedNames(t *testing.T) {
	r := &stepRecorder{}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	var enteringNames []string
	for _, rec := range sink.Records().WithMessage("step entering").AtExactLevel(slog.LevelDebug) {
		enteringNames = append(enteringNames, rec.AttrString(t, "step"))
	}
	if !equalCalls(enteringNames, closedStepNames) {
		t.Errorf("entering DEBUG step names = %v, want %v", enteringNames, closedStepNames)
	}
	for _, name := range enteringNames {
		if strings.Contains(name, "@portal-restoring") {
			t.Errorf("entering DEBUG must use normalized names, not legacy %q", name)
		}
	}
}

func countCalls(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

func indexOf(calls []string, name string) int {
	for i, c := range calls {
		if c == name {
			return i
		}
	}
	return -1
}

func TestOrchestratorRun_invokesSweepOrphanDaemonsExactlyOnce(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	if got := countCalls(r.calls, "SweepOrphanDaemons"); got != 1 {
		t.Errorf("SweepOrphanDaemons call count = %d, want 1; calls=%v", got, r.calls)
	}
}

func TestOrchestratorRun_runsSweepOrphanDaemonsBetweenSetAndEnsureSaver(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	setIdx := indexOf(r.calls, "Set")
	sweepIdx := indexOf(r.calls, "SweepOrphanDaemons")
	saverIdx := indexOf(r.calls, "EnsureSaver")
	if setIdx == -1 || sweepIdx == -1 || saverIdx == -1 {
		t.Fatalf("expected Set, SweepOrphanDaemons, EnsureSaver in calls; got %v", r.calls)
	}
	if setIdx >= sweepIdx || sweepIdx >= saverIdx {
		t.Errorf("expected ordering Set < SweepOrphanDaemons < EnsureSaver; got Set=%d SweepOrphanDaemons=%d EnsureSaver=%d (%v)",
			setIdx, sweepIdx, saverIdx, r.calls)
	}
}

func TestOrchestratorRun_continuesPastSweepOrphanDaemonsFailure(t *testing.T) {
	sentinel := errors.New("orphan-sweep boom")
	r := &stepRecorder{SweepOrphanDaemonsErr: sentinel}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("SweepOrphanDaemons failure must not propagate; got %v", err)
	}

	if _, ok := errors.AsType[*FatalError](err); ok {
		t.Errorf("Run unexpectedly returned *FatalError on soft SweepOrphanDaemons failure: %v", err)
	}
	warn := sink.Records().WithMessage("step failed").AtExactLevel(slog.LevelWarn).Only(t, "step-failure WARN")
	if step := warn.AttrString(t, "step"); step != stepSweepOrphanDaemons {
		t.Errorf("step-failure WARN step = %q, want %q", step, stepSweepOrphanDaemons)
	}
	if got := warn.ErrorAttr(t, "error"); !errors.Is(got, sentinel) {
		t.Errorf("step-failure WARN error = %v, want %v", got, sentinel)
	}

	want := wantAllCalls
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_sweepOrphanDaemonsHappyPathEmitsNoWarn(t *testing.T) {
	r := &stepRecorder{}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	for _, rec := range sink.Records().AtExactLevel(slog.LevelWarn) {
		if rec.AttrOrEmpty("step") == stepSweepOrphanDaemons {
			t.Errorf("nil-returning SweepOrphanDaemons must not emit a Warn entry; got %+v", rec)
		}
	}
}

func TestOrchestratorRun_surfacesASaverThatNeverCameUpAsAWarningAndAbortsNoStep(t *testing.T) {
	r := &stepRecorder{EnsureSaverErr: fmt.Errorf("%w: last=identify-dead pid=4321", tmux.ErrSaverDaemonNotReady)}
	sink := &logtest.Sink{}
	o := newOrchestrator(r, slog.New(sink))

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not turn a saver that never came up into a fatal; got %v", err)
	}
	if len(warnings) != 1 || warnings[0].Lines[0] != SaverDownWarning().Lines[0] {
		t.Fatalf("warnings = %#v; want exactly one SaverDownWarning", warnings)
	}
	want := wantAllCalls
	if !slices.Equal(r.calls, want) {
		t.Errorf("calls = %v, want every step to run: %v", r.calls, want)
	}
}
