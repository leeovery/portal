// Tests in this file mutate package-level state and MUST NOT use t.Parallel.
package cmd

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/tmux"
)

// hookCleanupDeps assembles the minimal daemonDeps the throttled hooks-cleanup
// gate (maybeRunHookCleanup) reads: the tmux Client (as the AllPaneLister), the
// HookStore, and the Logger. lastCleanup is left at its zero value for the
// caller to drive to a controlled instant.
func hookCleanupDeps(fc *daemonFakeCommander, store *hooks.Store, logger *slog.Logger) *daemonDeps {
	return &daemonDeps{
		Client:    tmux.NewClient(fc),
		HookStore: store,
		Logger:    logger,
	}
}

// discardDaemonLogger returns a no-op *slog.Logger for gate tests that assert on
// store/commander side effects rather than log output.
func discardDaemonLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMaybeRunHookCleanup_DoesNotRunBeforeInterval(t *testing.T) {
	seed := `{
  "stale:0.0": {"on-resume": "cmd-stale"}
}`
	store, _ := newTempHooksStore(t, seed)
	fc := &daemonFakeCommander{panesOut: "live:0.0"}
	deps := hookCleanupDeps(fc, store, discardDaemonLogger())

	anchor := time.Now() // NOT elapsed: time.Since(anchor) < hookCleanupInterval
	deps.lastCleanup = anchor

	maybeRunHookCleanup(deps)

	// (a) the stale entry survives — cleanup never ran.
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun["stale:0.0"]; !ok {
		t.Errorf("stale entry reaped before interval elapsed; hooks=%v", keysOf(postRun))
	}

	// (b) ListAllPanes was never invoked (no list-panes call recorded).
	if got := fc.callsContaining("list-panes"); len(got) != 0 {
		t.Errorf("list-panes invoked before interval elapsed: %v", got)
	}

	// (c) lastCleanup is untouched.
	if !deps.lastCleanup.Equal(anchor) {
		t.Errorf("lastCleanup advanced before interval elapsed: got %v, want %v", deps.lastCleanup, anchor)
	}
}

func TestMaybeRunHookCleanup_RunsAndResetsOnceIntervalElapsed(t *testing.T) {
	seed := `{
  "stale:0.0": {"on-resume": "cmd-stale"},
  "live:0.0": {"on-resume": "cmd-live"}
}`
	store, _ := newTempHooksStore(t, seed)
	fc := &daemonFakeCommander{panesOut: "live:0.0"}
	deps := hookCleanupDeps(fc, store, discardDaemonLogger())

	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second) // elapsed
	beforeCall := time.Now()

	maybeRunHookCleanup(deps)

	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun["stale:0.0"]; ok {
		t.Errorf("stale entry not reaped once interval elapsed; hooks=%v", keysOf(postRun))
	}
	if _, ok := postRun["live:0.0"]; !ok {
		t.Errorf("live entry wrongly reaped; hooks=%v", keysOf(postRun))
	}

	// lastCleanup advanced to ~now (on or after the pre-call instant).
	if deps.lastCleanup.Before(beforeCall) {
		t.Errorf("lastCleanup not advanced after cleanup: got %v, want >= %v", deps.lastCleanup, beforeCall)
	}
}

func TestMaybeRunHookCleanup_FiresAtIntervalBoundary(t *testing.T) {
	seed := `{
  "stale:0.0": {"on-resume": "cmd-stale"}
}`
	store, _ := newTempHooksStore(t, seed)
	fc := &daemonFakeCommander{panesOut: "live:0.0"}
	deps := hookCleanupDeps(fc, store, discardDaemonLogger())

	// Exactly the interval: time.Since(lastCleanup) is >= hookCleanupInterval
	// (some wall time always elapses before the check), proving the >= boundary
	// is inclusive.
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval)

	maybeRunHookCleanup(deps)

	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun["stale:0.0"]; ok {
		t.Errorf("cleanup did not fire at the interval boundary; hooks=%v", keysOf(postRun))
	}
}

func TestMaybeRunHookCleanup_LogsWarnAndSwallowsCleanupError(t *testing.T) {
	// Force a store.Load error (EISDIR) by pointing the store at a directory —
	// hooks.Store.Load returns an empty map (not an error) for malformed JSON,
	// so the directory trick is the reliable way to make CleanStale's Load fail.
	dir := t.TempDir()
	bogusPath := filepath.Join(dir, "hooks.json")
	if err := os.MkdirAll(bogusPath, 0o755); err != nil {
		t.Fatalf("mkdir bogus hooks path: %v", err)
	}
	store := hooks.NewStore(bogusPath)

	// Non-empty panesOut so the mass-deletion guard is NOT the branch taken —
	// Load fails first, exercising the returned-error path.
	fc := &daemonFakeCommander{panesOut: "live:0.0"}
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps := hookCleanupDeps(fc, store, logger)

	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second) // elapsed
	beforeCall := time.Now()

	// Must not panic or exit.
	maybeRunHookCleanup(deps)

	if got := sink.Body(); !strings.Contains(got, "hooks stale-cleanup failed") {
		t.Errorf("expected gate WARN 'hooks stale-cleanup failed' under daemon component; got:\n%s", got)
	}

	// lastCleanup still advances after a failing cleanup (retry next cadence,
	// not every tick).
	if deps.lastCleanup.Before(beforeCall) {
		t.Errorf("lastCleanup not advanced after failing cleanup: got %v, want >= %v", deps.lastCleanup, beforeCall)
	}
}

func TestMaybeRunHookCleanup_PinsSwallowListErrorTrue(t *testing.T) {
	// swallowListError=true is proven by seeding a ListAllPanes error: the
	// cleanup logs its own WARN, returns nil (no gate WARN), and reaps nothing.
	// onRemoved=nil is exercised (without panic) by the reap path in
	// TestMaybeRunHookCleanup_RunsAndResetsOnceIntervalElapsed; lister/store are
	// proven by the reap / no-reap behaviour across these tests.
	seed := `{
  "stale:0.0": {"on-resume": "cmd-stale"}
}`
	store, _ := newTempHooksStore(t, seed)
	fc := &daemonFakeCommander{panesErr: errors.New("tmux dead")}
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps := hookCleanupDeps(fc, store, logger)

	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second) // elapsed
	beforeCall := time.Now()

	// Must not panic or crash on the ListAllPanes error.
	maybeRunHookCleanup(deps)

	// swallowListError=true → no reap (cleanup short-circuits after the
	// list-panes WARN, before CleanStale).
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun["stale:0.0"]; !ok {
		t.Errorf("entry reaped despite ListAllPanes error; hooks=%v", keysOf(postRun))
	}

	// The error was swallowed inside runHookStaleCleanup (returned nil), so the
	// gate's own WARN must NOT fire.
	if got := sink.Body(); strings.Contains(got, "hooks stale-cleanup failed") {
		t.Errorf("gate WARN fired despite swallowed ListAllPanes error; got:\n%s", got)
	}

	// lastCleanup still advances.
	if deps.lastCleanup.Before(beforeCall) {
		t.Errorf("lastCleanup not advanced after swallowed list error: got %v, want >= %v", deps.lastCleanup, beforeCall)
	}
}

func TestMaybeRunHookCleanup_ReusesMassDeletionGuard(t *testing.T) {
	// Elapsed throttle + non-empty hooks + zero live panes → the shared helper's
	// mass-deletion hazard guard defers (never wipes) and logs its hazard WARN.
	seed := `{
  "a:0.0": {"on-resume": "cmd-a"},
  "b:0.0": {"on-resume": "cmd-b"}
}`
	store, _ := newTempHooksStore(t, seed)
	fc := &daemonFakeCommander{panesOut: ""} // zero live panes
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps := hookCleanupDeps(fc, store, logger)

	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second) // elapsed

	maybeRunHookCleanup(deps)

	// No entry reaped — the guard defers rather than wiping.
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if len(postRun) != 2 {
		t.Errorf("mass-deletion guard did not defer; post-run hooks=%v", keysOf(postRun))
	}

	// Hazard WARN logged by the shared helper (via deps.Logger).
	if got := sink.Body(); !strings.Contains(got, "mass-deletion hazard") {
		t.Errorf("expected mass-deletion hazard WARN; got:\n%s", got)
	}
}
