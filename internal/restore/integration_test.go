package restore_test

// Phase 3 integration tests exercise the end-to-end skeleton-restore pipeline
// against a real tmux server. Each test runs an isolated tmux instance via
// `tmux -S <unique-socket-path>` rooted in a per-test scratch dir, so the
// user's tmux is never touched and concurrent test runs cannot collide.
//
// Tests are gated on `tmux` being present on PATH; if not, they skip cleanly
// rather than fail. Heavy end-to-end paths (the 3-second hydrate timeout, the
// scrollback-file-missing branch) are intentionally skipped here — they have
// dedicated unit tests at the handler level and offer no incremental coverage
// at this scope. See the task brief for the rationale.
//
// The `tmuxSocket` harness (isolated-server scaffolding, socket commander,
// session-poll helper) lives in internal/tmuxtest and is shared with
// cmd/bootstrap's Phase 5 integration suite — see internal/tmuxtest/socket.go.

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// restoreWithMarker drives the bootstrap-owned @portal-restoring lifecycle
// inline for these integration tests: set the marker, run Restore(), unset
// the marker. Bootstrap.Orchestrator (cmd/bootstrap) owns this discipline in
// production; the helper exists only so the Phase-3 integration tests can
// exercise the same set/clear contract without re-implementing the marker
// API inside internal/restore. The clear is registered via defer so it runs
// on every exit path — including when Restore returns an error.
//
// Returns the err leg of Restore's (corrupt, err) tuple. The corrupt bool
// is irrelevant to the marker lifecycle and is dropped here; callers that
// need to assert on it call o.Restore() directly.
func restoreWithMarker(t *testing.T, client *tmux.Client, o *restore.Orchestrator) error {
	t.Helper()
	if err := client.SetServerOption(state.RestoringMarkerName, "1"); err != nil {
		return err
	}
	defer func() {
		if err := client.UnsetServerOption(state.RestoringMarkerName); err != nil {
			t.Logf("UnsetServerOption(%s): %v", state.RestoringMarkerName, err)
		}
	}()
	_, err := o.Restore()
	return err
}

// TestPhase3Integration_SaveRestoreRoundTrip is the default-mode smoke
// test: a single-session, single-window, single-pane round-trip without
// the binary-build / signal-hydrate machinery so it runs under plain
// `go test ./...`. It captures a live session, kills the server, restores
// from the persisted index against a fresh server, and asserts the saved
// session is recreated with its skeleton marker set. A second Restore()
// call must be a silent no-op (live-session skip) so the test guards
// against double-creates.
//
// The expanded planning-acceptance round-trip (multi-session × multi-
// window × multi-pane, ANSI scrollback byte fidelity, per-session env,
// zoom + active pane preservation, @portal-skeleton-* clearance after
// helper dump) lives in integration_full_test.go behind the `integration`
// build tag — see planning task 3-13 expanded by 12-4. Both tests are
// kept: the smoke runs in the default lane for fast feedback, the full
// round-trip guards the plan acceptance bullet under `-tags=integration`.
func TestPhase3Integration_SaveRestoreRoundTrip(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-")
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Stand up a single saved session with one window and one pane.
	ts.Run(t, "new-session", "-d", "-s", "alpha")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	client := ts.Client()

	// CAPTURE.
	idx, err := state.CaptureStructure(client, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	if len(idx.Sessions) != 1 || idx.Sessions[0].Name != "alpha" {
		t.Fatalf("expected one captured session named alpha, got %+v", idx.Sessions)
	}

	// PERSIST.
	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := writeFile(state.SessionsJSON(stateDir), data); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	// KILL the server so tmux loses the live session entirely.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("expected list-sessions to error after kill-server")
	}

	// EnsureServer mirrors what bootstrap's PersistentPreRunE does: a fresh
	// tmux server is brought up before any set-option call, which itself does
	// not auto-start a server. The orchestrator assumes a live server.
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	// RESTORE against the freshly-started server.
	logger := restoretest.OpenTestLogger(t, stateDir)

	o := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}
	if err := restoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	// VERIFY: alpha is alive again.
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected alpha in list-sessions; got %q", out)
	}

	// VERIFY: skeleton marker was set for alpha's single pane.
	wantMarker := "@portal-skeleton-" + state.SanitizePaneKey("alpha", 0, 0)
	markerOut := ts.Run(t, "show-options", "-sv", wantMarker)
	if strings.TrimSpace(markerOut) == "" {
		t.Errorf("expected marker %q to be set; got empty value", wantMarker)
	}

	// VERIFY: @portal-restoring was cleared after the marker block exited.
	if out, err := ts.TryRun("show-options", "-sv", state.RestoringMarkerName); err == nil && strings.TrimSpace(out) != "" {
		t.Errorf("%s should be unset after marker block; got %q", state.RestoringMarkerName, out)
	}

	// VERIFY: re-running Restore is a silent no-op (live-session skip).
	if _, err := o.Restore(); err != nil {
		t.Fatalf("second Restore: %v", err)
	}
	out2 := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	// Count occurrences of "alpha" — must remain exactly one.
	if got := strings.Count(out2, "alpha"); got != 1 {
		t.Errorf("expected exactly one alpha session after second Restore; got %d in %q", got, out2)
	}
}

// TestPhase3Integration_SweepOrphanFIFOs verifies the FIFO sweep against a
// fresh state directory: an orphan FIFO is removed, a live one is preserved.
// Sweep does not strictly require tmux but the test gate is kept for
// consistency with the rest of the integration suite.
func TestPhase3Integration_SweepOrphanFIFOs(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	liveKey := state.SanitizePaneKey("alpha", 0, 0)
	orphanKey := state.SanitizePaneKey("ghost", 0, 0)

	live := state.FIFOPath(stateDir, liveKey)
	orphan := state.FIFOPath(stateDir, orphanKey)
	if err := state.CreateFIFO(live); err != nil {
		t.Fatalf("CreateFIFO live: %v", err)
	}
	if err := state.CreateFIFO(orphan); err != nil {
		t.Fatalf("CreateFIFO orphan: %v", err)
	}

	liveSet := map[string]struct{}{liveKey: {}}
	if err := state.SweepOrphanFIFOs(stateDir, liveSet, nil); err != nil {
		t.Fatalf("SweepOrphanFIFOs: %v", err)
	}

	if !pathExists(live) {
		t.Errorf("live FIFO %s was unexpectedly removed", live)
	}
	if pathExists(orphan) {
		t.Errorf("orphan FIFO %s was not removed", orphan)
	}
}

// TestPhase3Integration_CorruptSessionsJSON wires real tmux up to an
// orchestrator pointed at a corrupt sessions.json. Restore() must return
// a wrapped state.ErrCorruptIndex, log a WARN, and not create any
// sessions on the live server. (The user-facing stderr warning emission
// moved to cmd/bootstrap_warnings.go in Phase 6 task 6-9.)
func TestPhase3Integration_CorruptSessionsJSON(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-")
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Garbage sessions.json drives ReadIndex's skip-with-error path.
	if err := writeFile(state.SessionsJSON(stateDir), []byte("{not json")); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	logger := restoretest.OpenTestLogger(t, stateDir)

	o := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}
	rwmErr := restoreWithMarker(t, client, o)
	if rwmErr == nil {
		t.Fatal("restoreWithMarker returned nil; expected wrapped state.ErrCorruptIndex")
	}
	if !errors.Is(rwmErr, state.ErrCorruptIndex) {
		t.Errorf("restoreWithMarker err = %v; want errors.Is(err, state.ErrCorruptIndex) = true", rwmErr)
	}

	// No user-visible sessions should have been created. The orchestrator
	// auto-starts a tmux server when it sets @portal-restoring, but no
	// new-session call should follow the corrupt-index abort, so any session
	// present must be Portal's reserved bootstrap session
	// (tmux.PortalBootstrapName), not "alpha" or similar saved names.
	out, err := ts.TryRun("list-sessions", "-F", "#{session_name}")
	if err == nil {
		// If tmux did auto-start a server it will list at most the reserved
		// bootstrap session. Anything else means restore created a session.
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line == tmux.PortalBootstrapName {
				continue
			}
			t.Errorf("unexpected session %q after corrupt-index restore; out=%q", line, out)
		}
	}
}

// TestPhase3Integration_RestoreUsesLiveIndicesUnderBaseIndexDrift is the
// regression test for Phase 7 task 7-9: when a session is saved with default
// (0,0) indices but restored against a tmux server configured with non-zero
// base-index/pane-base-index, FIFO paths and skeleton-marker keys must be
// derived from the LIVE list-panes output, not from the saved indices or any
// prediction-only target.
//
// Test flow:
//  1. Capture a session "alpha" against a server with default 0/0 indices.
//  2. Persist sessions.json.
//  3. Kill the server.
//  4. Start a fresh server with base-index=1 + pane-base-index=1.
//  5. Run Restore via the orchestrator.
//  6. Assert: FIFO exists at the LIVE key (alpha:1.1), not at the saved key
//     (alpha:0.0); the skeleton marker is set against the LIVE key.
func TestPhase3Integration_RestoreUsesLiveIndicesUnderBaseIndexDrift(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-")
	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// CAPTURE under default 0/0 base / pane-base indices — sessions.json
	// records pane at 0:0.
	ts.Run(t, "new-session", "-d", "-s", "alpha")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	client := ts.Client()
	idx, err := state.CaptureStructure(client, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	if len(idx.Sessions) != 1 {
		t.Fatalf("expected 1 captured session, got %d", len(idx.Sessions))
	}
	// Sanity: the saved entry's pane index is 0 (the default).
	savedPane := idx.Sessions[0].Windows[0].Panes[0]
	if savedPane.Index != 0 {
		t.Fatalf("saved pane Index = %d, want 0 (capture should reflect server default)", savedPane.Index)
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := writeFile(state.SessionsJSON(stateDir), data); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	// KILL the server so we can bring up a fresh one with drifted indices.
	ts.KillServer()

	// Bring up a fresh server with base-index=1 / pane-base-index=1. A
	// bootstrap session keeps the server alive while we set the options;
	// tmux exits when there are no sessions.
	ts.Run(t, "new-session", "-d", "-s", "_bootstrap")
	ts.WaitForSession(t, "_bootstrap", 2*time.Second)
	tmuxtest.ApplyBaseIndices(t, ts, 1, 1)

	// RESTORE.
	logger := restoretest.OpenTestLogger(t, stateDir)

	o := &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
	}
	if err := restoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	// VERIFY: alpha is alive again under base-index=1.
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected alpha in list-sessions; got %q", out)
	}

	// VERIFY: list-panes against alpha returns the LIVE coords (1,1) under
	// the drifted base indices.
	livePanesOut := ts.Run(t, "list-panes", "-s", "-t", "alpha", "-F", "#{window_index}:#{pane_index}")
	livePanesOut = strings.TrimSpace(livePanesOut)
	if livePanesOut != "1:1" {
		t.Fatalf("alpha live panes = %q, want %q (base-index drift)", livePanesOut, "1:1")
	}

	// VERIFY: FIFO exists at the LIVE key (1,1), not the saved (0,0).
	liveKey := state.SanitizePaneKey("alpha", 1, 1)
	liveFIFO := state.FIFOPath(stateDir, liveKey)
	if _, err := os.Lstat(liveFIFO); err != nil {
		t.Errorf("expected FIFO at live key %s, missing: %v", liveFIFO, err)
	}
	savedKey := state.SanitizePaneKey("alpha", 0, 0)
	savedFIFO := state.FIFOPath(stateDir, savedKey)
	if _, err := os.Lstat(savedFIFO); err == nil {
		t.Errorf("did not expect FIFO at saved-key path %s under index drift", savedFIFO)
	}

	// VERIFY: skeleton marker is set against the LIVE key.
	wantMarker := "@portal-skeleton-" + liveKey
	markerOut := ts.Run(t, "show-options", "-sv", wantMarker)
	if strings.TrimSpace(markerOut) == "" {
		t.Errorf("expected marker %q to be set; got empty value", wantMarker)
	}
	dontWantMarker := "@portal-skeleton-" + savedKey
	if out, err := ts.TryRun("show-options", "-sv", dontWantMarker); err == nil && strings.TrimSpace(out) != "" {
		t.Errorf("did not expect marker %q (saved-key); got %q", dontWantMarker, out)
	}
}

// writeFile is a thin wrapper that pins the file mode for state-directory
// writes (sessions.json is mode 0600 on production paths) so the test stays
// faithful to the on-disk semantics.
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

// pathExists reports whether path exists. Returns false on any stat error
// (including ENOENT and EACCES) — callers that need to distinguish the two
// should call os.Lstat directly.
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
