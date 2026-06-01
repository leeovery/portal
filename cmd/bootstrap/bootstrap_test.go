package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// stepRecorder is a single fake that satisfies every step interface and
// records the order in which methods were called. Per-method errors are
// configurable so each test can simulate failures at exact spec steps.
type stepRecorder struct {
	calls []string

	EnsureServerErr       error
	RegisterErr           error
	SetErr                error
	SweepOrphanDaemonsErr error
	EnsureSaverErr        error
	RestoreCorrupt        bool
	RestoreErr            error
	EagerSignalHydrateErr error
	ClearErr              error
	CleanStaleMarkersErr  error
	SweepErr              error
	CleanStaleErr         error
	ServerStarted         bool
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

func (r *stepRecorder) EagerSignalHydrate() error {
	r.calls = append(r.calls, "EagerSignalHydrate")
	return r.EagerSignalHydrateErr
}

func (r *stepRecorder) CleanStaleMarkers() error {
	r.calls = append(r.calls, "CleanStaleMarkers")
	return r.CleanStaleMarkersErr
}

func (r *stepRecorder) Sweep() error {
	r.calls = append(r.calls, "Sweep")
	return r.SweepErr
}

func (r *stepRecorder) CleanStale() error {
	r.calls = append(r.calls, "CleanStale")
	return r.CleanStaleErr
}

// RecordingLogger is a slog.Handler that captures Debug / Info / Warn /
// Error records so tests can assert that step-entry diagnostics land at
// DEBUG, best-effort failures via Warn, and fatal failures via Error.
//
// Each captured record is rendered into a level-specific slice of message
// phrases (the slog message + a flattened "key=value" attr trailer) so the
// pre-migration substring assertions keep working against the post-migration
// terse-message-plus-attrs shape. Parallel *Components slices record the
// component attr supplied via log.For so tests can pin component routing
// (e.g. "bootstrap") without a real on-disk logger.
//
// Use Logger() to obtain a *slog.Logger to inject into a step core's Logger
// field; the records route back into this recorder. Exported so external
// `package bootstrap_test` tests can share it.
type RecordingLogger struct {
	debugs          []string
	debugComponents []string
	infos           []string
	infoComponents  []string
	warnings        []string
	warnComponents  []string
	errors          []string
	errorComponents []string

	// shared points at the slice-owning recorder so handlers derived via
	// WithAttrs/WithGroup record back into the same buffers; nil on the root.
	shared *RecordingLogger
	// bound holds attrs accumulated via WithAttrs (notably the component).
	bound []slog.Attr
}

// recordingLoggerHandler is the slog.Handler returned by
// RecordingLogger.WithAttrs/WithGroup. It carries the accumulated bound attrs
// and replays them onto every record routed back into the owning recorder.
type recordingLoggerHandler struct {
	owner *RecordingLogger
	bound []slog.Attr
}

func (h *recordingLoggerHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *recordingLoggerHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(h.bound)+len(attrs))
	next = append(next, h.bound...)
	next = append(next, attrs...)
	return &recordingLoggerHandler{owner: h.owner, bound: next}
}

func (h *recordingLoggerHandler) WithGroup(_ string) slog.Handler {
	return &recordingLoggerHandler{owner: h.owner, bound: h.bound}
}

func (h *recordingLoggerHandler) Handle(_ context.Context, r slog.Record) error {
	return h.owner.record(h.bound, r)
}

// Logger returns a *slog.Logger whose records are captured by this recorder.
func (l *RecordingLogger) Logger() *slog.Logger { return slog.New(l) }

// Enabled records every level.
func (l *RecordingLogger) Enabled(_ context.Context, _ slog.Level) bool { return true }

// WithAttrs returns a handler that records the bound attrs (notably the
// component bound via .With("component", ...)) and replays them onto every
// record routed back into this recorder. Without accumulating the bound attrs
// the component — attached by the production logger via log.For/.With, not at
// each call site — would be lost from the captured records.
func (l *RecordingLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(l.bound)+len(attrs))
	next = append(next, l.bound...)
	next = append(next, attrs...)
	return &recordingLoggerHandler{owner: l.owner(), bound: next}
}

// WithGroup is not exercised by these tests; it returns a handler preserving
// the bound attrs.
func (l *RecordingLogger) WithGroup(_ string) slog.Handler {
	return &recordingLoggerHandler{owner: l.owner(), bound: l.bound}
}

func (l *RecordingLogger) owner() *RecordingLogger {
	if l.shared != nil {
		return l.shared
	}
	return l
}

// Handle captures one record into the level-specific slices.
func (l *RecordingLogger) Handle(_ context.Context, r slog.Record) error {
	return l.record(l.bound, r)
}

// record renders a record (prefixed by the supplied bound attrs) into the
// owning recorder's level slices.
func (l *RecordingLogger) record(bound []slog.Attr, r slog.Record) error {
	owner := l.owner()
	var component string
	var trailer strings.Builder
	emit := func(a slog.Attr) bool {
		if a.Key == "component" {
			component = a.Value.String()
			return true
		}
		trailer.WriteString(" ")
		trailer.WriteString(a.Key)
		trailer.WriteString("=")
		trailer.WriteString(a.Value.String())
		return true
	}
	for _, a := range bound {
		emit(a)
	}
	r.Attrs(func(a slog.Attr) bool { return emit(a) })
	msg := r.Message + trailer.String()
	switch r.Level {
	case slog.LevelDebug:
		owner.debugs = append(owner.debugs, msg)
		owner.debugComponents = append(owner.debugComponents, component)
	case slog.LevelInfo:
		owner.infos = append(owner.infos, msg)
		owner.infoComponents = append(owner.infoComponents, component)
	case slog.LevelWarn:
		owner.warnings = append(owner.warnings, msg)
		owner.warnComponents = append(owner.warnComponents, component)
	case slog.LevelError:
		owner.errors = append(owner.errors, msg)
		owner.errorComponents = append(owner.errorComponents, component)
	}
	return nil
}

// AllEntries returns every recorded log entry across all four levels in
// "<LEVEL>: <msg>" form for diagnostic output. Levels are emitted in
// fixed order (DEBUG, INFO, WARN, ERROR), not chronological — sufficient
// for the audit-log substring assertions that are this method's sole use.
func (l *RecordingLogger) AllEntries() []string {
	out := make([]string, 0, len(l.debugs)+len(l.infos)+len(l.warnings)+len(l.errors))
	for _, m := range l.debugs {
		out = append(out, "DEBUG: "+m)
	}
	for _, m := range l.infos {
		out = append(out, "INFO: "+m)
	}
	for _, m := range l.warnings {
		out = append(out, "WARN: "+m)
	}
	for _, m := range l.errors {
		out = append(out, "ERROR: "+m)
	}
	return out
}

// newOrchestrator wires a single stepRecorder into every step seam so the
// recorded call slice reflects the canonical eleven-step ordering.
func newOrchestrator(r *stepRecorder, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		Server:        r,
		Hooks:         r,
		Restoring:     r,
		OrphanSweeper: r,
		Saver:         r,
		Restore:       r,
		EagerSignaler: r,
		StaleMarkers:  r,
		Sweeper:       r,
		Clean:         r,
		Logger:        logger,
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
		"SweepOrphanDaemons",
		"EnsureSaver",
		"Restore",
		"EagerSignalHydrate",
		"Clear",
		"CleanStaleMarkers",
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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
		"SweepOrphanDaemons",
		"EnsureSaver",
		"Restore",
		"EagerSignalHydrate",
		"Clear",
		"CleanStaleMarkers",
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

// TestOrchestratorRun_continuesPastEagerSignalHydrateFailure proves the
// eager-signal step is best-effort, mirroring the soft-warning posture of
// CleanStaleMarkers (step 9), Sweep (step 10), and CleanStale (step 11):
// an EagerSignalHydrate error MUST NOT short-circuit Run, MUST NOT produce
// a *FatalError, and MUST log via Warn so the failure is observable in
// portal.log. The Warn message MUST embed the canonical step label
// "step 7 (EagerSignalHydrate) failed" so adapter-wrapped errors travel
// through to portal.log unchanged. Clear, CleanStaleMarkers, Sweep, and
// CleanStale MUST still run after an eager-signal failure.
func TestOrchestratorRun_continuesPastEagerSignalHydrateFailure(t *testing.T) {
	sentinel := errors.New("eager-signal boom")
	r := &stepRecorder{EagerSignalHydrateErr: sentinel}
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("EagerSignalHydrate failure must not propagate; got %v", err)
	}

	var fatal *FatalError
	if errors.As(err, &fatal) {
		t.Errorf("Run unexpectedly returned *FatalError on soft EagerSignalHydrate failure: %v", err)
	}
	if len(logger.warnings) == 0 {
		t.Fatal("expected logger.Warn to record the soft EagerSignalHydrate failure")
	}

	// The Warn message MUST embed the canonical step label and the
	// underlying cause so adapter-wrapped errors are preserved verbatim
	// in portal.log.
	foundCause := false
	for _, msg := range logger.warnings {
		if strings.Contains(msg, "step failed") && strings.Contains(msg, "EagerSignalHydrate") && strings.Contains(msg, sentinel.Error()) {
			foundCause = true
			break
		}
	}
	if !foundCause {
		t.Errorf("expected a step-7 (EagerSignalHydrate) Warn message containing %q; got %v", sentinel.Error(), logger.warnings)
	}

	// All downstream steps must still run after an eager-signal failure.
	want := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"SweepOrphanDaemons",
		"EnsureSaver",
		"Restore",
		"EagerSignalHydrate",
		"Clear",
		"CleanStaleMarkers",
		"Sweep",
		"CleanStale",
	}
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

func TestOrchestratorRun_clearsRestoringEvenWhenRestoreFails(t *testing.T) {
	// Restore reports a corrupt-index failure; per spec the orchestrator
	// treats it as soft and continues, but step 8 (Clear) MUST still run
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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
		"SweepOrphanDaemons",
		"EnsureSaver",
		"Restore",
		"EagerSignalHydrate",
		"Clear",
		"CleanStaleMarkers",
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
	o.OrphanSweeper = r2
	o.Saver = r2
	o.Restore = r2
	o.EagerSignaler = r2
	o.StaleMarkers = r2
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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
	// Order matches step order: SaverDownWarning (step 5) first,
	// CorruptSessionsJSONWarning (step 6) second.
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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
// sweep step at position 10 of the eleven-step sequence: after Clear (step 8)
// and CleanStaleMarkers (step 9) so the @portal-restoring suppression
// window has closed and stale markers protecting orphan FIFOs have been
// unset, but before CleanStale (step 11). The CleanStaleMarkers step MUST
// also fall between Clear and Sweep so any stale markers protecting
// orphan FIFOs are unset before SweepOrphanFIFOs reclaims those FIFOs.
func TestOrchestratorRun_runsSweepBetweenClearAndCleanStale(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	clearIdx, cleanMarkersIdx, sweepIdx, cleanIdx := -1, -1, -1, -1
	for i, c := range r.calls {
		switch c {
		case "Clear":
			clearIdx = i
		case "CleanStaleMarkers":
			cleanMarkersIdx = i
		case "Sweep":
			sweepIdx = i
		case "CleanStale":
			cleanIdx = i
		}
	}
	if clearIdx == -1 || cleanMarkersIdx == -1 || sweepIdx == -1 || cleanIdx == -1 {
		t.Fatalf("expected Clear, CleanStaleMarkers, Sweep, CleanStale in calls; got %v", r.calls)
	}
	if clearIdx >= cleanMarkersIdx || cleanMarkersIdx >= sweepIdx || sweepIdx >= cleanIdx {
		t.Errorf("expected ordering Clear < CleanStaleMarkers < Sweep < CleanStale; got Clear=%d CleanStaleMarkers=%d Sweep=%d CleanStale=%d (%v)",
			clearIdx, cleanMarkersIdx, sweepIdx, cleanIdx, r.calls)
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
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

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
		if strings.Contains(msg, sentinel.Error()) && strings.Contains(msg, "SweepOrphanFIFOs") {
			foundCause = true
			break
		}
	}
	if !foundCause {
		t.Errorf("expected a step-10 Warn message containing %q; got %v", sentinel.Error(), logger.warnings)
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

// TestOrchestratorRun_runsCleanStaleMarkersBetweenClearAndSweep pins the
// stale-marker cleanup step at position 9 of the eleven-step sequence:
// strictly after Clear (step 8) so it observes the post-restore tmux
// state, and strictly before Sweep (step 10) so any stale markers
// protecting orphan FIFOs are unset first, allowing those FIFOs to be
// reclaimed in the same bootstrap. CleanStale (step 11) follows.
func TestOrchestratorRun_runsCleanStaleMarkersBetweenClearAndSweep(t *testing.T) {
	r := &stepRecorder{}
	o := newOrchestrator(r, nil)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	clearIdx, cleanMarkersIdx, sweepIdx, cleanIdx := -1, -1, -1, -1
	for i, c := range r.calls {
		switch c {
		case "Clear":
			clearIdx = i
		case "CleanStaleMarkers":
			cleanMarkersIdx = i
		case "Sweep":
			sweepIdx = i
		case "CleanStale":
			cleanIdx = i
		}
	}
	if clearIdx == -1 || cleanMarkersIdx == -1 || sweepIdx == -1 || cleanIdx == -1 {
		t.Fatalf("expected Clear, CleanStaleMarkers, Sweep, CleanStale in calls; got %v", r.calls)
	}
	if clearIdx >= cleanMarkersIdx || cleanMarkersIdx >= sweepIdx || sweepIdx >= cleanIdx {
		t.Errorf("expected ordering Clear < CleanStaleMarkers < Sweep < CleanStale; got Clear=%d CleanStaleMarkers=%d Sweep=%d CleanStale=%d (%v)",
			clearIdx, cleanMarkersIdx, sweepIdx, cleanIdx, r.calls)
	}
}

// TestOrchestratorRun_continuesPastCleanStaleMarkersFailure proves the
// stale-marker cleanup step is best-effort, mirroring the soft-warning
// posture of CleanStale (step 11) and Sweep (step 10): a CleanStaleMarkers
// error MUST NOT short-circuit Run, MUST NOT produce a *FatalError, and
// MUST log via Warn so the failure is observable in portal.log. The Warn
// message MUST embed the canonical step label "step 9 (CleanStaleMarkers)"
// and the underlying cause so adapter-wrapped errors travel through to
// portal.log unchanged. Sweep and CleanStale MUST still run after a
// stale-marker cleanup failure.
func TestOrchestratorRun_continuesPastCleanStaleMarkersFailure(t *testing.T) {
	sentinel := errors.New("clean stale markers boom")
	r := &stepRecorder{CleanStaleMarkersErr: sentinel}
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("CleanStaleMarkers failure must not propagate; got %v", err)
	}

	var fatal *FatalError
	if errors.As(err, &fatal) {
		t.Errorf("Run unexpectedly returned *FatalError on soft CleanStaleMarkers failure: %v", err)
	}
	if len(logger.warnings) == 0 {
		t.Fatal("expected logger.Warn to record the soft CleanStaleMarkers failure")
	}

	// The Warn message MUST embed the canonical step label and the
	// underlying cause so adapter-wrapped errors are preserved verbatim
	// in portal.log.
	foundCause := false
	for _, msg := range logger.warnings {
		if strings.Contains(msg, sentinel.Error()) && strings.Contains(msg, "step failed") && strings.Contains(msg, "CleanStaleMarkers") {
			foundCause = true
			break
		}
	}
	if !foundCause {
		t.Errorf("expected a step-9 (CleanStaleMarkers) Warn message containing %q; got %v", sentinel.Error(), logger.warnings)
	}

	// Sweep and CleanStale must still run after a CleanStaleMarkers failure.
	sweepRan, cleanRan := false, false
	for _, c := range r.calls {
		if c == "Sweep" {
			sweepRan = true
		}
		if c == "CleanStale" {
			cleanRan = true
		}
	}
	if !sweepRan {
		t.Errorf("Sweep must run even when CleanStaleMarkers fails; calls = %v", r.calls)
	}
	if !cleanRan {
		t.Errorf("CleanStale must run even when CleanStaleMarkers fails; calls = %v", r.calls)
	}
}

// Compile-time assertion: *Orchestrator satisfies Runner. cmd/root.go's
// BootstrapDeps.Orchestrator field is typed as Runner; this guards
// against future drift in either the interface or the concrete type.
var _ Runner = (*Orchestrator)(nil)

// TestOrchestratorRun_emitsDebugLinePerExecutedStep is the spec-Observability
// guard: bootstrap events emit at DEBUG. Each of the nine executed steps in
// the happy path MUST log at least one DEBUG line on entry so portal.log
// (when PORTAL_LOG_LEVEL=debug) records the step boundary the operator can
// scan when a session fails to come back. Names match the canonical labels
// emitted by Run; if a step is renamed, this test pins the rename to land
// here as well.
func TestOrchestratorRun_emitsDebugLinePerExecutedStep(t *testing.T) {
	r := &stepRecorder{}
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	// Each executed step must produce at least one DEBUG line whose
	// message references that step's canonical label.
	steps := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"SweepOrphanDaemons",
		"EnsureSaver",
		"Restore",
		"EagerSignalHydrate",
		"Clear",
		"CleanStaleMarkers",
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

// TestOrchestratorRun_stepEntryLinesEmitUnderBootstrapComponent is the Task 1-9
// acceptance for the bootstrap orchestrator's step-entry instrumentation: each
// "step entering" DEBUG line is bound to component=bootstrap (the component the
// production wiring binds via log.For("bootstrap")). Binds the component on the
// injected logger and asserts every recorded "step entering" DEBUG carries it.
func TestOrchestratorRun_stepEntryLinesEmitUnderBootstrapComponent(t *testing.T) {
	r := &stepRecorder{}
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger().With("component", "bootstrap"))

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	var stepEntries int
	for i, line := range logger.debugs {
		if !strings.Contains(line, "step entering") {
			continue
		}
		stepEntries++
		if logger.debugComponents[i] != "bootstrap" {
			t.Errorf("step-entry DEBUG[%d] component = %q, want %q (line=%q)",
				i, logger.debugComponents[i], "bootstrap", line)
		}
	}
	if stepEntries == 0 {
		t.Errorf("expected ≥1 'step entering' DEBUG line; got debugs=%v", logger.debugs)
	}
}

// countCalls returns how many times name appears in calls.
func countCalls(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

// indexOf returns the first index of name in calls, or -1 if absent.
func indexOf(calls []string, name string) int {
	for i, c := range calls {
		if c == name {
			return i
		}
	}
	return -1
}

// TestOrchestratorRun_invokesSweepOrphanDaemonsExactlyOnce pins task 4-4's
// structural acceptance: the new orchestrator step calls
// OrphanSweeper.SweepOrphanDaemons exactly once per Run.
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

// TestOrchestratorRun_runsSweepOrphanDaemonsBetweenSetAndEnsureSaver pins the
// orphan-daemon sweep at position 4 of the eleven-step sequence: strictly
// after Set @portal-restoring (step 3) and strictly before EnsureSaver
// (step 5). This is the spec invariant — orphans must die before the new
// saver-pane daemon comes up so the new daemon's first tick is uncontested.
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

// TestOrchestratorRun_continuesPastSweepOrphanDaemonsFailure proves the
// orphan-daemon sweep is best-effort, mirroring the soft-warning posture of
// EnsureSaver, EagerSignalHydrate, CleanStaleMarkers, Sweep, and CleanStale:
// a SweepOrphanDaemons error MUST NOT short-circuit Run, MUST NOT produce a
// *FatalError, and MUST log via Warn so the failure is observable in
// portal.log. The Warn message MUST embed the canonical step label
// "step 4 (SweepOrphanDaemons) failed" and the underlying cause so
// adapter-wrapped errors travel through to portal.log unchanged. All
// downstream steps MUST still run after a sweep failure.
func TestOrchestratorRun_continuesPastSweepOrphanDaemonsFailure(t *testing.T) {
	sentinel := errors.New("orphan-sweep boom")
	r := &stepRecorder{SweepOrphanDaemonsErr: sentinel}
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Fatalf("SweepOrphanDaemons failure must not propagate; got %v", err)
	}

	var fatal *FatalError
	if errors.As(err, &fatal) {
		t.Errorf("Run unexpectedly returned *FatalError on soft SweepOrphanDaemons failure: %v", err)
	}
	if len(logger.warnings) == 0 {
		t.Fatal("expected logger.Warn to record the soft SweepOrphanDaemons failure")
	}

	foundCause := false
	for _, msg := range logger.warnings {
		if strings.Contains(msg, "step failed") && strings.Contains(msg, "SweepOrphanDaemons") && strings.Contains(msg, sentinel.Error()) {
			foundCause = true
			break
		}
	}
	if !foundCause {
		t.Errorf("expected a step-4 (SweepOrphanDaemons) Warn message containing %q; got %v", sentinel.Error(), logger.warnings)
	}

	// All downstream steps must still run after a sweep failure.
	want := []string{
		"EnsureServer",
		"RegisterPortalHooks",
		"Set",
		"SweepOrphanDaemons",
		"EnsureSaver",
		"Restore",
		"EagerSignalHydrate",
		"Clear",
		"CleanStaleMarkers",
		"Sweep",
		"CleanStale",
	}
	if !equalCalls(r.calls, want) {
		t.Errorf("calls = %v, want %v", r.calls, want)
	}
}

// TestOrchestratorRun_sweepOrphanDaemonsHappyPathEmitsNoWarn confirms the
// happy-path log discipline: a nil-returning SweepOrphanDaemons emits no
// Warn entries. Combined with the negative test above, this pins the
// log-on-failure-only contract that step 4 inherits from the other
// best-effort steps.
func TestOrchestratorRun_sweepOrphanDaemonsHappyPathEmitsNoWarn(t *testing.T) {
	r := &stepRecorder{}
	logger := &RecordingLogger{}
	o := newOrchestrator(r, logger.Logger())

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run errored: %v", err)
	}

	for _, msg := range logger.warnings {
		if strings.Contains(msg, "SweepOrphanDaemons") {
			t.Errorf("nil-returning SweepOrphanDaemons must not emit a Warn entry; got %q", msg)
		}
	}
}
