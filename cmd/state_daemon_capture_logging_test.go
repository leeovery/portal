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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

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

// readPortalLog returns the on-disk portal.log body after flushing the
// supplied logger so subsequent assertions see every write. The
// captureAndCommit call site this exercises is at
// cmd/state_daemon.go:325 (state.CaptureStructure(..., deps.Logger)).
func readPortalLog(t *testing.T, logger *state.Logger, dir string) string {
	t.Helper()
	// Flush the logger before reading so buffered writes land on disk.
	// This is an intentional double-close — makeDeps' t.Cleanup will
	// also Close the logger at test teardown, but assertions need the
	// body now. The error from the second close is discarded by the
	// cleanup (and the first close here) so the redundancy is safe.
	_ = logger.Close()
	data, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		t.Fatalf("read portal.log: %v", err)
	}
	return string(data)
}

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
		sessionsOut: "A|1|0\nB|1|0",
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

	logger, err := state.OpenLogger(state.PortalLog(dir), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

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

	log := readPortalLog(t, logger, dir)

	// Required substrings: WARN level, "daemon" component, failing session
	// name "B", and a substring of the underlying error.
	if !strings.Contains(log, " | WARN | ") {
		t.Errorf("expected WARN-level entry; log:\n%s", log)
	}
	if !strings.Contains(log, "| "+state.ComponentDaemon+" |") {
		t.Errorf("expected ComponentDaemon (%q) entry; log:\n%s", state.ComponentDaemon, log)
	}
	if !strings.Contains(log, `"B"`) {
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
		sessionsOut: "A|1|0\nB|1|0",
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

	logger, err := state.OpenLogger(state.PortalLog(dir), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

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

	log := readPortalLog(t, logger, dir)

	// Exactly two WARN entries under ComponentDaemon — one per failing
	// session. (Note: a leak through to other WARN sites — e.g. tick's
	// own "tick:" wrapper — would cause an extra match; the all-natural-
	// churn path returns nil error from CaptureStructure so tick must NOT
	// log a "tick:" wrapper.)
	warnPrefix := "| WARN | " + state.ComponentDaemon + " |"
	warnCount := strings.Count(log, warnPrefix)
	if warnCount != 2 {
		t.Errorf("WARN entries under ComponentDaemon = %d, want 2; log:\n%s", warnCount, log)
	}
	if !strings.Contains(log, `"A"`) {
		t.Errorf("expected WARN for session A; log:\n%s", log)
	}
	if !strings.Contains(log, `"B"`) {
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
