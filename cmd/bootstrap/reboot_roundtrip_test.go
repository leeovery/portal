//go:build integration

// Phase 5 task 5-9 — end-to-end reboot round-trip.
//
// This file holds the highest-value regression guard for the
// session-resurrection workflow: a save-then-kill-server-then-restore-
// then-hydrate cycle that asserts every dimension the spec calls out
// (structure, layout, zoom, CWD, environment, hook firing, ANSI scrollback
// bytes) survives a tmux server reboot.
//
// Why this lives in cmd/bootstrap/ (and not internal/restore/):
//   - The Phase 3 round-trip (TestPhase3Integration_SaveRestoreRoundTrip)
//     proves the in-package skeleton-restore primitive works against a
//     fresh server. It does NOT exercise the bootstrap orchestrator's
//     nine-step wiring nor the in-pane hydrate helper.
//   - This test wires the full bootstrap.Orchestrator with the production
//     RestoreAdapter so step ordering, marker lifecycle, and FIFO arming
//     are all exercised. It then drives the FIFO byte-write that
//     `portal state signal-hydrate` performs in production, letting each
//     pane's `portal state hydrate` helper run to completion (dump
//     scrollback → unset marker → fire on-resume hook → exec $SHELL).
//
// Why we drive the FIFO directly rather than spawn a real attach:
//   - Per the planning doc (5-9 acceptance bullet) full PTY attach is
//     fragile across CI environments. Direct FIFO drive is documented as
//     acceptable and is byte-equivalent to what signal-hydrate does
//     (open O_WRONLY|O_NONBLOCK + write 1 byte).
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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// roundTripCfg parameterises the round-trip helper so the default-indices
// run and the base-index-drift sub-test share one body.
//
//	saveBase / savePaneBase: the tmux base-index / pane-base-index in
//	  effect when the snapshot was captured. Encoded into sessions.json.
//	restoreBase / restorePaneBase: the tmux base-index / pane-base-index
//	  the fresh server is brought up with after kill. Drives the live
//	  pane indices the hydrate helper operates against.
//
// When the saved and restore values match, the test exercises the
// "no drift" path — saved index == live index, helper hookKey == live
// paneKey. When they differ, the test exercises the spec's drift
// guarantee: hooks.json lookup uses the saved structural identifier,
// not the live (post-drift) paneKey, so on-resume hooks survive a
// base-index change between save and restore.
type roundTripCfg struct {
	saveBase, savePaneBase       int
	restoreBase, restorePaneBase int
}

// TestPhase5RebootRoundTripEndToEnd is the primary save → kill → restore
// → hydrate round-trip. It asserts that every dimension the spec calls
// out survives a tmux server reboot when base-index / pane-base-index
// stay constant (the steady-state common case).
func TestPhase5RebootRoundTripEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRebootRoundTrip(t, roundTripCfg{
		saveBase: 0, savePaneBase: 0,
		restoreBase: 0, restorePaneBase: 0,
	})
}

// TestPhase5RebootRoundTripBaseIndexDrift flips base-index and
// pane-base-index between save and restore (saved 0/0, restored 1/1)
// and asserts the structural-key lookup still resolves: the on-resume
// hook is keyed by the saved structural identifier (`alpha:0.0`), but
// the live pane after restore lives at (`alpha:1.1`). The hydrate
// helper consults hooks.json by the saved key — which is what
// SessionRestorer.collectArmInfos puts into the helper's --hook-key
// flag — so the hook still fires.
func TestPhase5RebootRoundTripBaseIndexDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRebootRoundTrip(t, roundTripCfg{
		saveBase: 0, savePaneBase: 0,
		restoreBase: 1, restorePaneBase: 1,
	})
}

// runRebootRoundTrip is the shared body of the two round-trip sub-tests.
// It builds the portal binary so the in-pane hydrate helper resolves on
// PATH, sets up isolated state + hooks dirs, captures a hand-crafted
// topology under saveBase indices, kills the server, restarts it under
// restoreBase indices, runs the bootstrap orchestrator with production-
// adapter wirings, drives signal-hydrate via direct FIFO byte-writes,
// and asserts on every dimension the spec covers.
func runRebootRoundTrip(t *testing.T, cfg roundTripCfg) {
	t.Helper()

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	// Hook side-effect file. The on-resume hook appends a line on each
	// firing; the test reads it after hydrate completes to assert the
	// hook fired exactly once (not zero times — proves it ran; not more
	// than once — proves the helper's `exec $SHELL` actually replaced
	// the helper rather than spawning a child).
	hookFireFile := filepath.Join(t.TempDir(), "hook-fired.txt")

	// Per-pane CWDs; t.TempDir paths are guaranteed to exist for the
	// duration of the test so new-session -c / split-window -c won't
	// reject them.
	cwdAlphaW0 := t.TempDir()
	cwdAlphaW1 := t.TempDir()
	cwdBeta := t.TempDir()

	envValue := "round-trip-test-value"

	// Saved structural identifier of the pane that owns the on-resume
	// hook. Saved indices use the saveBase / savePaneBase pair; the
	// helper invokes hooks lookup with this exact string regardless of
	// any drift on restore.
	savedHookKey := tmux.PaneTarget("alpha",
		cfg.saveBase+0, cfg.savePaneBase+0)

	// Register the on-resume hook against the saved structural key.
	// The hook command appends a marker to hookFireFile. `>>` ensures
	// concurrent fires from the test would each leave a distinct line,
	// which is how we assert "exactly once" rather than "at least once".
	hookCmd := fmt.Sprintf("echo HOOK_FIRED >> %s", hookFireFile)
	store := hooks.NewStore(hooksPath)
	if err := store.Set(savedHookKey, "on-resume", hookCmd); err != nil {
		t.Fatalf("hooks.Set: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-rt-")
	client := ts.Client()

	// Bootstrap the server with a temporary "_seed" session so we can
	// configure base-index BEFORE creating "alpha" / "beta". The seed is
	// underscore-prefixed so state.CaptureStructure (via keepSessionNames)
	// excludes it from the captured Index — same discipline the daemon
	// uses to skip the `_portal-saver` session in production.
	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)

	// Apply save-time base indices to the tmux server. set-option -g
	// affects new sessions; -s affects what `show-options -s` reports.
	// We need both so capture sees the value AND tmux assigns the
	// configured indices to the sessions we create next.
	applyBaseIndices(t, ts, cfg.saveBase, cfg.savePaneBase)

	// Build the saved topology live in tmux. Two sessions:
	//
	//   alpha (multi-window, multi-pane):
	//     window 0: 2 panes
	//       pane 0: cwdAlphaW0; carries the on-resume hook
	//       pane 1: cwdAlphaW0; will be marked zoomed (sole zoom in
	//               the round-trip)
	//     window 1: 1 pane
	//       pane 0: cwdAlphaW1; will be marked the active pane of
	//               that window
	//     environment: PORTAL_TEST_ENV=<envValue>
	//
	//   beta (single window, single pane):
	//     window 0: 1 pane @ cwdBeta
	//
	// Two sessions exercise the loop logic; the multi-window/pane
	// alpha covers the geometry surface (layout, zoom, active pane);
	// beta acts as the trivial-topology control case.
	createSavedTopology(t, ts, savedTopologyArgs{
		envValue:   envValue,
		cwdAlphaW0: cwdAlphaW0,
		cwdAlphaW1: cwdAlphaW1,
		cwdBeta:    cwdBeta,
		base:       cfg.saveBase,
		paneBase:   cfg.savePaneBase,
	})

	// Drive the save path: capture the live structure, write per-pane
	// scrollback files, and atomically commit sessions.json. This is
	// exactly what daemonDeps.captureAndCommit does — invoked here
	// outside the tick loop so the test doesn't depend on daemon
	// timing or the saver session existing.
	idx := captureAndCommit(t, client, stateDir)

	// Override one pane's scrollback with a known ANSI fixture. Doing
	// this AFTER Commit means the on-disk schema is still produced by
	// the real save path (file paths, modes, parent dir creation are
	// the production code's responsibility), and we only swap the
	// bytes for determinism — capture-pane output is timing- and
	// terminal-dependent which would make a byte-compare flaky.
	ansiFixture := []byte("\x1b[31mred\x1b[0m\nbefore reboot\n")
	hookPaneKey := state.SanitizePaneKey("alpha",
		cfg.saveBase+0, cfg.savePaneBase+0)
	hookScrollbackPath := state.ScrollbackFile(stateDir, hookPaneKey)
	if err := os.WriteFile(hookScrollbackPath, ansiFixture, 0o600); err != nil {
		t.Fatalf("write fixture scrollback: %v", err)
	}

	// Sanity: the captured index must match the topology we built.
	// This guards against the test silently regressing into a no-op
	// (e.g. capture mis-classifies the saver session and drops alpha).
	verifyCapturedIndex(t, idx, cfg)

	// Kill the server. The next list-sessions MUST error — proves the
	// server actually died and the upcoming Restore is operating
	// against a fresh server, not a still-live one.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}

	// Bring up a fresh server with a `_seed` bootstrap session so we
	// can set base-index BEFORE Restore creates alpha/beta. Mirrors
	// the pre-save bootstrap discipline: underscore-prefixed names
	// are excluded from any subsequent capture, so the seed session
	// is invisible to the spec-level state model.
	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)
	applyBaseIndices(t, ts, cfg.restoreBase, cfg.restorePaneBase)

	// Wire the bootstrap orchestrator with production adapters for the
	// steps we want to exercise (Restoring marker lifecycle, real
	// Restore via RestoreAdapter, real FIFOSweeper) and no-op stubs for
	// the rest (Hooks registration, EnsureSaver, CleanStale) — the
	// step under test is Restore + the marker discipline around it,
	// not hook registration or saver bootstrap.
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
		Hooks:     bootstrap.NoOpHooks{},
		Restoring: &bootstrapadapter.RestoringMarker{Client: client},
		Saver:     bootstrap.NoOpSaver{},
		Restore:   &bootstrapadapter.RestoreAdapter{Inner: restoreInner},
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		Clean: bootstrap.NoOpStaleCleaner{},
	}

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// Assert structure / layout / zoom / CWDs / environment NOW —
	// before signal-hydrate fires. At this point each restored pane
	// is running `portal state hydrate ...` blocked on its FIFO
	// open(O_RDONLY): the pane's `pane_current_path` is the saved
	// cwd, no shell has started, and no rc-file `cd` could have
	// shifted things. Checking these dimensions at the post-shell
	// point would be ambiguous (e.g. an oh-my-zsh user's `.zshrc`
	// runs `cd ~` so pane_current_path would no longer match the
	// captured cwd even though Restore did the right thing).
	verifyLiveStructure(t, ts, cfg)
	verifyLayoutAndZoom(t, ts, cfg)
	verifyCWDs(t, ts, cfg, cwdAlphaW0, cwdAlphaW1, cwdBeta)
	verifyEnvironment(t, client, "alpha",
		"PORTAL_TEST_ENV", envValue)

	// Drive signal-hydrate: for each restored pane, write 1 byte to
	// its FIFO (with the same ENXIO/EAGAIN retry budget the
	// production cmd/state_signal_hydrate.go uses). The helper inside
	// the pane will then read the byte, dump scrollback, sleep
	// 100ms, unset the skeleton marker, fire the on-resume hook, and
	// exec $SHELL.
	restoretest.DriveSignalHydrate(t, client, stateDir,
		[]string{"alpha", "beta"})

	// Wait for every helper to finish: the spec contract is that the
	// helper unsets its `@portal-skeleton-<paneKey>` server option
	// post-dump and post-settle (step 8 of "Helper Behavior on
	// Startup"). Polling the marker set is a reliable barrier without
	// having to introspect helper subprocesses.
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second)

	// ANSI scrollback bytes survive: capture-pane -e -p -S - against
	// the hook-owning pane should contain the red SGR sequence and
	// the literal "before reboot" line we wrote to the .bin file.
	verifyANSIScrollback(t, ts, "alpha",
		cfg.restoreBase+0, cfg.restorePaneBase+0)

	// Hook fired exactly once. The helper's `exec $SHELL` step
	// replaces the helper process so the hook command runs once
	// and only once per restored pane (per the spec §"Resume
	// Hooks", the helper-driven path fires on reboot recovery).
	verifyHookFiredOnce(t, hookFireFile)
}

// savedTopologyArgs bundles the per-CWD strings and base indices used to
// stand up the live pre-save topology. Grouping the inputs in a struct
// keeps createSavedTopology's signature short and intent clear at call
// sites.
type savedTopologyArgs struct {
	envValue   string
	cwdAlphaW0 string
	cwdAlphaW1 string
	cwdBeta    string
	base       int
	paneBase   int
}

// createSavedTopology stands up the alpha + beta saved sessions live in
// tmux. The shape mirrors what the round-trip body asserts on
// (multi-window/multi-pane alpha with one zoomed pane and a per-session
// env var; trivial-topology beta as a control). All shells are spawned
// with `sleep infinity` so panes outlive the test body without doing
// anything that would contaminate scrollback — the round-trip's
// scrollback fixture is overwritten on disk after capture, so the
// in-pane content here doesn't matter.
func createSavedTopology(t *testing.T, ts *tmuxtest.Socket, args savedTopologyArgs) {
	t.Helper()
	// alpha session, window 0, pane 0 — the pane that will own the
	// on-resume hook. Initial command "sleep infinity" so the pane
	// stays alive without producing scrollback noise.
	ts.Run(t, "new-session", "-d", "-s", "alpha", "-c", args.cwdAlphaW0, "sleep", "infinity")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	// Set per-session environment variable.
	ts.Run(t, "set-environment", "-t", "alpha", "PORTAL_TEST_ENV", args.envValue)

	// alpha window 0, pane 1 — split-window into the same window.
	ts.Run(t, "split-window", "-t", "alpha", "-c", args.cwdAlphaW0, "sleep", "infinity")

	// alpha window 1 — new-window, then nothing else (single pane).
	ts.Run(t, "new-window", "-t", "alpha", "-c", args.cwdAlphaW1, "sleep", "infinity")

	// Mark alpha:w0.p1 as zoomed — resize-pane -Z is a toggle, so we
	// only call it once. The captured Window.Zoomed should reflect
	// `#{window_zoomed_flag}` for window 0 = true.
	zoomTarget := tmux.PaneTarget("alpha", args.base+0, args.paneBase+1)
	ts.Run(t, "resize-pane", "-t", zoomTarget, "-Z")

	// Make alpha:w1.p0 the active pane of its window. (Single-pane
	// windows always have their sole pane active, so this is mostly
	// a sanity statement; the assertion later proves the restored
	// active-pane bit matches the captured one.)

	// beta session — single window, single pane.
	ts.Run(t, "new-session", "-d", "-s", "beta", "-c", args.cwdBeta, "sleep", "infinity")
	ts.WaitForSession(t, "beta", 2*time.Second)
}

// applyBaseIndices sets server-scope and global base-index / pane-base-
// index on the live tmux server. -g controls the values new sessions
// inherit; -s controls what `show-option -sv` reports — both matter for
// PredictLiveIndices and for the live coords tmux assigns to fresh
// sessions/panes.
func applyBaseIndices(t *testing.T, ts *tmuxtest.Socket, base, paneBase int) {
	t.Helper()
	ts.Run(t, "set-option", "-g", "base-index", fmt.Sprintf("%d", base))
	ts.Run(t, "set-option", "-g", "pane-base-index", fmt.Sprintf("%d", paneBase))
	ts.Run(t, "set-option", "-s", "base-index", fmt.Sprintf("%d", base))
	ts.Run(t, "set-option", "-s", "pane-base-index", fmt.Sprintf("%d", paneBase))
}

// captureAndCommit drives the daemon's per-tick save path inline:
//   - List skeleton markers (none on a fresh server, so empty set).
//   - state.CaptureStructure to walk live sessions/windows/panes.
//   - Write per-pane scrollback bytes (we use a deterministic placeholder;
//     the round-trip body overwrites the hook pane's file with a known
//     ANSI fixture afterwards).
//   - state.Commit to atomically persist sessions.json.
//
// Returns the captured Index so the caller can sanity-check.
func captureAndCommit(t *testing.T, client *tmux.Client, stateDir string) state.Index {
	t.Helper()
	skipSet, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}
	idx, err := state.CaptureStructure(client, skipSet, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}

	// Write per-pane scrollback. CaptureAndHashPane is the production
	// helper, but we don't need its output bytes here — the round-trip
	// body overwrites the one file we'll byte-compare. Other panes get
	// empty bytes so file presence (and Commit's GC discipline) is
	// exercised but no large/non-deterministic content lands on disk.
	hm := state.HashMap{}
	anyChanged := false
	for _, sess := range idx.Sessions {
		for _, w := range sess.Windows {
			for _, p := range w.Panes {
				key := state.SanitizePaneKey(sess.Name, w.Index, p.Index)
				written, err := state.WriteScrollbackIfChanged(stateDir, key, []byte{}, 0, hm)
				if err != nil {
					t.Fatalf("WriteScrollbackIfChanged %s: %v", key, err)
				}
				if written {
					anyChanged = true
				}
			}
		}
	}
	if err := state.Commit(stateDir, idx, anyChanged, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return idx
}

// verifyCapturedIndex sanity-checks that the captured snapshot matches
// the topology createSavedTopology built. Failures here mean the test
// silently degraded (e.g. capture dropped a session and the round-trip
// is no longer testing what its name claims).
func verifyCapturedIndex(t *testing.T, idx state.Index, cfg roundTripCfg) {
	t.Helper()
	if got := len(idx.Sessions); got != 2 {
		t.Fatalf("captured %d sessions; want 2", got)
	}
	// Sessions are sorted alphabetically by Canonicalize.
	if idx.Sessions[0].Name != "alpha" || idx.Sessions[1].Name != "beta" {
		t.Fatalf("session names = [%s, %s]; want [alpha, beta]",
			idx.Sessions[0].Name, idx.Sessions[1].Name)
	}
	alpha := idx.Sessions[0]
	if got := len(alpha.Windows); got != 2 {
		t.Fatalf("alpha windows = %d; want 2", got)
	}
	if got := len(alpha.Windows[0].Panes); got != 2 {
		t.Fatalf("alpha w0 panes = %d; want 2", got)
	}
	if got := len(alpha.Windows[1].Panes); got != 1 {
		t.Fatalf("alpha w1 panes = %d; want 1", got)
	}
	// Zoom flag must round-trip to the captured Window.
	if !alpha.Windows[0].Zoomed {
		t.Fatalf("alpha w0 not zoomed in capture; want zoomed=true")
	}
	// Saved indices reflect the saveBase / savePaneBase configuration.
	if alpha.Windows[0].Index != cfg.saveBase {
		t.Errorf("alpha w0.Index = %d; want %d", alpha.Windows[0].Index, cfg.saveBase)
	}
	if alpha.Windows[0].Panes[0].Index != cfg.savePaneBase {
		t.Errorf("alpha w0p0.Index = %d; want %d", alpha.Windows[0].Panes[0].Index, cfg.savePaneBase)
	}
	// Per-session environment captured.
	if got := alpha.Environment["PORTAL_TEST_ENV"]; got == "" {
		t.Errorf("alpha env PORTAL_TEST_ENV missing in capture; got %v", alpha.Environment)
	}
}

// verifyLiveStructure asserts the restored topology matches the saved
// shape: alpha has windows at restoreBase+{0,1} with the right pane
// counts; beta has a single window with a single pane.
func verifyLiveStructure(t *testing.T, ts *tmuxtest.Socket, cfg roundTripCfg) {
	t.Helper()
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("session %q missing post-restore; got %q", want, out)
		}
	}
	alphaPanes := ts.Run(t, "list-panes", "-s", "-t", "alpha",
		"-F", "#{window_index}:#{pane_index}")
	wantAlphaPanes := []string{
		fmt.Sprintf("%d:%d", cfg.restoreBase+0, cfg.restorePaneBase+0),
		fmt.Sprintf("%d:%d", cfg.restoreBase+0, cfg.restorePaneBase+1),
		fmt.Sprintf("%d:%d", cfg.restoreBase+1, cfg.restorePaneBase+0),
	}
	for _, want := range wantAlphaPanes {
		if !strings.Contains(alphaPanes, want) {
			t.Errorf("alpha live pane %q missing; got %q", want, alphaPanes)
		}
	}
	betaPanes := ts.Run(t, "list-panes", "-s", "-t", "beta",
		"-F", "#{window_index}:#{pane_index}")
	wantBeta := fmt.Sprintf("%d:%d", cfg.restoreBase+0, cfg.restorePaneBase+0)
	if !strings.Contains(betaPanes, wantBeta) {
		t.Errorf("beta live pane %q missing; got %q", wantBeta, betaPanes)
	}
}

// verifyLayoutAndZoom asserts that ApplyWindowGeometry produced the
// saved zoom state: alpha:w0 should have window_zoomed_flag=1, alpha:w1
// should be zoomed=0 (we only zoomed w0 pre-save).
func verifyLayoutAndZoom(t *testing.T, ts *tmuxtest.Socket, cfg roundTripCfg) {
	t.Helper()
	w0 := ts.Run(t, "display-message", "-p",
		"-t", fmt.Sprintf("alpha:%d", cfg.restoreBase+0),
		"#{window_zoomed_flag}")
	if strings.TrimSpace(w0) != "1" {
		t.Errorf("alpha:%d zoom flag = %q; want 1", cfg.restoreBase+0, w0)
	}
	w1 := ts.Run(t, "display-message", "-p",
		"-t", fmt.Sprintf("alpha:%d", cfg.restoreBase+1),
		"#{window_zoomed_flag}")
	if strings.TrimSpace(w1) != "0" {
		t.Errorf("alpha:%d zoom flag = %q; want 0", cfg.restoreBase+1, w1)
	}
}

// verifyCWDs asserts every restored pane's pane_current_path matches the
// CWD captured pre-save. Spec § "What Survives a Reboot" — CWD is part
// of the structural snapshot and must round-trip.
func verifyCWDs(t *testing.T, ts *tmuxtest.Socket, cfg roundTripCfg, cwdAlphaW0, cwdAlphaW1, cwdBeta string) {
	t.Helper()
	cases := []struct {
		target string
		want   string
	}{
		{tmux.PaneTarget("alpha", cfg.restoreBase+0, cfg.restorePaneBase+0), cwdAlphaW0},
		{tmux.PaneTarget("alpha", cfg.restoreBase+0, cfg.restorePaneBase+1), cwdAlphaW0},
		{tmux.PaneTarget("alpha", cfg.restoreBase+1, cfg.restorePaneBase+0), cwdAlphaW1},
		{tmux.PaneTarget("beta", cfg.restoreBase+0, cfg.restorePaneBase+0), cwdBeta},
	}
	for _, c := range cases {
		got := strings.TrimSpace(ts.Run(t, "display-message", "-p",
			"-t", c.target, "#{pane_current_path}"))
		// macOS resolves t.TempDir paths through /private/var → /var
		// symlinks; tmux reports the resolved path. Compare suffixes
		// rather than full paths so we don't over-specify.
		gotResolved := resolveSymlinks(got)
		wantResolved := resolveSymlinks(c.want)
		if gotResolved != wantResolved {
			t.Errorf("cwd %s = %q (resolved %q); want %q (resolved %q)",
				c.target, got, gotResolved, c.want, wantResolved)
		}
	}
}

// resolveSymlinks follows symlinks in path; on error returns the
// original path. macOS-only concern — t.TempDir lives under
// /var/folders which is itself a symlink to /private/var/folders, and
// tmux reports the underlying path. Callers compare resolved paths so
// the test does not differ between linux and darwin.
func resolveSymlinks(p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return r
}

// verifyEnvironment asserts the named per-session environment variable
// round-tripped — i.e. SetSessionEnvironment was called during Restore
// with the captured value. show-environment lists all set vars; we
// search for the exact KEY=VALUE line.
func verifyEnvironment(t *testing.T, client *tmux.Client, session, key, want string) {
	t.Helper()
	out, err := client.ShowEnvironment(session)
	if err != nil {
		t.Fatalf("ShowEnvironment %q: %v", session, err)
	}
	wantLine := key + "=" + want
	if !strings.Contains(out, wantLine) {
		t.Errorf("session %q env missing %q; got:\n%s", session, wantLine, out)
	}
}

// verifyANSIScrollback captures the live pane buffer (with ANSI escapes
// preserved via -e) and asserts the seeded fixture survived: the SGR
// red sequence MUST be present and the literal "before reboot" line
// MUST appear. Substring assertions are appropriate here — capture-pane
// adds its own line wrapping and trailing whitespace, so a byte-for-byte
// compare against the fixture would be brittle without serving zero
// extra coverage.
func verifyANSIScrollback(t *testing.T, ts *tmuxtest.Socket, session string, win, pane int) {
	t.Helper()
	target := tmux.PaneTarget(session, win, pane)
	out := ts.Run(t, "capture-pane", "-e", "-p", "-S", "-", "-t", target)
	// Red SGR on (\x1b[31m). The reset (\x1b[0m) and "before reboot"
	// line are likewise expected; we assert each independently so a
	// failing test pinpoints which dimension regressed.
	checks := []struct {
		needle string
		desc   string
	}{
		{"\x1b[31m", "red SGR escape"},
		{"red", "red literal"},
		{"before reboot", "fixture text"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.needle) {
			t.Errorf("scrollback for %s missing %s (%q); got:\n%q",
				target, c.desc, c.needle, out)
		}
	}
}

// verifyHookFiredOnce asserts the on-resume hook ran exactly once. The
// hook command is `echo HOOK_FIRED >> file`, so the marker count in the
// file is the firing count. Zero firings means hook lookup failed
// (possibly because hooks.json wasn't on the helper's path or hookKey
// drift under base-index drift wasn't handled). Multiple firings would
// mean the helper's `exec $SHELL` branch didn't actually exec — the
// shell must replace the helper, not spawn a child.
func verifyHookFiredOnce(t *testing.T, hookFireFile string) {
	t.Helper()
	data, err := os.ReadFile(hookFireFile)
	if err != nil {
		t.Fatalf("read hook fire file %s: %v", hookFireFile, err)
	}
	count := strings.Count(string(data), "HOOK_FIRED")
	if count != 1 {
		t.Errorf("hook fired %d times; want exactly 1\nfile contents:\n%s",
			count, data)
	}
}
