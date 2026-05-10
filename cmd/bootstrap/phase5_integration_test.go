package bootstrap_test

// Phase 5 integration tests exercise the ten-step bootstrap.Orchestrator
// against a real tmux server using the same isolated-socket pattern as
// internal/restore/integration_test.go (Phase 3, task 3-13). Each test runs an
// isolated tmux instance via `tmux -S <abs-socket-path>` rooted in a per-test
// scratch dir, so the user's tmux is never touched and concurrent test runs
// cannot collide.
//
// Tests gate on `tmux` being on PATH and skip cleanly otherwise. Heavy
// end-to-end paths (PTY-spawned attach, ANSI byte-level scrollback compare,
// real daemon spawning) are intentionally NOT exercised here — they have
// dedicated unit tests at the handler level. The goal is meaningful coverage
// of the orchestration wiring (step ordering, marker visibility across steps,
// and skeleton creation from sessions.json) without pretending to do more.
//
// The `tmuxSocket` harness lives in internal/tmuxtest (shared with the
// internal/restore Phase 3 integration suite); see internal/tmuxtest/socket.go.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestPhase5_RestoringMarkerSuppressesCaptures has been promoted into a
// non-vacuous integration test in phase5_marker_suppression_integration_test.go
// (gated by `//go:build integration`). The original assertion-of-absence
// shape was vacuously true — it stubbed Saver/Restore so no structural
// events ever fired during the @portal-restoring window. The replacement
// installs a real probe hook AND a real RestoreAdapter so at least one
// session-created event MUST fire during the window for the test to even
// be meaningful, then asserts both the probe-fire (non-vacuity guard) and
// the saved_at-unchanged invariant (suppression contract).

// TestPhase5_OrchestratorEndToEndSmoke runs the bootstrap orchestrator with
// real wirings for Server, Hooks, and Restoring against a live tmux server,
// and stubs Saver/Restore/Clean to keep the test bounded. The smoke is:
//
//   - Pre-existing user session ("alpha") survives Run.
//   - @portal-restoring is unset post-Run.
//   - Portal's full hook table is registered (9 entries: 7 save-trigger,
//     2 hydration-trigger). The migrate-rename hook is deferred to v2.
//
// This is not an end-to-end save/restore round-trip — that is covered by
// TestPhase3Integration_SaveRestoreRoundTrip in internal/restore. The unique
// coverage here is the orchestrator's wiring of all three real steps in
// spec order without exploding on a real tmux server.
func TestPhase5_OrchestratorEndToEndSmoke(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-p5-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	ts.Run(t, "new-session", "-d", "-s", "alpha")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Hooks: &bootstrapadapter.HookRegistrar{Client: client},
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// alpha must still be alive.
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "alpha") {
		t.Errorf("expected alpha in list-sessions; got %q", out)
	}

	// @portal-restoring must be unset.
	if val, found, err := client.TryGetServerOption(state.RestoringMarkerName); err != nil {
		t.Fatalf("TryGetServerOption: %v", err)
	} else if found {
		t.Errorf("@portal-restoring still set; value=%q", val)
	}

	// Portal hooks must be registered for every event in the spec table.
	//
	// We query each expected (event, substring) pair via `show-hooks -g <event>`
	// rather than the unscoped `show-hooks -g` because tmux 3.6 omits a couple
	// of events (notably pane-focus-out and window-layout-changed) from the
	// unscoped dump even when their hooks are present and fire correctly.
	// Targeted queries are reliable on every supported tmux version.
	type hookExpect struct {
		event     string
		substring string
	}
	wantHooks := []hookExpect{
		// 7 save-trigger events.
		{"session-created", "portal state notify"},
		{"session-closed", "portal state notify"},
		{"session-renamed", "portal state notify"},
		{"window-linked", "portal state notify"},
		{"window-unlinked", "portal state notify"},
		{"window-layout-changed", "portal state notify"},
		{"pane-focus-out", "portal state notify"},
		// 2 hydration-trigger events.
		{"client-attached", "portal state signal-hydrate"},
		{"client-session-changed", "portal state signal-hydrate"},
		// The migrate-rename hook is deferred to v2 (see hooks_register.go);
		// session-renamed only carries the notify entry above.
	}
	for _, want := range wantHooks {
		out, err := ts.TryRun("show-hooks", "-g", want.event)
		if err != nil {
			t.Errorf("show-hooks -g %s: %v\n%s", want.event, err, out)
			continue
		}
		if !strings.Contains(out, want.substring) {
			t.Errorf("hook on %s missing %q; got %q", want.event, want.substring, out)
		}
	}
}

// TestPhase5_RestoreCreatesMissingSession proves that when sessions.json
// contains a session NAME not currently live, the bootstrap orchestrator's
// Restore step skeleton-creates that session by the time Run returns. This is
// the spec's central acceptance criterion for `portal attach NAME` against a
// sessions.json-only name.
//
// Wiring: real RestoringMarker (Set/Clear), real restore.Orchestrator
// wrapped via bootstrapadapter.RestoreAdapter so its Restore() satisfies
// bootstrap.Restorer — the bootstrap orchestrator owns the
// @portal-restoring marker lifecycle separately — no-op Saver/Hooks/Sweeper/Clean.
//
// Overlap note: TestPhase3Integration_SaveRestoreRoundTrip already exercises
// the capture→persist→kill→restore round-trip. The unique coverage here is
// that the bootstrap.Orchestrator (not just restore.Orchestrator) wires the
// Restore step correctly — i.e. step 5 actually creates the missing session
// when invoked through the ten-step sequence.
func TestPhase5_RestoreCreatesMissingSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-p5-")
	stateDir := newIntegrationStateDir(t)

	// Hand-craft a sessions.json containing one session named "missing-foo"
	// with one window and one pane. The scrollback file path is recorded
	// inside the encoded hydrate command but is not read by Restore itself —
	// only by the in-pane `portal state hydrate` helper, which is
	// out-of-scope here.
	idx := state.Index{
		Sessions: []state.Session{{
			Name: "missing-foo",
			Windows: []state.Window{{
				Index:  0,
				Layout: "tiled",
				Active: true,
				Panes: []state.Pane{{
					Index:          0,
					Active:         true,
					ScrollbackFile: "scrollback/missing-foo-w0-p0.bin",
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

	// Pre-condition: missing-foo must NOT be live yet.
	if _, err := ts.TryRun("has-session", "-t", "missing-foo"); err == nil {
		t.Fatal("missing-foo unexpectedly live before Run")
	}

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

	// Post-condition: missing-foo must be live, created by step 5.
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "missing-foo") {
		t.Errorf("expected missing-foo in list-sessions; got %q", out)
	}

	// @portal-restoring must be cleared by step 7.
	if val, found, err := client.TryGetServerOption(state.RestoringMarkerName); err != nil {
		t.Fatalf("TryGetServerOption: %v", err)
	} else if found {
		t.Errorf("@portal-restoring still set after Run; value=%q", val)
	}

	// Skeleton marker for missing-foo's single pane must be present —
	// confirms ApplySkeletonMarkers ran as part of restore.
	wantMarker := "@portal-skeleton-" + state.SanitizePaneKey("missing-foo", 0, 0)
	if val, found, err := client.TryGetServerOption(wantMarker); err != nil {
		t.Fatalf("TryGetServerOption %s: %v", wantMarker, err)
	} else if !found || val == "" {
		t.Errorf("expected skeleton marker %q to be set; found=%v value=%q", wantMarker, found, val)
	}
}

// TestPhase5_FIFOSweeperRemovesOrphansAfterRestore proves that step 9
// (FIFOSweeper) removes orphan hydrate-*.fifo files whose paneKey is not
// represented by a live `@portal-skeleton-*` marker, while preserving
// FIFOs whose paneKey corresponds to a marker freshly set by step 5
// (Restore). This is the integration-level guarantee that the sweep
// observes the post-Restore marker set, not the pre-Restore one — i.e.
// step 9 runs strictly after step 5.
//
// Wiring: real RestoringMarker, real restore.Orchestrator wrapped in
// bootstrapadapter.RestoreAdapter (so Restore actually creates the
// session and sets the skeleton marker), real bootstrapadapter.FIFOSweeper.
// Saver/Hooks/Clean are no-ops — the test is scoped to the Restore →
// Sweep handoff.
func TestPhase5_FIFOSweeperRemovesOrphansAfterRestore(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-p5-")
	stateDir := newIntegrationStateDir(t)

	// sessions.json describing one session — Restore will skeleton-create
	// it and set the @portal-skeleton-<paneKey> marker for its single pane.
	idx := state.Index{
		Sessions: []state.Session{{
			Name: "swept-foo",
			Windows: []state.Window{{
				Index:  0,
				Layout: "tiled",
				Active: true,
				Panes: []state.Pane{{
					Index:          0,
					Active:         true,
					ScrollbackFile: "scrollback/swept-foo-w0-p0.bin",
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

	// Pre-create two FIFOs in stateDir:
	//   - liveKey matches the paneKey Restore will mark live.
	//   - orphanKey is not represented in sessions.json, so no skeleton
	//     marker will be set for it; step 9 must remove it.
	liveKey := state.SanitizePaneKey("swept-foo", 0, 0)
	orphanKey := state.SanitizePaneKey("ghost-bar", 0, 0)
	livePath := state.FIFOPath(stateDir, liveKey)
	orphanPath := state.FIFOPath(stateDir, orphanKey)
	if err := state.CreateFIFO(livePath); err != nil {
		t.Fatalf("create live FIFO: %v", err)
	}
	if err := state.CreateFIFO(orphanPath); err != nil {
		t.Fatalf("create orphan FIFO: %v", err)
	}

	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	logger := openTestLogger(t, stateDir)

	restoreInner := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: &bootstrapadapter.RestoreAdapter{Inner: restoreInner},
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Live FIFO MUST survive — its paneKey corresponds to the skeleton
	// marker step 5 set on the live tmux server.
	if _, err := os.Lstat(livePath); err != nil {
		t.Errorf("live FIFO removed (paneKey=%q): %v", liveKey, err)
	}

	// Orphan FIFO MUST be removed — no skeleton marker exists for its
	// paneKey, so step 9 swept it.
	if _, err := os.Lstat(orphanPath); !os.IsNotExist(err) {
		t.Errorf("orphan FIFO not removed (paneKey=%q): lstat err = %v", orphanKey, err)
	}
}
