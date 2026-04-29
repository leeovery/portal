package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// stepRecorder is a single fake that satisfies every step interface and
// records the order in which methods were called. Per-method errors are
// configurable so each test can simulate failures at exact spec steps.
type stepRecorder struct {
	calls []string

	EnsureServerErr error
	RegisterErr     error
	SetErr          error
	EnsureSaverErr  error
	RestoreCorrupt  bool
	RestoreErr      error
	ClearErr        error
	SweepErr        error
	CleanStaleErr   error
	ServerStarted   bool
}

func (r *stepRecorder) EnsureServer() (bool, error) {
	r.calls = append(r.calls, "EnsureServer")
	return r.ServerStarted, r.EnsureServerErr
}

func (r *stepRecorder) RegisterPortalHooks() error {
	r.calls = append(r.calls, "RegisterPortalHooks")
	return r.RegisterErr
}

func (r *stepRecorder) Set() error {
	r.calls = append(r.calls, "Set")
	return r.SetErr
}

func (r *stepRecorder) Clear() error {
	r.calls = append(r.calls, "Clear")
	return r.ClearErr
}

func (r *stepRecorder) EnsureSaver() error {
	r.calls = append(r.calls, "EnsureSaver")
	return r.EnsureSaverErr
}

func (r *stepRecorder) Restore() (bool, error) {
	r.calls = append(r.calls, "Restore")
	return r.RestoreCorrupt, r.RestoreErr
}

func (r *stepRecorder) Sweep() error {
	r.calls = append(r.calls, "Sweep")
	return r.SweepErr
}

func (r *stepRecorder) CleanStale() error {
	r.calls = append(r.calls, "CleanStale")
	return r.CleanStaleErr
}

// recordingLogger captures Debug / Warn / Error invocations so tests can
// assert that step-entry diagnostics land at DEBUG, best-effort failures land
// via Warn, and fatal failures land via Error before the orchestrator
// returns. Entries are the fully formatted message so tests can verify
// wrapped causes (e.g. step-7 Sweep error) flow through to portal.log
// unchanged.
type recordingLogger struct {
	debugs   []string
	warnings []string
	errors   []string
}

func (l *recordingLogger) Debug(component, format string, args ...any) {
	_ = component
	l.debugs = append(l.debugs, fmt.Sprintf(format, args...))
}

func (l *recordingLogger) Warn(component, format string, args ...any) {
	_ = component
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
}

func (l *recordingLogger) Error(component, format string, args ...any) {
	_ = component
	l.errors = append(l.errors, fmt.Sprintf(format, args...))
}

// newOrchestrator wires a single stepRecorder into every step seam so the
// recorded call slice reflects the canonical nine-step ordering.
func newOrchestrator(r *stepRecorder, logger Logger) *Orchestrator {
	return &Orchestrator{
		Server:    r,
		Hooks:     r,
		Restoring: r,
		Saver:     r,
		Restore:   r,
		Sweeper:   r,
		Clean:     r,
		Logger:    logger,
	}
}

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

	want := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"EnsureSaver",
		"Restore",
		"Clear",
		"Sweep",
		"CleanStale",
	}
	if !equalCalls(r.calls, want) {
		t.Errorf("call order = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_propagatesEnsureServerError(t *testing.T) {
	sentinel := errors.New("server boom")
	r := &stepRecorder{EnsureServerErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

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
	wantPrefix := "Portal failed to start tmux server: "
	if !strings.HasPrefix(fatal.UserMessage, wantPrefix) {
		t.Errorf("UserMessage = %q, want prefix %q", fatal.UserMessage, wantPrefix)
	}
	if !strings.Contains(fatal.UserMessage, "server boom") {
		t.Errorf("UserMessage = %q, want to contain underlying %q", fatal.UserMessage, "server boom")
	}
	if len(logger.errors) == 0 {
		t.Error("expected logger.Error to be called before fatal return")
	}
	want := []string{"EnsureServer"}
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_propagatesRegisterHooksError(t *testing.T) {
	sentinel := errors.New("register boom")
	r := &stepRecorder{RegisterErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

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
	wantPrefix := "Portal failed to register tmux hooks: "
	if !strings.HasPrefix(fatal.UserMessage, wantPrefix) {
		t.Errorf("UserMessage = %q, want prefix %q", fatal.UserMessage, wantPrefix)
	}
	if !strings.Contains(fatal.UserMessage, "register boom") {
		t.Errorf("UserMessage = %q, want to contain underlying %q", fatal.UserMessage, "register boom")
	}
	if len(logger.errors) == 0 {
		t.Error("expected logger.Error to be called before fatal return")
	}
	want := []string{"EnsureServer", "RegisterPortalHooks"}
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_propagatesSetRestoringErrorAndSkipsLaterSteps(t *testing.T) {
	sentinel := errors.New("set marker boom")
	r := &stepRecorder{SetErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

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
	if len(logger.errors) == 0 {
		t.Error("expected logger.Error to be called before fatal return")
	}
	for _, c := range r.calls {
		if c == "EnsureSaver" {
			t.Errorf("EnsureSaver must not run when Set fails; calls = %v", r.calls)
		}
		if c == "Restore" {
			t.Errorf("Restore must not run when Set fails; calls = %v", r.calls)
		}
	}
	// Set must have been called.
	found := false
	for _, c := range r.calls {
		if c == "Set" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected Set in calls, got %v", r.calls)
	}
}

func TestOrchestratorRun_continuesPastEnsureSaverFailureAndAppendsWarning(t *testing.T) {
	sentinel := errors.New("saver boom")
	r := &stepRecorder{EnsureSaverErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

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
	want := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"EnsureSaver",
		"Restore",
		"Clear",
		"Sweep",
		"CleanStale",
	}
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
	if len(logger.warnings) == 0 {
		t.Error("expected logger to record at least one warning")
	}
}

func TestOrchestratorRun_clearsRestoringEvenWhenRestoreFails(t *testing.T) {
	// Restore reports a corrupt-index failure; per spec the orchestrator
	// treats it as soft and continues, but step 6 (Clear) MUST still run
	// before Run returns so the @portal-restoring window does not leak.
	corruptErr := fmt.Errorf("restore: %w", state.ErrCorruptIndex)
	r := &stepRecorder{RestoreCorrupt: true, RestoreErr: corruptErr}
	o := newOrchestrator(r, nil)

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must treat corrupt-index restore as soft; got %v", err)
	}

	// Clear must appear after Restore.
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
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

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
	if len(logger.errors) == 0 {
		t.Error("expected logger.Error to be called before fatal return")
	}
}

func TestOrchestratorRun_continuesPastCleanStaleFailure(t *testing.T) {
	sentinel := errors.New("clean boom")
	r := &stepRecorder{CleanStaleErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("CleanStale failure must not propagate; got %v", err)
	}
	if len(logger.warnings) == 0 {
		t.Error("expected logger to record at least one warning")
	}
}

func TestOrchestratorRun_isIdempotentAcrossInvocations(t *testing.T) {
	r1 := &stepRecorder{}
	o := newOrchestrator(r1, nil)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("first Run errored: %v", err)
	}
	want := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"EnsureSaver",
		"Restore",
		"Clear",
		"Sweep",
		"CleanStale",
	}
	if !equalCalls(r1.calls, want) {
		t.Errorf("first calls = %v, want %v", r1.calls, want)
	}

	r2 := &stepRecorder{}
	o.Server = r2
	o.Hooks = r2
	o.Restoring = r2
	o.Saver = r2
	o.Restore = r2
	o.Sweeper = r2
	o.Clean = r2

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("second Run errored: %v", err)
	}
	if !equalCalls(r2.calls, want) {
		t.Errorf("second calls = %v, want %v", r2.calls, want)
	}
}

func TestOrchestratorRun_returnsServerStartedFlagFromEnsureServer(t *testing.T) {
	t.Run("true when EnsureServer reports started", func(t *testing.T) {
		r := &stepRecorder{ServerStarted: true}
		o := newOrchestrator(r, nil)

		started, _, err := o.Run(context.Background())
		if err != nil {
			t.Fatalf("Run errored: %v", err)
		}
		if !started {
			t.Error("expected serverStarted=true, got false")
		}
	})

	t.Run("false when EnsureServer reports not started", func(t *testing.T) {
		r := &stepRecorder{ServerStarted: false}
		o := newOrchestrator(r, nil)

		started, _, err := o.Run(context.Background())
		if err != nil {
			t.Fatalf("Run errored: %v", err)
		}
		if started {
			t.Error("expected serverStarted=false, got true")
		}
	})
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
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not surface saver failures; got %v", err)
	}

	// Regression: saver failure must travel through the warnings slice and
	// the Warn logger — not the FatalError type that triggers stderr+exit.
	var fatal *FatalError
	if errors.As(err, &fatal) {
		t.Errorf("Run unexpectedly returned *FatalError on soft saver failure: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings len = %d, want 1; got %#v", len(warnings), warnings)
	}
	if warnings[0].Lines[0] != SaverDownWarning().Lines[0] {
		t.Errorf("warnings[0] = %q, want SaverDownWarning", warnings[0].Lines[0])
	}
	if len(logger.errors) != 0 {
		t.Errorf("logger.Error must NOT be called for soft saver failure; got %v", logger.errors)
	}
	if len(logger.warnings) == 0 {
		t.Error("expected logger.Warn to be called for soft saver failure")
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
	corruptErr := fmt.Errorf("restore: %w", state.ErrCorruptIndex)
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
		RestoreErr:     fmt.Errorf("restore: %w", state.ErrCorruptIndex),
	}
	o := newOrchestrator(r, nil)

	_, warnings, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must treat both as soft; got err=%v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings len = %d, want 2; got %#v", len(warnings), warnings)
	}
	// Order matches step order: SaverDownWarning (step 4) first,
	// CorruptSessionsJSONWarning (step 5) second.
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
		RestoreErr:     fmt.Errorf("restore: %w", state.ErrCorruptIndex),
	}
	o := newOrchestrator(r, nil)

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("expected nil err for soft corrupt-index path; got %v", err)
	}
}

// TestOrchestratorRun_doesNotEscalateNonCorruptRestoreError is the contract
// guard for task 7-10: a future Restorer implementation that violates the
// (corrupt bool, err error) contract by returning (false, err) for a soft
// per-session failure MUST NOT escalate that error to a PersistentPreRunE
// abort. The orchestrator logs and continues — the spec's degrade-locally
// principle wins over a chatty implementation.
func TestOrchestratorRun_doesNotEscalateNonCorruptRestoreError(t *testing.T) {
	sentinel := errors.New("restore boom")
	r := &stepRecorder{RestoreCorrupt: false, RestoreErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must NOT escalate a non-corrupt soft restore error; got %v", err)
	}
	if len(logger.warnings) == 0 {
		t.Error("expected logger.Warn to record the contract-violating soft failure")
	}
	if len(logger.errors) != 0 {
		t.Errorf("logger.Error must NOT be called for a soft restore failure; got %v", logger.errors)
	}
}

// TestOrchestratorRun_doesNotEmitCorruptWarningWhenCorruptFalse confirms
// that the orchestrator only emits CorruptSessionsJSONWarning when the
// implementation reports corrupt=true. A non-corrupt soft failure is
// logged but does not produce a user-facing warning surface.
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

// TestOrchestratorRun_runsSweepBetweenClearAndCleanStale pins the FIFO
// sweep step at position 7 of the nine-step sequence: after Clear (step 6)
// so the @portal-restoring suppression window has closed, but before
// CleanStale (step 8) so the per-pane skeleton markers it observes are
// still set on the live tmux server (skeleton markers outlive the
// @portal-restoring marker; they are cleared per-pane on hydration).
func TestOrchestratorRun_runsSweepBetweenClearAndCleanStale(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	clearIdx, sweepIdx, cleanIdx := -1, -1, -1
	for i, c := range r.calls {
		switch c {
		case "Clear":
			clearIdx = i
		case "Sweep":
			sweepIdx = i
		case "CleanStale":
			cleanIdx = i
		}
	}
	if clearIdx == -1 || sweepIdx == -1 || cleanIdx == -1 {
		t.Fatalf("expected Clear, Sweep, CleanStale in calls; got %v", r.calls)
	}
	if clearIdx >= sweepIdx || sweepIdx >= cleanIdx {
		t.Errorf("expected ordering Clear < Sweep < CleanStale; got Clear=%d Sweep=%d CleanStale=%d (%v)",
			clearIdx, sweepIdx, cleanIdx, r.calls)
	}
}

// TestOrchestratorRun_continuesPastSweepFailure proves the FIFO sweep is
// best-effort: a Sweep error MUST NOT short-circuit Run, MUST NOT produce
// a *FatalError, and MUST log via Warn so the failure is observable in
// portal.log. The Warn message MUST embed the underlying cause so the
// FIFOSweeper adapter's wrapped marker-enumeration errors travel through
// to portal.log unchanged.
func TestOrchestratorRun_continuesPastSweepFailure(t *testing.T) {
	sentinel := errors.New("sweep boom")
	r := &stepRecorder{SweepErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Sweep failure must not propagate; got %v", err)
	}

	var fatal *FatalError
	if errors.As(err, &fatal) {
		t.Errorf("Run unexpectedly returned *FatalError on soft sweep failure: %v", err)
	}
	if len(logger.warnings) == 0 {
		t.Fatal("expected logger.Warn to record the soft sweep failure")
	}

	// The Warn message MUST embed the underlying cause so the
	// FIFOSweeper adapter's wrapped error (e.g. "list skeleton markers:
	// <cause>") is preserved verbatim in portal.log.
	foundCause := false
	for _, msg := range logger.warnings {
		if strings.Contains(msg, sentinel.Error()) && strings.Contains(msg, "step 7") {
			foundCause = true
			break
		}
	}
	if !foundCause {
		t.Errorf("expected a step-7 Warn message containing %q; got %v", sentinel.Error(), logger.warnings)
	}

	// CleanStale must still run after a sweep failure.
	cleanRan := false
	for _, c := range r.calls {
		if c == "CleanStale" {
			cleanRan = true
			break
		}
	}
	if !cleanRan {
		t.Errorf("CleanStale must run even when Sweep fails; calls = %v", r.calls)
	}
}

// Compile-time assertion: *Orchestrator satisfies Runner. cmd/root.go's
// BootstrapDeps.Orchestrator field is typed as Runner; this guards
// against future drift in either the interface or the concrete type.
var _ Runner = (*Orchestrator)(nil)

// TestOrchestratorRun_emitsDebugLinePerExecutedStep is the spec-Observability
// guard: bootstrap events emit at DEBUG. Each of the eight executed steps in
// the happy path MUST log at least one DEBUG line on entry so portal.log
// (when PORTAL_LOG_LEVEL=debug) records the step boundary the operator can
// scan when a session fails to come back. Names match the canonical labels
// emitted by Run; if a step is renamed, this test pins the rename to land
// here as well.
func TestOrchestratorRun_emitsDebugLinePerExecutedStep(t *testing.T) {
	r := &stepRecorder{}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	// Each executed step must produce at least one DEBUG line whose
	// message references that step's canonical label.
	steps := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"EnsureSaver",
		"Restore",
		"Clear",
		"Sweep",
		"CleanStale",
	}
	for _, step := range steps {
		matches := 0
		for _, line := range logger.debugs {
			if strings.Contains(line, step) {
				matches++
			}
		}
		if matches < 1 {
			t.Errorf("step %q: expected ≥1 DEBUG line referencing it; got debugs=%v", step, logger.debugs)
		}
	}
}
