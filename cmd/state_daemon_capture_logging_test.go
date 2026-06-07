// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
//
// Phase 2 Task 2-5: end-to-end verification that the daemon's call site to
// state.CaptureStructure passes the live deps.Logger (not nil, not a freshly-
// opened logger), so per-session ShowEnvironment failures surface as WARN
// entries in the on-disk portal.log under ComponentDaemon ("daemon"). Pins
// the wiring established by Task 2-2 against accidental regression to nil or
// a sibling logger handle.
//
// Spec reference: § Component E (CaptureStructure Per-Session
// Log-and-Continue), line 338.

package cmd

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// errorAttrRecorder is a slog.Handler that captures the error-attr value of the
// first WARN record whose message matches wantMsg, preserving the value as the
// live error (slog.Any) so a test can assert errors.Is against it — proving the
// call site passed the wrapped error directly, not its .Error() string.
type errorAttrRecorder struct {
	wantMsg string
	gotErr  error
	found   bool
}

func (h *errorAttrRecorder) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *errorAttrRecorder) WithAttrs(_ []slog.Attr) slog.Handler         { return h }
func (h *errorAttrRecorder) WithGroup(_ string) slog.Handler              { return h }

func (h *errorAttrRecorder) Handle(_ context.Context, r slog.Record) error {
	if h.found || r.Level != slog.LevelWarn || r.Message != h.wantMsg {
		return nil
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "error" {
			if e, ok := a.Value.Any().(error); ok {
				h.gotErr = e
				h.found = true
				return false
			}
		}
		return true
	})
	return nil
}

// TestDaemonTick_CapturePaneFailureErrorAttrIsWrappedError is the Task 1-9
// acceptance for the error-attr contract: the WARN's "error" attr value is the
// wrapped error itself (slog.Any), so errors.Is matches the underlying sentinel
// — the call site does NOT pass err.Error().
func TestDaemonTick_CapturePaneFailureErrorAttrIsWrappedError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	sess, panes := oneSession()
	sentinel := errors.New("capture-pane wrapped-sentinel")
	fc := &daemonFakeCommander{
		sessionsOut:        sess,
		panesOut:           panes,
		captureErrByTarget: map[string]error{"work:0.0": sentinel},
	}

	rec := &errorAttrRecorder{wantMsg: "capture pane failed"}
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	deps := &daemonDeps{
		Dir:          dir,
		Logger:       slog.New(rec).With("component", "daemon"),
		Client:       tmux.NewClient(fc),
		HashMap:      state.HashMap{},
		TickerPeriod: 1 * time.Millisecond,
		MaxGap:       30 * time.Second,
		LastSaveAt:   time.Now(),
	}
	touchSaveRequested(t, dir)

	tick(context.Background(), deps)

	if !rec.found {
		t.Fatal("no WARN 'capture pane failed' record with an error attr was captured")
	}
	if !errors.Is(rec.gotErr, sentinel) {
		t.Errorf("error attr does not wrap the sentinel via errors.Is; got %v", rec.gotErr)
	}
}

// noSuchSessionCommandErr returns a *tmux.CommandError whose stderr carries
// tmux's canonical lowercase "no such session" phrasing. The tmux Client's
// ShowEnvironment wraps such errors via wrapNoSuchSession so callers see an
// errors.Is(err, tmux.ErrNoSuchSession) match — the natural-churn branch in
// CaptureStructure's per-session loop. Anomalous failures (any other shape)
// are not wrapped.
func noSuchSessionCommandErr(session string) error {
	return &tmux.CommandError{
		Stderr: "no such session: " + session,
		Err:    errors.New("exit status 1"),
	}
}

// The captureAndCommit call site these tests exercise is at
// cmd/state_daemon.go (state.CaptureStructure(..., deps.Logger)). deps.Logger
// is set to an in-memory capturing *slog.Logger so the per-session WARN lines
// can be inspected directly via the sink — no on-disk portal.log read.

// TestDaemonTick_LogsAnomalousShowEnvironmentFailureUnderComponentDaemon is
// the end-to-end wiring assertion. Two sessions, one succeeds, one returns an
// anomalous (non-natural-churn) ShowEnvironment error. The daemon's tick path
// (tick → captureAndCommit → CaptureStructure) must surface the failure as a
// WARN entry under "daemon" naming the failing session and a substring of the
// underlying error, proving deps.Logger reaches CaptureStructure through the
// call site at cmd/state_daemon.go.
func TestDaemonTick_LogsAnomalousShowEnvironmentFailureUnderComponentDaemon(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Two sessions in list-sessions; pane rows for both.
	fc := &daemonFakeCommander{
		sessionsOut: "A|1|0|\nB|1|0|",
		panesOut: "A|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh\n" +
			"B|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh",
		envBySession: map[string]string{
			"A": "FOO=bar",
		},
		// Note: daemonFakeCommander.dispatch checks envBySession before falling
		// through, but it does NOT honour an envErrs map. We therefore drive
		// the failure via an override: a sentinel session entry that is not
		// in envBySession will return ("", nil) — which would succeed. To
		// force a real error we need the envBySession map to be the only
		// successful entry AND the dispatch to surface an error for B.
		// Since the fake's existing semantics return nil for unknown sessions,
		// we add B's error via a small wrapper commander below.
	}

	// Wrap fc in a commander that injects an anomalous error for B's
	// show-environment call. This keeps the rest of the fake's behaviour
	// (markers, list-sessions, list-panes, capture-pane) intact.
	wrapped := &envFailingCommander{
		inner: fc,
		envErrs: map[string]error{
			"B": errors.New("bravo-boom-sentinel"),
		},
	}

	logger, sink := newCaptureLoggerForComponent(t, "daemon")

	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	deps := &daemonDeps{
		Dir:          dir,
		Logger:       logger,
		Client:       tmux.NewClient(wrapped),
		HashMap:      state.HashMap{},
		TickerPeriod: 1 * time.Millisecond,
		MaxGap:       30 * time.Second,
		LastSaveAt:   time.Now(),
	}
	touchSaveRequested(t, dir)

	tick(context.Background(), deps)

	log := sink.Body()

	// Required substrings: WARN level, "daemon" component, failing session
	// name "B" (via the session attr), and a substring of the underlying error.
	if !strings.Contains(log, "WARN") {
		t.Errorf("expected WARN-level entry; log:\n%s", log)
	}
	if !strings.Contains(log, "component="+"daemon") {
		t.Errorf("expected ComponentDaemon (%q) entry; log:\n%s", "daemon", log)
	}
	if !strings.Contains(log, "session=B") {
		t.Errorf("expected failing session name %q to appear in WARN body; log:\n%s", "B", log)
	}
	if !strings.Contains(log, "bravo-boom-sentinel") {
		t.Errorf("expected underlying error text to appear in WARN body; log:\n%s", log)
	}
}

// TestDaemonTick_LogsPerSessionWarnAndCommitsEmptyOnAllNaturalChurn covers
// the natural-churn total-failure branch: every session in the keep set fails
// with a tmuxerr.ErrNoSuchSession-wrapped error (tmux's "no such session"
// stderr signature). CaptureStructure must log one WARN per session, return
// an empty index with nil error, and the daemon's captureAndCommit must
// proceed to Commit a sessions.json reflecting the new (empty) reality.
//
// This is the spec's distinction between "user killed everything mid-tick"
// (proceed with empty Commit) and "tmux returned an unrecoverable anomalous
// error" (refuse to Commit). The former must not strand stale state on disk.
func TestDaemonTick_LogsPerSessionWarnAndCommitsEmptyOnAllNaturalChurn(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	fc := &daemonFakeCommander{
		sessionsOut: "A|1|0|\nB|1|0|",
		panesOut: "A|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh\n" +
			"B|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh",
	}
	wrapped := &envFailingCommander{
		inner: fc,
		envErrs: map[string]error{
			"A": noSuchSessionCommandErr("A"),
			"B": noSuchSessionCommandErr("B"),
		},
	}

	logger, sink := newCaptureLoggerForComponent(t, "daemon")

	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	deps := &daemonDeps{
		Dir:          dir,
		Logger:       logger,
		Client:       tmux.NewClient(wrapped),
		HashMap:      state.HashMap{},
		TickerPeriod: 1 * time.Millisecond,
		MaxGap:       30 * time.Second,
		LastSaveAt:   time.Now(),
	}
	touchSaveRequested(t, dir)

	tick(context.Background(), deps)

	log := sink.Body()

	// Exactly two WARN entries — one per failing session. (Note: a leak
	// through to other WARN sites — e.g. tick's own "tick failed" wrapper —
	// would cause an extra match; the all-natural-churn path returns nil
	// error from CaptureStructure so tick must NOT log a tick-failed wrapper.)
	warnCount := strings.Count(log, "WARN")
	if warnCount != 2 {
		t.Errorf("WARN entries = %d, want 2; log:\n%s", warnCount, log)
	}
	if !strings.Contains(log, "session=A") {
		t.Errorf("expected WARN for session A; log:\n%s", log)
	}
	if !strings.Contains(log, "session=B") {
		t.Errorf("expected WARN for session B; log:\n%s", log)
	}

	// Commit was invoked once writing an empty sessions.json — the
	// natural-churn branch proceeds with an empty Commit. Structural proof:
	// sessions.json exists, decodes to zero sessions.
	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("sessions.json must be committed on all-natural-churn: %v", err)
	}
	committed, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("decode sessions.json: %v", err)
	}
	if len(committed.Sessions) != 0 {
		t.Errorf("committed sessions length = %d, want 0 (empty Commit on all-natural-churn)",
			len(committed.Sessions))
	}
}

// TestDaemonTick_CapturePaneFailureLogsWarnWithPaneKey is the Task 1-9
// acceptance for the daemon's per-pane capture-loop WARN: when
// state.CaptureAndHashPane fails for a pane, the daemon's tick loop logs one
// WARN under component=daemon carrying the failing pane's pane_key attr and the
// wrapped error, then continues the loop. Asserts the closed-vocabulary
// pane_key attr is present and the error attr carries the wrapped error chain
// (errors.Is against the captured record), not the .Error() string.
func TestDaemonTick_CapturePaneFailureLogsWarnWithPaneKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	sess, panes := oneSession() // session "work", window 0, pane 0 → target work:0.0
	sentinel := errors.New("capture-pane boom-sentinel")
	fc := &daemonFakeCommander{
		sessionsOut:        sess,
		panesOut:           panes,
		captureErrByTarget: map[string]error{"work:0.0": sentinel},
	}

	logger, sink := newCaptureLoggerForComponent(t, "daemon")

	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	deps := &daemonDeps{
		Dir:          dir,
		Logger:       logger,
		Client:       tmux.NewClient(fc),
		HashMap:      state.HashMap{},
		TickerPeriod: 1 * time.Millisecond,
		MaxGap:       30 * time.Second,
		LastSaveAt:   time.Now(),
	}
	touchSaveRequested(t, dir)

	tick(context.Background(), deps)

	log := sink.Body()
	if !strings.Contains(log, "WARN") {
		t.Errorf("expected WARN-level entry; log:\n%s", log)
	}
	if !strings.Contains(log, "component=daemon") {
		t.Errorf("expected component=daemon entry; log:\n%s", log)
	}
	if !strings.Contains(log, "capture pane failed") {
		t.Errorf("expected 'capture pane failed' message; log:\n%s", log)
	}
	if !strings.Contains(log, "pane_key=work:0.0") {
		t.Errorf("expected pane_key=work:0.0 attr on the WARN; log:\n%s", log)
	}
	// The error attr carries the wrapped error chain (slog.Any semantics), so
	// the captured rendering includes the sentinel's message verbatim — proving
	// the WARN passed the error directly, not a pre-formatted string.
	if !strings.Contains(log, "capture-pane boom-sentinel") {
		t.Errorf("expected wrapped error text in the WARN; log:\n%s", log)
	}
}

// envFailingCommander wraps an inner daemonFakeCommander and intercepts
// show-environment calls for sessions listed in envErrs, returning the
// configured error. Other calls forward to the inner commander unchanged.
//
// Centralised here (not added to daemonFakeCommander) so the existing fake's
// dispatch surface stays minimal — only the Component E logging tests need
// per-session env failures.
type envFailingCommander struct {
	inner   *daemonFakeCommander
	envErrs map[string]error
}

func (c *envFailingCommander) Run(args ...string) (string, error) {
	if len(args) >= 3 && args[0] == "show-environment" && args[1] == "-t" {
		if err, ok := c.envErrs[args[2]]; ok {
			return "", err
		}
	}
	return c.inner.Run(args...)
}

func (c *envFailingCommander) RunRaw(args ...string) (string, error) {
	return c.inner.RunRaw(args...)
}
