package bootstrap_test

// Phase 5 integration tests exercise the eight-step bootstrap.Orchestrator
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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// skipIfNoTmux skips the test when tmux is not on PATH. Mirrors
// internal/restore/integration_test.go's helper of the same name.
func skipIfNoTmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available; skipping integration test")
	}
}

// markerProbeStub records whether @portal-restoring was set at the moment
// the wrapped step was invoked. Used by Test 1 to prove that @portal-restoring
// is observable on the live tmux server during steps 4-5 (EnsureSaver,
// Restore) and absent during step 7 (CleanStale).
type markerProbeStub struct {
	client    *tmux.Client
	wantValue string // "1" when marker should be present, "" when absent
	called    bool
	observed  string
	found     bool
}

// probe queries @portal-restoring on the live server and records the result.
func (m *markerProbeStub) probe() error {
	val, found, err := m.client.TryGetServerOption(state.RestoringMarkerName)
	if err != nil {
		return err
	}
	m.called = true
	m.observed = val
	m.found = found
	return nil
}

// EnsureSaver records the marker state and returns nil. Stubs the saver step.
func (m *markerProbeStub) EnsureSaver() error { return m.probe() }

// Restore records the marker state and reports (false, err). Stubs the
// restore step under the (corrupt, err) Restorer contract; this stub
// never simulates the corrupt-index path so corrupt is always false.
func (m *markerProbeStub) Restore() (bool, error) { return false, m.probe() }

// CleanStale records the marker state and returns nil. Stubs the clean step.
func (m *markerProbeStub) CleanStale() error { return m.probe() }

// noopSaver is a saver step that performs no work and reports success. Used
// where the saver step is incidental to the test scenario.
type noopSaver struct{}

// EnsureSaver always returns nil.
func (noopSaver) EnsureSaver() error { return nil }

// noopRestorer is a restore step that performs no work and reports success.
// Used by tests that exercise marker lifecycle but do not need real
// skeleton-restore behaviour.
type noopRestorer struct{}

// Restore always returns (false, nil) — happy path under the
// (corrupt, err) Restorer contract.
func (noopRestorer) Restore() (bool, error) { return false, nil }

// noopCleaner is a clean step that performs no work and reports success.
type noopCleaner struct{}

// CleanStale always returns nil.
func (noopCleaner) CleanStale() error { return nil }

// noopHooks is a hook registrar that performs no work and reports success.
// Used by Test 1 to keep that scenario focused on marker lifecycle without
// also asserting on Portal's hook table.
type noopHooks struct{}

// RegisterPortalHooks always returns nil.
func (noopHooks) RegisterPortalHooks() error { return nil }

// restoreOrchestratorAdapter wraps a *restore.Orchestrator so its
// Restore() method satisfies bootstrap.Restorer. The bootstrap
// orchestrator owns the @portal-restoring lifecycle separately (steps 3
// and 6) so the inner Restore() must not bundle marker management.
type restoreOrchestratorAdapter struct {
	inner *restore.Orchestrator
}

// Restore delegates to the wrapped restore.Orchestrator's Restore method,
// returning the (corrupt, err) tuple verbatim under the bootstrap.Restorer
// contract.
func (a *restoreOrchestratorAdapter) Restore() (bool, error) { return a.inner.Restore() }

// TestPhase5_RestoringMarkerSuppressesCaptures proves that the
// @portal-restoring server option is set on the live tmux server before step
// 4 (EnsureSaver) runs and stays set through step 5 (Restore), then is
// cleared before step 7 (CleanStale) runs. This is the integration-level
// guarantee the save daemon depends on: when its tick observes
// @portal-restoring="1" it must skip its capture cycle. Testing the daemon's
// suppression directly would require standing up the full daemon process;
// instead this test verifies the contract from the producer side — the
// orchestrator does set/clear the marker around the suppression window.
func TestPhase5_RestoringMarkerSuppressesCaptures(t *testing.T) {
	skipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-p5-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	saverProbe := &markerProbeStub{client: client, wantValue: "1"}
	restoreProbe := &markerProbeStub{client: client, wantValue: "1"}
	cleanProbe := &markerProbeStub{client: client, wantValue: ""}

	o := &bootstrap.Orchestrator{
		Server:    client,
		Hooks:     noopHooks{},
		Restoring: &bootstrapadapter.RestoringMarker{Client: client},
		Saver:     saverProbe,
		Restore:   restoreProbe,
		Clean:     cleanProbe,
	}

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Saver step: marker MUST be present and equal to "1".
	if !saverProbe.called {
		t.Error("EnsureSaver probe was not invoked")
	}
	if !saverProbe.found || saverProbe.observed != "1" {
		t.Errorf("EnsureSaver: @portal-restoring found=%v value=%q; want found=true value=%q",
			saverProbe.found, saverProbe.observed, "1")
	}

	// Restore step: marker MUST still be present.
	if !restoreProbe.called {
		t.Error("Restore probe was not invoked")
	}
	if !restoreProbe.found || restoreProbe.observed != "1" {
		t.Errorf("Restore: @portal-restoring found=%v value=%q; want found=true value=%q",
			restoreProbe.found, restoreProbe.observed, "1")
	}

	// CleanStale step: marker MUST be cleared by step 6 before step 7 runs.
	if !cleanProbe.called {
		t.Error("CleanStale probe was not invoked")
	}
	if cleanProbe.found {
		t.Errorf("CleanStale: @portal-restoring still present (value=%q); want absent",
			cleanProbe.observed)
	}

	// Final state: marker MUST be unset after Run returns.
	val, found, err := client.TryGetServerOption(state.RestoringMarkerName)
	if err != nil {
		t.Fatalf("TryGetServerOption final: %v", err)
	}
	if found {
		t.Errorf("@portal-restoring still set after Run; value=%q", val)
	}
}

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
	skipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-p5-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	ts.Run(t, "new-session", "-d", "-s", "alpha")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	o := &bootstrap.Orchestrator{
		Server:    client,
		Hooks:     &bootstrapadapter.HookRegistrar{Client: client},
		Restoring: &bootstrapadapter.RestoringMarker{Client: client},
		Saver:     noopSaver{},
		Restore:   noopRestorer{},
		Clean:     noopCleaner{},
	}

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
// Wiring: real RestoringMarker (Set/Clear), real restore.Orchestrator wrapped
// to call the bare Restore() — the bootstrap orchestrator owns the
// @portal-restoring marker lifecycle separately — no-op Saver/Hooks/Clean.
//
// Overlap note: TestPhase3Integration_SaveRestoreRoundTrip already exercises
// the capture→persist→kill→restore round-trip. The unique coverage here is
// that the bootstrap.Orchestrator (not just restore.Orchestrator) wires the
// Restore step correctly — i.e. step 5 actually creates the missing session
// when invoked through the eight-step sequence.
func TestPhase5_RestoreCreatesMissingSession(t *testing.T) {
	skipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-p5-")
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

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

	logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	restoreInner := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}

	o := &bootstrap.Orchestrator{
		Server:    client,
		Hooks:     noopHooks{},
		Restoring: &bootstrapadapter.RestoringMarker{Client: client},
		Saver:     noopSaver{},
		Restore:   &restoreOrchestratorAdapter{inner: restoreInner},
		Clean:     noopCleaner{},
	}

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Post-condition: missing-foo must be live, created by step 5.
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "missing-foo") {
		t.Errorf("expected missing-foo in list-sessions; got %q", out)
	}

	// @portal-restoring must be cleared by step 6.
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
