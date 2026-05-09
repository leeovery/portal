//go:build integration

// Phase 2 task 2-7 — scrollback-save resumption end-to-end regression.
//
// Fix Component A alone resolves the user-visible resurrection symptom.
// Fix Component B closes a quieter side effect: while a marker is live for
// a paneKey, the daemon's capture loop skips scrollback save for that pane
// (cmd/state_daemon.go:131-133). For panes whose markers leaked but whose
// underlying sessions are still alive (or were re-created with the same
// key), scrollback content is silently not being saved. The cleanup step
// closes this gap by unsetting markers whose paneKey is no longer
// represented by a live pane — which makes the daemon's next tick save
// scrollback for any subsequently-created pane at the same paneKey.
//
// This file holds the end-to-end regression guard for that scrollback-save
// resumption: it drives a real tmux server through seed-marker → bootstrap
// (which invokes CleanStaleMarkers) → daemon-equivalent capture-and-commit
// and asserts the scrollback file lands on disk only when the cleanup
// step has actually unset the leaked marker. A negative-control sub-test
// swaps in bootstrap.NoOpMarkerCleaner{} to prove the assertion would fail
// if step 7 were disabled — closing the regression-guard contract from
// spec § Acceptance Criteria #8.
//
// Build & run:
//   go test -tags=integration ./cmd/bootstrap/...
//
// Tests in this file are NOT included in the default `go test ./...` run
// because the `//go:build integration` tag gates them off. They also call
// `testing.Short()` so `go test -short -tags=integration ./...` skips them
// — useful for quick-check CI lanes.

package bootstrap_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// TestScrollbackResumption_DaemonTickSavesScrollbackAfterCleanup is the
// primary positive: a leaked marker for a paneKey whose pane has been
// killed is unset by the bootstrap cleanup step, and once a fresh pane
// appears at the same paneKey the next daemon-equivalent tick saves its
// scrollback (the skip-save guard at cmd/state_daemon.go:131-133 no
// longer applies).
//
// Spec § Acceptance Criteria #8 — scrollback-save resumption.
func TestScrollbackResumption_DaemonTickSavesScrollbackAfterCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-sbres-")
	client := ts.Client()

	// _seed keeps the live-pane set non-empty when we kill the foo
	// session below — required so the cleanup step's mass-unset hazard
	// guard does not trip and skip the unset pass entirely. The
	// underscore prefix excludes _seed from CaptureStructure (matches
	// the daemon's _portal-saver discipline).
	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)
	tmuxtest.ApplyBaseIndices(t, ts, 0, 0)

	// Stand up the foo session whose paneKey will own the leaked
	// marker. sleep infinity keeps the pane alive without contaminating
	// scrollback.
	ts.Run(t, "new-session", "-d", "-s", "foo", "sleep", "infinity")
	ts.WaitForSession(t, "foo", 2*time.Second)

	paneKey := state.SanitizePaneKey("foo", 0, 0)
	markerName := state.SkeletonMarkerPrefix + paneKey

	// Kill the foo session BEFORE seeding the marker so the live-pane
	// set observed by cleanup excludes paneKey. This is the
	// "leaked-but-pane-not-currently-live" precondition the cleanup
	// algorithm operates on. _seed remains alive so the live set is
	// non-empty (mass-unset hazard guard does not trip).
	ts.Run(t, "kill-session", "-t", "foo")

	// Seed the leaked marker. Using SetServerOption directly mirrors
	// the production set path (state.SetSkeletonMarker) without
	// importing it, keeping the test free of any coupling to the
	// marker-set code path the spec § Out of Scope forbids modifying.
	if err := client.SetServerOption(markerName, "1"); err != nil {
		t.Fatalf("SetServerOption seed marker: %v", err)
	}

	// Run the bootstrap orchestrator with the production
	// StaleMarkerCleaner adapter wired; every other step is stubbed to
	// a NoOp so a regression in this test's failure pinpoints the
	// cleanup step rather than incidental orchestrator wiring.
	logger := openTestLogger(t, stateDir)
	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		StaleMarkers: &bootstrapadapter.StaleMarkerCleaner{
			Client: client,
			Logger: logger,
		},
		Logger: logger,
	})
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// Marker for the no-longer-live paneKey must now be absent.
	_, found, err := client.TryGetServerOption(markerName)
	if err != nil {
		t.Fatalf("TryGetServerOption %s: %v", markerName, err)
	}
	if found {
		t.Fatalf("marker %s present after cleanup; want absent", markerName)
	}

	// Re-create the foo session at the same name so paneKey "foo__0.0"
	// once again resolves to a live pane. This is the spec's "pane
	// re-created" branch from § Why This Step Is Needed: the marker
	// leaked, cleanup unset it, and the new pane at the same paneKey
	// must now have its scrollback saved by the next daemon tick.
	ts.Run(t, "new-session", "-d", "-s", "foo", "sleep", "infinity")
	ts.WaitForSession(t, "foo", 2*time.Second)

	// Drive one daemon-equivalent tick — see runDaemonTick for the
	// exact sequence (mirrors cmd/state_daemon.go captureAndCommit).
	runDaemonTick(t, client, stateDir)

	// Scrollback file for the re-created pane MUST exist and be
	// non-empty. capture-pane on a sleeping pane returns at least the
	// blank visible buffer, so a present-but-zero-byte file would still
	// indicate a regression: the daemon would have written empty bytes
	// and dedup-cached a hash of zero, masking future captures. We
	// assert non-empty to guard both shapes.
	scrollbackPath := state.ScrollbackFile(stateDir, paneKey)
	info, err := os.Stat(scrollbackPath)
	if err != nil {
		t.Fatalf("scrollback file %s missing after daemon tick "+
			"(spec AC #8 regression — cleanup step did not "+
			"unblock scrollback save): %v", scrollbackPath, err)
	}
	if info.Size() == 0 {
		t.Fatalf("scrollback file %s is empty after daemon tick; "+
			"want non-empty (capture-pane should produce at least "+
			"one byte for a live pane)", scrollbackPath)
	}
}

// TestScrollbackResumption_WithoutCleanupScrollbackNotSaved is the
// negative-control / regression-guard variant. Same setup as the primary
// positive but the orchestrator is wired with bootstrap.NoOpMarkerCleaner{}
// instead of the production StaleMarkerCleaner. The leaked marker
// therefore survives bootstrap, the daemon's skip-save guard kicks in for
// the (re-created) pane at the same paneKey, and the scrollback file is
// NEVER written.
//
// This sub-test is the spec § Acceptance Criteria #8 regression guard:
// if a future change disables step 7, the primary positive above would
// silently turn into this negative control's outcome, but neither the
// resurrection symptom nor any user-facing surface would alert. The
// negative control fails loudly in that scenario.
func TestScrollbackResumption_WithoutCleanupScrollbackNotSaved(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-sbres-noop-")
	client := ts.Client()

	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)
	tmuxtest.ApplyBaseIndices(t, ts, 0, 0)

	ts.Run(t, "new-session", "-d", "-s", "foo", "sleep", "infinity")
	ts.WaitForSession(t, "foo", 2*time.Second)

	paneKey := state.SanitizePaneKey("foo", 0, 0)
	markerName := state.SkeletonMarkerPrefix + paneKey

	ts.Run(t, "kill-session", "-t", "foo")

	if err := client.SetServerOption(markerName, "1"); err != nil {
		t.Fatalf("SetServerOption seed marker: %v", err)
	}

	logger := openTestLogger(t, stateDir)

	// The only difference from the primary positive: NoOpMarkerCleaner
	// in StaleMarkers (the default). Step 7 is effectively disabled. The
	// whole point of this sub-test is to prove the primary positive's
	// assertions would fail under this configuration — i.e. the
	// regression guard is sharp.
	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Logger: logger,
	})
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// Marker MUST still be present — NoOpMarkerCleaner.CleanStaleMarkers
	// is a no-op, so step 7 leaves the seeded marker untouched.
	_, found, err := client.TryGetServerOption(markerName)
	if err != nil {
		t.Fatalf("TryGetServerOption %s: %v", markerName, err)
	}
	if !found {
		t.Fatalf("marker %s absent after no-op cleanup; want present "+
			"(regression-guard contract requires the marker to "+
			"survive when step 7 is disabled)", markerName)
	}

	// Re-create the pane at the same paneKey. With the marker still
	// set, the daemon's skip-save guard will fire on the next tick.
	ts.Run(t, "new-session", "-d", "-s", "foo", "sleep", "infinity")
	ts.WaitForSession(t, "foo", 2*time.Second)

	runDaemonTick(t, client, stateDir)

	// Scrollback file MUST NOT exist. The marker is still set so the
	// daemon's skip-save guard at cmd/state_daemon.go:131-133 fires
	// before WriteScrollbackIfChanged is reached.
	scrollbackPath := state.ScrollbackFile(stateDir, paneKey)
	if _, err := os.Stat(scrollbackPath); !os.IsNotExist(err) {
		t.Fatalf("scrollback file %s exists after no-op cleanup tick; "+
			"want absent (skip-save guard should suppress writes "+
			"while marker is set). stat err=%v", scrollbackPath, err)
	}
}

// TestScrollbackResumption_LiveHydrateInProgressMarkerPreserved exercises
// the cleanup step's selectivity end-to-end: a stale marker (no live pane
// at its paneKey) is unset, while a "hydrate-in-progress" marker (a live
// pane at its paneKey) is preserved. The scrollback save outcome reflects
// the marker outcome — the unset-marker pane has its scrollback saved on
// the next tick, the preserved-marker pane has its scrollback skipped.
//
// Spec § Behaviour Against Partial Restore Failures and § Stale-marker
// cleanup — bootstrap integration: cleanup operates only on markers
// whose corresponding paneKey is absent from the live-pane set.
func TestScrollbackResumption_LiveHydrateInProgressMarkerPreserved(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-sbres-mix-")
	client := ts.Client()

	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)
	tmuxtest.ApplyBaseIndices(t, ts, 0, 0)

	// Two real sessions: stalefoo will be killed (its marker becomes
	// stale); livebar stays alive (its marker is the legitimate
	// hydrate-in-progress case the cleanup must preserve).
	ts.Run(t, "new-session", "-d", "-s", "stalefoo", "sleep", "infinity")
	ts.WaitForSession(t, "stalefoo", 2*time.Second)
	ts.Run(t, "new-session", "-d", "-s", "livebar", "sleep", "infinity")
	ts.WaitForSession(t, "livebar", 2*time.Second)

	stalePaneKey := state.SanitizePaneKey("stalefoo", 0, 0)
	livePaneKey := state.SanitizePaneKey("livebar", 0, 0)
	staleMarker := state.SkeletonMarkerPrefix + stalePaneKey
	liveMarker := state.SkeletonMarkerPrefix + livePaneKey

	// Kill stalefoo so its paneKey is absent from the live-pane set
	// when cleanup runs. livebar stays alive — cleanup must observe it
	// and preserve its marker.
	ts.Run(t, "kill-session", "-t", "stalefoo")

	if err := client.SetServerOption(staleMarker, "1"); err != nil {
		t.Fatalf("SetServerOption stale marker: %v", err)
	}
	if err := client.SetServerOption(liveMarker, "1"); err != nil {
		t.Fatalf("SetServerOption live marker: %v", err)
	}

	logger := openTestLogger(t, stateDir)
	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		StaleMarkers: &bootstrapadapter.StaleMarkerCleaner{
			Client: client,
			Logger: logger,
		},
		Logger: logger,
	})
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// Stale marker must be absent.
	if _, found, err := client.TryGetServerOption(staleMarker); err != nil {
		t.Fatalf("TryGetServerOption %s: %v", staleMarker, err)
	} else if found {
		t.Errorf("stale marker %s present after cleanup; want absent",
			staleMarker)
	}

	// Live marker must be preserved.
	if _, found, err := client.TryGetServerOption(liveMarker); err != nil {
		t.Fatalf("TryGetServerOption %s: %v", liveMarker, err)
	} else if !found {
		t.Errorf("live marker %s absent after cleanup; want preserved "+
			"(hydrate-in-progress pane must keep its marker)",
			liveMarker)
	}

	// Re-create stalefoo at the same paneKey so the next tick has a
	// pane to save scrollback for.
	ts.Run(t, "new-session", "-d", "-s", "stalefoo", "sleep", "infinity")
	ts.WaitForSession(t, "stalefoo", 2*time.Second)

	runDaemonTick(t, client, stateDir)

	// stalefoo's scrollback MUST be saved — its marker was unset by
	// cleanup, so the daemon's skip-save guard does not fire.
	if info, err := os.Stat(state.ScrollbackFile(stateDir, stalePaneKey)); err != nil {
		t.Errorf("scrollback file for unset-marker pane %s missing "+
			"after daemon tick: %v", stalePaneKey, err)
	} else if info.Size() == 0 {
		t.Errorf("scrollback file for unset-marker pane %s is empty; "+
			"want non-empty", stalePaneKey)
	}

	// livebar's scrollback MUST NOT be saved — its marker is still
	// set, so the skip-save guard fires.
	if _, err := os.Stat(state.ScrollbackFile(stateDir, livePaneKey)); !os.IsNotExist(err) {
		t.Errorf("scrollback file for preserved-marker pane %s exists "+
			"after daemon tick; want absent (skip-save guard "+
			"should suppress writes while marker is set). "+
			"stat err=%v", livePaneKey, err)
	}
}

// runDaemonTick lives in daemon_tick_test_helpers_test.go — a shared
// helper consumed by the reboot round-trip too. The default option set
// (skip-guard ON, real CaptureAndHashPane bytes) is exactly what these
// scrollback-resumption tests need, so all call sites above pass no
// options.
