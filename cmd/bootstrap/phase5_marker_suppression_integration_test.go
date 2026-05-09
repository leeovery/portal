//go:build integration

// Phase 5 task 5-8 (review-cycle 1, task 12-3) — non-vacuous marker-suppression
// integration test.
//
// This test replaces the original TestPhase5_RestoringMarkerSuppressesCaptures
// from phase5_integration_test.go, which only proved that
// @portal-restoring was *set* during steps 4-5 — it did not prove that any
// structural event was actually fired during the marker window. That made the
// "no save advances during the window" assertion vacuously true: with stub
// Saver/Restore, no hooks ever fired, so suppression was untested.
//
// This expansion locks the suppression contract by:
//
//   1. Installing a probe `set-hook -ga session-created` that appends a line
//      to a tempfile every time a session is created on the isolated socket.
//   2. Wiring a real RestoreAdapter that creates a session inside the marker
//      window (via skeleton restoration), guaranteeing at least one
//      session-created event fires.
//   3. Asserting the probe tempfile is non-empty (NON-VACUITY GUARD: the test
//      MUST fail if the probe records zero events) AND that
//      sessions.json.saved_at is unchanged across the run (suppression: no
//      save committed during the window).
//
// Build & run:
//   go test -tags=integration ./cmd/bootstrap/...
//
// Why a separate file: the //go:build integration tag is scoped to this single
// test so the sibling tests in phase5_integration_test.go (which exercise
// non-suppression orchestration wiring) keep running on the default
// `go test ./...` lane.

package bootstrap_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestPhase5_RestoringMarkerSuppressesCaptures_NonVacuous is the
// non-vacuous expansion of the marker-suppression integration test.
//
// SCOPE — this test exercises Restore-side write discipline only. It
// proves that during the @portal-restoring window:
//
//	(a) at least one structural event (session-created) actually fires
//	    inside the window — a non-vacuity guard against a future refactor
//	    that turns the assertion into a pass-by-doing-nothing; AND
//	(b) sessions.json.saved_at does not advance — neither Restore itself
//	    nor any code path it touches commits a write to sessions.json
//	    while the marker is set.
//
// What this test deliberately does NOT cover: the daemon-tick suppression
// path proper (the spec's "Save-Side Architecture → Triggers &
// Serialization → Properties → Restoration guard" contract — daemon ticks
// observing @portal-restoring=1 at tick entry and skipping the capture).
// That contract is the daemon's own responsibility and is exercised by
// the daemon's unit tests; this test wires bootstrap.NoOpSaver so the
// daemon code path is not reachable from here. The two coverages are
// complementary, not redundant.
func TestPhase5_RestoringMarkerSuppressesCaptures_NonVacuous(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-p5-")
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Probe tempfile: the session-created hook appends one line per fire.
	// Lives outside stateDir so a stray sessions.json scan cannot mistake it
	// for portal-managed state.
	probeFile := filepath.Join(t.TempDir(), "session-created.events")

	// Seed sessions.json with a single saved session whose name is not yet
	// live on the isolated server. Restore step 5 will skeleton-create it
	// — that new-session call is what fires session-created during the
	// @portal-restoring window. The pre-run saved_at is captured below so
	// assertion (b) can compare timestamps after Run.
	preRunSavedAt := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	idx := state.Index{
		SavedAt: preRunSavedAt,
		Sessions: []state.Session{{
			Name: "probe-target",
			Windows: []state.Window{{
				Index:  0,
				Layout: "tiled",
				Active: true,
				Panes: []state.Pane{{
					Index:          0,
					Active:         true,
					ScrollbackFile: "scrollback/probe-target-w0-p0.bin",
				}},
			}},
		}},
	}
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(stateDir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// Install the probe: every session-created event appends the current
	// epoch-nanoseconds to probeFile. Using -ga so this entry coexists with
	// any hooks the orchestrator's Hooks step might register (this test
	// uses NoOpHooks, but the -ga discipline matches production wiring and
	// keeps the cleanup contract symmetric).
	probeCmd := "run-shell \"echo $(date +%s%N) >> " + probeFile + "\""
	if out, err := ts.TryRun("set-hook", "-ga", "session-created", probeCmd); err != nil {
		t.Fatalf("install probe hook: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		// Remove all global session-created entries the test added. The
		// kill-server in tmuxtest.Socket cleanup tears down all hooks
		// regardless, but explicit removal documents the cleanup contract
		// and isolates this test from any hook-replay pattern a future
		// harness change might introduce.
		_, _ = ts.TryRun("set-hook", "-gu", "session-created")
	})

	logger := openTestLogger(t, stateDir)

	restoreInner := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: &bootstrapadapter.RestoreAdapter{Inner: restoreInner},
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Sanity check: Restore step 5 must have actually created the saved
	// session. If skeleton restoration silently no-op'd, the probe will be
	// empty for an unrelated reason and the non-vacuity assertion below
	// would mask the real failure. Surfacing this as a separate fatal
	// distinguishes "Restore broken" from "suppression broken."
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "probe-target") {
		t.Fatalf("Restore did not create probe-target; list-sessions=%q — non-vacuity guard cannot be evaluated", out)
	}

	// Assertion (a): NON-VACUITY GUARD. The probe tempfile MUST contain at
	// least one line — proving session-created fired during the marker
	// window (skeleton restoration's new-session call is the only path
	// that creates a session inside Run).
	probeBytes, err := os.ReadFile(probeFile)
	if err != nil {
		// Probe never fired: probeFile may not exist. Treat as zero events.
		if !os.IsNotExist(err) {
			t.Fatalf("read probe file %s: %v", probeFile, err)
		}
		probeBytes = nil
	}
	probeLines := countNonEmptyLines(string(probeBytes))
	if probeLines == 0 {
		t.Fatal("non-vacuity guard failed: probe recorded zero session-created events during the @portal-restoring window — the suppression assertion below would be vacuously true")
	}

	// Assertion (b): SUPPRESSION INVARIANT. sessions.json.saved_at must
	// equal the pre-run value. With NoOpSaver wired, no save can advance
	// the timestamp through the orchestrator's own steps; any change
	// indicates an unintended write path during Restore.
	postIdx, skip, err := state.ReadIndex(stateDir)
	if err != nil {
		t.Fatalf("ReadIndex post-Run: %v", err)
	}
	if skip {
		t.Fatal("ReadIndex post-Run reported skip=true; sessions.json was unexpectedly removed during Run")
	}
	if !postIdx.SavedAt.Equal(preRunSavedAt) {
		t.Errorf("sessions.json.saved_at advanced during the marker window: pre=%v post=%v",
			preRunSavedAt, postIdx.SavedAt)
	}

	// @portal-restoring must be cleared by step 6 — same guarantee the
	// original test made; preserved here so this file fully replaces
	// the marker-lifecycle coverage.
	if val, found, err := client.TryGetServerOption(state.RestoringMarkerName); err != nil {
		t.Fatalf("TryGetServerOption final: %v", err)
	} else if found {
		t.Errorf("@portal-restoring still set after Run; value=%q", val)
	}
}

// countNonEmptyLines returns the count of lines in s that contain at least
// one non-whitespace character. Used by the probe-fire assertion so a probe
// file containing only a trailing newline (or whitespace) is correctly
// treated as zero events.
func countNonEmptyLines(s string) int {
	count := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
