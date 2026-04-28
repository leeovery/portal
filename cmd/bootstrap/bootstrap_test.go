package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
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
	RestoreErr      error
	ClearErr        error
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

func (r *stepRecorder) Restore() error {
	r.calls = append(r.calls, "Restore")
	return r.RestoreErr
}

func (r *stepRecorder) CleanStale() error {
	r.calls = append(r.calls, "CleanStale")
	return r.CleanStaleErr
}

// recordingLogger captures Warn invocations so tests can assert that
// best-effort failures are surfaced through the Logger seam.
type recordingLogger struct {
	warnings []string
}

func (l *recordingLogger) Warn(component, format string, args ...any) {
	_ = component
	_ = format
	_ = args
	l.warnings = append(l.warnings, component)
}

// newOrchestrator wires a single stepRecorder into every step seam so the
// recorded call slice reflects the canonical eight-step ordering.
func newOrchestrator(r *stepRecorder, logger Logger) *Orchestrator {
	return &Orchestrator{
		Server:    r,
		Hooks:     r,
		Restoring: r,
		Saver:     r,
		Restore:   r,
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

	_, err := o.Run(context.Background())
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
		"CleanStale",
	}
	if !equalCalls(r.calls, want) {
		t.Errorf("call order = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_propagatesEnsureServerError(t *testing.T) {
	sentinel := errors.New("server boom")
	r := &stepRecorder{EnsureServerErr: sentinel}
	o := newOrchestrator(r, nil)

	_, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "step 1") {
		t.Errorf("expected error to mention step 1, got %q", err.Error())
	}
	want := []string{"EnsureServer"}
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_propagatesRegisterHooksError(t *testing.T) {
	sentinel := errors.New("register boom")
	r := &stepRecorder{RegisterErr: sentinel}
	o := newOrchestrator(r, nil)

	_, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "step 2") {
		t.Errorf("expected error to mention step 2, got %q", err.Error())
	}
	want := []string{"EnsureServer", "RegisterPortalHooks"}
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_propagatesSetRestoringErrorAndSkipsLaterSteps(t *testing.T) {
	sentinel := errors.New("set marker boom")
	r := &stepRecorder{SetErr: sentinel}
	o := newOrchestrator(r, nil)

	_, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "step 3") {
		t.Errorf("expected error to mention step 3, got %q", err.Error())
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

func TestOrchestratorRun_continuesPastEnsureSaverFailureAndRecordsLastSaverErr(t *testing.T) {
	sentinel := errors.New("saver boom")
	r := &stepRecorder{EnsureSaverErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

	_, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("Run must not return saver failures; got %v", err)
	}
	if o.LastSaverErr == nil {
		t.Fatal("expected LastSaverErr to be set, got nil")
	}
	if !errors.Is(o.LastSaverErr, sentinel) {
		t.Errorf("LastSaverErr = %v, want %v", o.LastSaverErr, sentinel)
	}
	want := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"EnsureSaver",
		"Restore",
		"Clear",
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
	sentinel := errors.New("restore boom")
	r := &stepRecorder{RestoreErr: sentinel}
	o := newOrchestrator(r, nil)

	_, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected restore error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
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
	o := newOrchestrator(r, nil)

	_, err := o.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "step 6") {
		t.Errorf("expected error to mention step 6, got %q", err.Error())
	}
}

func TestOrchestratorRun_continuesPastCleanStaleFailure(t *testing.T) {
	sentinel := errors.New("clean boom")
	r := &stepRecorder{CleanStaleErr: sentinel}
	logger := &recordingLogger{}
	o := newOrchestrator(r, logger)

	_, err := o.Run(context.Background())
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

	if _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("first Run errored: %v", err)
	}
	want := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"EnsureSaver",
		"Restore",
		"Clear",
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
	o.Clean = r2

	if _, err := o.Run(context.Background()); err != nil {
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

		started, err := o.Run(context.Background())
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

		started, err := o.Run(context.Background())
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

	_, _ = o.Run(context.Background())

	for _, c := range r.calls {
		if c == "EnsureSaver" {
			t.Fatalf("EnsureSaver must not run when Set fails; calls = %v", r.calls)
		}
	}
}

func TestSaverDownError_unwrapsCause(t *testing.T) {
	cause := errors.New("underlying")
	wrapped := &SaverDownError{Cause: cause}

	if !errors.Is(wrapped, cause) {
		t.Errorf("errors.Is(wrapped, cause) = false, want true")
	}
	if !strings.Contains(wrapped.Error(), "underlying") {
		t.Errorf("Error() = %q; expected to mention cause", wrapped.Error())
	}
}
