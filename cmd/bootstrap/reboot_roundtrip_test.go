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
//     eleven-step wiring nor the in-pane hydrate helper.
//   - This test wires the full bootstrap.Orchestrator with the production
//     RestoreAdapter so step ordering, marker lifecycle, and FIFO arming
//     are all exercised. It then drives the FIFO byte-write that
//     `portal state signal-hydrate` performs in production, letting each
//     pane's `portal state hydrate` helper run to completion (dump
//     scrollback → unset marker → fire on-resume hook → exec $SHELL).
//
// Coverage of the production hook pipeline (Phase 13 task 13-2):
//   - The default sub-tests (TestPhase5RebootRoundTripEndToEnd and
//     TestPhase5RebootRoundTripBaseIndexDrift) exec the built `portal`
//     binary's `state signal-hydrate <session>` subcommand for each
//     restored session — argv-identical to what the registered tmux
//     `client-attached` hook invokes via `run-shell`. This exercises the
//     full hook → CLI argv → runSignalHydrate body → FIFO write pipeline
//     end-to-end.
//   - TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary restores
//     two sessions and exec's signal-hydrate for each in sequence,
//     simulating a `client-attached` (session A) followed by a
//     `client-session-changed` (session B) — the two hooks the spec
//     registers as siblings (spec § Hook Registration). Both sessions
//     hydrate to completion; the test asserts every restored pane's
//     skeleton marker is cleared and verifyLiveStructure still passes.
//   - DriveSignalHydrate (direct FIFO byte-write) remains as a fallback
//     for sub-tests where exec'ing the binary would re-walk paths
//     covered upstream (notably the base-index drift variant). Per the
//     Phase 5 task 5-9 acceptance bullet's tolerance for CI fragility,
//     keeping a direct-FIFO fallback ensures the gate stays green even
//     if the binary path proves flaky on a future CI lane.
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
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
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
	// useBinary selects the hydrate-driver: when true, runRebootRoundTrip
	// exec's the built `portal state signal-hydrate <session>` binary —
	// argv-identical to the production `client-attached` hook's
	// `run-shell` invocation. When false, the test falls back to
	// DriveSignalHydrate's direct FIFO byte-write (byte-equivalent but
	// bypasses the CLI argv → cobra → runSignalHydrate plumbing).
	//
	// Phase 13 task 13-2 acceptance: at least one round-trip subtest
	// drives via the binary; the base-index drift variant retains the
	// fallback because exec'ing the binary against a divergent live
	// base-index re-walks paths the binary-driven primary path already
	// covers, and the variant's contribution is structural-key drift
	// resolution — not the hook pipeline.
	useBinary bool
}

// TestPhase5RebootRoundTripEndToEnd is the primary save → kill → restore
// → hydrate round-trip. It asserts that every dimension the spec calls
// out survives a tmux server reboot when base-index / pane-base-index
// stay constant (the steady-state common case).
//
// Phase 5 task 5-9 acceptance bullets satisfied by this sub-test:
//   - structure / layout / zoom / CWDs / environment / ANSI scrollback
//     round-trip
//   - resume-hook command captures an assertable side-effect (firing
//     exactly once)
//   - `@portal-skeleton-<paneKey>` markers cleared after hydration
//   - `client-attached` hook pathway exercised end-to-end via the
//     binary-driven hydrate driver (Phase 13 task 13-2)
func TestPhase5RebootRoundTripEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRebootRoundTrip(t, roundTripCfg{
		saveBase: 0, savePaneBase: 0,
		restoreBase: 0, restorePaneBase: 0,
		useBinary: true,
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
//
// Phase 5 task 5-9 acceptance bullet satisfied by this sub-test:
//   - `base-index` / `pane-base-index` variation covered (the saved
//     vs restored indices diverge here, exercising the structural-key
//     drift contract).
//
// This sub-test retains the direct-FIFO DriveSignalHydrate driver as a
// CI flake-tolerant fallback. The hook pipeline itself is covered by
// the binary-driven primary sub-test above; the variant's distinct
// contribution is structural-key drift resolution, which the binary
// path would re-walk identically.
func TestPhase5RebootRoundTripBaseIndexDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRebootRoundTrip(t, roundTripCfg{
		saveBase: 0, savePaneBase: 0,
		restoreBase: 1, restorePaneBase: 1,
		useBinary: false,
	})
}

// runRebootRoundTrip is the shared body of the round-trip sub-tests. It
// builds the portal binary so the in-pane hydrate helper resolves on
// PATH, sets up isolated state + hooks dirs, captures a hand-crafted
// topology under saveBase indices, kills the server, restarts it under
// restoreBase indices, runs the bootstrap orchestrator with production-
// adapter wirings, drives signal-hydrate (via the built portal binary
// when cfg.useBinary, otherwise via direct FIFO byte-writes), and
// asserts on every dimension the spec covers.
func runRebootRoundTrip(t *testing.T, cfg roundTripCfg) {
	t.Helper()

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	stateDir := newIntegrationStateDir(t)

	// Isolated baseline env for any subprocess spawn that needs an
	// XDG_CONFIG_HOME scoped away from the developer's real
	// ~/.config/portal/ — see Component G of
	// slow-open-empty-previews-and-zombie-sessions. The stateDir is
	// still wired via PORTAL_STATE_DIR (newIntegrationStateDir above),
	// the env baseline below merely guarantees XDG_CONFIG_HOME does
	// not leak.
	env, _ := portaltest.IsolateStateForTest(t)

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
	// hook. Hook-store keys are in tmux's "session:window.pane" form
	// (see internal/restore/session.go:108 — collectArmInfos passes
	// tmux.PaneTarget(...) via --hook-key, and the hooks.Store keys at
	// internal/hooks/store_test.go follow the same shape). This is
	// distinct from state.SanitizePaneKey, which is the filesystem-
	// safe key used for FIFO paths, scrollback files, and skeleton
	// markers. Saved indices (saveBase / savePaneBase) are pinned at
	// save time so this string is stable across base-index drift on
	// restore.
	savedHookKey := tmux.PaneTarget("alpha",
		cfg.saveBase+0, cfg.savePaneBase+0)

	// Register the on-resume hook against the saved structural key.
	// The hook command appends a marker to hookFireFile. `>>` ensures
	// concurrent fires from the test would each leave a distinct line,
	// which is how we assert "exactly once" rather than "at least once".
	hookCmd := fmt.Sprintf("echo HOOK_FIRED >> %s", hookFireFile)
	store := hooks.NewStore(hooksPath)
	if err := store.Set(savedHookKey, "on-resume", hookCmd, "cli"); err != nil {
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
	tmuxtest.ApplyBaseIndices(t, ts, cfg.saveBase, cfg.savePaneBase)

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
	idx := runDaemonTick(t, client, stateDir, withoutSkipGuard(), withEmptyScrollback())

	// Override one pane's scrollback with a known ANSI fixture. Doing
	// this AFTER Commit means the on-disk schema is still produced by
	// the real save path (file paths, modes, parent dir creation are
	// the production code's responsibility), and we only swap the
	// bytes for determinism — capture-pane output is timing- and
	// terminal-dependent which would make a byte-compare flaky.
	//
	// NOTE: this on-disk write does NOT update state.HashMap, so the
	// in-memory hash for hookScrollbackPath is stale relative to disk.
	// This test never re-captures after the reboot, so the staleness
	// is invisible. A future variant that adds a post-hydrate
	// runDaemonTick step would need to invalidate the hash entry
	// (or re-seed it) before that capture, otherwise dedup will skip
	// the rewrite and the new fixture won't land on disk.
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
	tmuxtest.ApplyBaseIndices(t, ts, cfg.restoreBase, cfg.restorePaneBase)

	// Wire the bootstrap orchestrator with production adapters for the
	// steps we want to exercise (Restoring marker lifecycle, real
	// Restore via RestoreAdapter, real FIFOSweeper) and no-op stubs for
	// the rest (Hooks registration, EnsureSaver, CleanStale) — the
	// step under test is Restore + the marker discipline around it,
	// not hook registration or saver bootstrap.
	logger := restoretest.OpenTestLogger(t, stateDir)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger),
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		// Opt out of the integration builder's real-EagerSignaler default
		// (task 4-2): this round-trip drives signal-hydrate explicitly
		// via DriveSignalHydrate / DriveSignalHydrateBinary AFTER Run
		// returns, so the test owns the FIFO-write timing. With the real
		// eager signaler firing during Run, the helpers would already
		// have consumed their FIFO byte and exec'd $SHELL by the time
		// the manual driver runs — the manual write would then either
		// no-op against a closed FIFO or race against the helper's
		// post-replay teardown. The eager pipeline is covered
		// end-to-end by TestPhase1Integration_EagerSignalHydrate_*.
		EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{},
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// End-to-end regression guard for the hidden-sessions-showing-on-
	// startup spec. Asserted BEFORE the structural verifiers so a
	// regression (Root Cause 1: `_*` leak through ListSessions, Root
	// Cause 2: bootstrap session named `0` reappears) fails fast and
	// the failure pinpoints the bug rather than cascading downstream.
	verifyPostBootstrapSessionSet(t, ts, client,
		[]string{tmux.PortalBootstrapName, tmux.PortalSaverName, "_seed"},
		[]string{"alpha", "beta"})

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

	// Drive signal-hydrate. cfg.useBinary selects between:
	//   - Production-binary path: exec the built `portal state
	//     signal-hydrate <session>` for each restored session — argv-
	//     identical to what the registered tmux client-attached hook
	//     fires via run-shell. This exercises the full hook pipeline
	//     (CLI argv parsing → cobra dispatch → runSignalHydrate body →
	//     FIFO write).
	//   - Fallback direct-FIFO path: write the FIFO byte directly. Used
	//     by the base-index drift sub-test (where the variant's
	//     contribution is drift resolution, not the hook pipeline).
	// Either way, each helper inside its pane reads the byte, dumps
	// scrollback, sleeps 100ms, unsets the skeleton marker, fires the
	// on-resume hook, and exec's $SHELL.
	if cfg.useBinary {
		restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(),
			stateDir, hooksPath, []string{"alpha", "beta"}, env)
	} else {
		restoretest.DriveSignalHydrate(t, client, stateDir,
			[]string{"alpha", "beta"})
	}

	// Wait for every helper to finish: the spec contract is that the
	// helper unsets its `@portal-skeleton-<paneKey>` server option
	// post-dump and post-settle (step 8 of "Helper Behavior on
	// Startup"). Polling the marker set is a reliable barrier without
	// having to introspect helper subprocesses.
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second, 50*time.Millisecond)

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

	// Spec § "Acceptance Criteria" item 4: the misleading
	// `predicted=...__0.0 live=...__X.Y` WARN must be gone (deleted
	// in Phase 2-1), not silenced. portal.log must contain ZERO lines
	// matching the predicted-vs-live regex after a full bootstrap +
	// hydrate cycle. The base-index drift variant (saveBase=0,
	// restoreBase=1) is the configuration that would have produced
	// the WARN pre-fix; running the assertion here for both variants
	// provides a stronger runtime guarantee at no extra cost.
	verifyNoPredictedVsLiveWarns(t, filepath.Join(stateDir, "portal.log"))
}

// assertNoLogLineMatches reads portal.log at logPath and fails the test on
// the first line for which pred returns true, formatting the failure with
// failFmt + args (the matched line is appended as the final %s arg).
//
// Returns silently on a missing log file: an absent log means no WARN was
// ever emitted, which is the success state. The shared scan plumbing keeps
// file-IO + ENOENT-tolerance + line-iteration in one place; per-call
// diagnostic wording is supplied by the caller.
func assertNoLogLineMatches(t *testing.T, logPath string, pred func(string) bool, failFmt string, args ...any) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read portal.log %s: %v", logPath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if pred(line) {
			t.Fatalf(failFmt, append(args, line)...)
		}
	}
}

// verifyNoPredictedVsLiveWarns reads portal.log and fails if any line matches
// the predictedVsLiveWarnRegex (defined in predicted_vs_live_regex_test.go).
// The single shared regex var keeps the integration assertion and the unit
// test (TestPredictedVsLiveRegex_MatchesOffendingShapeAndIgnoresArmPanesWarning)
// compiling byte-identical patterns — a rename or shape-change in one is a
// rename or shape-change in both.
//
// Returns silently on a missing log file: an absent log means no WARN was
// ever emitted, which is the success state.
func verifyNoPredictedVsLiveWarns(t *testing.T, logPath string) {
	t.Helper()
	assertNoLogLineMatches(t, logPath, predictedVsLiveWarnRegex.MatchString,
		"portal.log contains predicted-vs-live WARN line "+
			"(spec AC #4 — diagnostic must be gone, not silenced): %s")
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

	// beta session — single window, single pane.
	ts.Run(t, "new-session", "-d", "-s", "beta", "-c", args.cwdBeta, "sleep", "infinity")
	ts.WaitForSession(t, "beta", 2*time.Second)
}

// The daemon-tick save path used by this round-trip lives in
// daemon_tick_test_helpers_test.go as runDaemonTick. See that file's
// docstring for why this round-trip opts out of the skip-save guard
// and uses empty scrollback bytes.

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

// verifyPostBootstrapSessionSet is the end-to-end regression guard for
// the hidden-sessions-showing-on-startup spec. It runs two complementary
// assertions against the live tmux server post-bootstrap:
//
// Assertion 1 (Root Cause 2 — RAW TMUX STATE):
//   - Reads tmux's session list directly via `ts.Run("list-sessions",
//     -F #{session_name})`, deliberately bypassing Client.ListSessions.
//     Bypassing the chokepoint filter is load-bearing: a regression
//     where StartServer drops its `-s <name>` arg makes tmux fall back
//     to its default session name `0` — which has no `_*` prefix and
//     would therefore be invisible to a Client.ListSessions-based
//     assertion.
//   - Asserts the raw set is a SUBSET of allowedReserved ∪
//     expectedRestored. The "subset" shape (rather than equality) is
//     deliberate: PortalSaverName is included in allowedReserved for
//     forward-compatibility even though bootstrap.NoOpSaver{} skips
//     creating it in this test path. Equality would force every test
//     wiring to either create _portal-saver or omit it from the
//     superset; subset tolerates either outcome.
//   - Failure message identifies the unexpected name so a regression
//     (e.g. `0` re-appearing after a Fix B regression) pinpoints the
//     leaked name rather than printing only the full set.
//
// Assertion 2 (Root Cause 1 — USER-FACING VISIBILITY):
//   - Calls Client.ListSessions and asserts the returned set equals
//     expectedRestored exactly. Reserved names (PortalBootstrapName,
//     PortalSaverName) MUST be filtered out. A Fix A regression (the
//     `_*` filter being removed) leaks reserved names through to user-
//     facing callers (TUI picker, `portal list`, etc.) and would be
//     caught here.
//   - Failure message identifies the leaked reserved name.
//
// The helper takes both names via the tmux package constants
// (PortalBootstrapName, PortalSaverName) — the test file MUST NOT
// embed the literal strings, so a future rename of either constant is
// caught at compile time, not by a stale string assertion.
func verifyPostBootstrapSessionSet(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, allowedReserved []string, expectedRestored []string) {
	t.Helper()

	// Assertion 1 — raw tmux state. Bypass Client.ListSessions on
	// purpose: see helper docstring.
	rawOut := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	rawSet := map[string]struct{}{}
	for _, line := range strings.Split(rawOut, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		rawSet[name] = struct{}{}
	}

	allowed := map[string]struct{}{}
	for _, n := range allowedReserved {
		allowed[n] = struct{}{}
	}
	for _, n := range expectedRestored {
		allowed[n] = struct{}{}
	}

	for name := range rawSet {
		if _, ok := allowed[name]; !ok {
			t.Errorf("raw tmux session list contains unexpected name %q; "+
				"allowed reserved=%v, expected restored=%v, raw=%v",
				name, allowedReserved, expectedRestored,
				sortedStringSet(rawSet))
		}
	}

	// Assertion 2 — user-facing visibility via Client.ListSessions.
	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("Client.ListSessions: %v", err)
	}
	gotSet := map[string]struct{}{}
	for _, s := range sessions {
		gotSet[s.Name] = struct{}{}
	}

	// Reserved names must be filtered out — assert with named
	// failure messages so a regression pinpoints which leaked.
	for _, reserved := range []string{tmux.PortalBootstrapName, tmux.PortalSaverName} {
		if _, leaked := gotSet[reserved]; leaked {
			t.Errorf("Client.ListSessions leaked reserved name %q; "+
				"underscore-prefix filter regression. got=%v",
				reserved, sortedStringSet(gotSet))
		}
	}

	// Returned set must equal expectedRestored exactly (no missing,
	// no extras among non-reserved names).
	expectedSet := map[string]struct{}{}
	for _, n := range expectedRestored {
		expectedSet[n] = struct{}{}
	}
	for name := range gotSet {
		if _, ok := expectedSet[name]; !ok {
			t.Errorf("Client.ListSessions returned unexpected name %q; "+
				"expected restored=%v, got=%v",
				name, expectedRestored, sortedStringSet(gotSet))
		}
	}
	for _, want := range expectedRestored {
		if _, ok := gotSet[want]; !ok {
			t.Errorf("Client.ListSessions missing expected restored name %q; got=%v",
				want, sortedStringSet(gotSet))
		}
	}
}

// sortedStringSet returns the keys of a string-set sorted, for stable
// failure messages.
func sortedStringSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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

// TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary exercises
// the `client-session-changed` half of the Phase 5 task 5-9 acceptance
// bullet — both `client-attached` AND `client-session-changed` must
// drive hydration end-to-end.
//
// The production hook contract (spec § Hook Registration):
//
//	set-hook -ga client-attached         'run-shell "... portal state signal-hydrate #{session_name}"'
//	set-hook -ga client-session-changed  'run-shell "... portal state signal-hydrate #{session_name}"'
//
// Both hooks invoke argv-identical CLI commands; the only difference is
// the trigger event. This test simulates an attach-then-switch sequence
// by exec'ing the built `portal state signal-hydrate <session>` binary
// for two restored sessions in succession:
//
//  1. signal-hydrate alpha — what client-attached fires when a client
//     first attaches to the restored alpha session.
//  2. signal-hydrate beta — what client-session-changed fires when the
//     attached client subsequently invokes `tmux switch-client -t beta`.
//
// Both invocations exec the production CLI binary, so the full hook
// pipeline (CLI argv → cobra dispatch → runSignalHydrate body → FIFO
// write → in-pane hydrate helper unblock → marker unset) is exercised
// for each session — not bypassed via a direct FIFO write.
//
// Why we don't actually attach a real client and run `tmux switch-
// client`: tmux's switch-client requires a real attached client, which
// requires a PTY. The portal repo has no PTY dependency (see go.mod —
// no creack/pty), and per the task notes "If PTY is too fragile ...
// document the limitation and fall back" — the binary-driven argv
// pipeline is byte-equivalent to what the hook would invoke under a
// real switch-client, with full coverage of every component except the
// tmux event-loop's hook-firing internals (which is tmux's own
// responsibility, not portal's).
//
// Phase 5 task 5-9 acceptance bullet satisfied by this sub-test:
//
//   - `client-session-changed` exercised (in addition to
//     `client-attached` covered by TestPhase5RebootRoundTripEndToEnd).
//   - Both restored sessions complete hydration to "skeleton markers
//     cleared" within the budget.
//   - verifyLiveStructure continues to pass against the
//     production-binary path.
func TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	stateDir := newIntegrationStateDir(t)

	// Isolated baseline env for DriveSignalHydrateBinary subprocess
	// spawns — see Component G of
	// slow-open-empty-previews-and-zombie-sessions.
	env, _ := portaltest.IsolateStateForTest(t)

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	// Two single-pane sessions — minimal topology that still exercises
	// the per-session hook pipeline. The complex multi-window/zoom
	// surface is already covered by TestPhase5RebootRoundTripEndToEnd;
	// this test focuses on the two-step attach+switch sequence and
	// keeps the topology trivial so a failure pinpoints the hook
	// pipeline rather than geometry handling.
	cwdAlpha := t.TempDir()
	cwdBeta := t.TempDir()

	ts := tmuxtest.New(t, "ptl-rt-switch-")
	client := ts.Client()

	// _seed bootstrap so the underscore-prefixed name is excluded from
	// capture (mirrors the daemon's _portal-saver discipline).
	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)

	ts.Run(t, "new-session", "-d", "-s", "alpha", "-c", cwdAlpha, "sleep", "infinity")
	ts.WaitForSession(t, "alpha", 2*time.Second)
	ts.Run(t, "new-session", "-d", "-s", "beta", "-c", cwdBeta, "sleep", "infinity")
	ts.WaitForSession(t, "beta", 2*time.Second)

	// Capture + commit using the same helper the primary round-trip
	// uses, so any regression in CaptureStructure shows up here too.
	idx := runDaemonTick(t, client, stateDir, withoutSkipGuard(), withEmptyScrollback())
	if got := len(idx.Sessions); got != 2 {
		t.Fatalf("captured %d sessions; want 2", got)
	}

	// Kill the server so Restore runs against a fresh one.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}

	logger := restoretest.OpenTestLogger(t, stateDir)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger),
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		// Opt out of the integration builder's real-EagerSignaler default
		// (task 4-2): this test simulates `client-attached` then
		// `client-session-changed` by exec'ing `portal state
		// signal-hydrate <session>` for alpha and beta in sequence —
		// the test's distinct contribution is the per-session sequencing
		// (alpha clears first, beta stays pending until step 2). A real
		// eager signaler firing during Run would clear BOTH markers
		// before the manual driver runs, defeating the per-session
		// sequencing assertion below.
		EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{},
	})
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// End-to-end regression guard (see verifyPostBootstrapSessionSet
	// for full rationale). Catches both Root Cause 1 (`_*` leak
	// through ListSessions) and Root Cause 2 (bootstrap session
	// named `0` reappears).
	verifyPostBootstrapSessionSet(t, ts, client,
		[]string{tmux.PortalBootstrapName, tmux.PortalSaverName, "_seed"},
		[]string{"alpha", "beta"})

	// Sanity: both sessions are live and skeleton-marked before we
	// drive any hook. If markers were never set, the test would
	// silently no-op below — explicitly assert the pre-condition.
	markersBefore, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers (pre-drive): %v", err)
	}
	if len(markersBefore) != 2 {
		t.Fatalf("expected 2 skeleton markers before drive; got %d (%v)",
			len(markersBefore), restoretest.SortedKeySet(markersBefore))
	}

	// Step 1 — simulate `client-attached` for alpha by exec'ing the
	// production CLI argv. The hook's run-shell invokes exactly this
	// command. We pass only "alpha" so beta's marker remains set
	// afterwards, mirroring the production sequence where attaching to
	// alpha hydrates only alpha's panes.
	restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(),
		stateDir, hooksPath, []string{"alpha"}, env)

	// Wait for alpha's marker to clear, then assert beta's is still
	// pending. This sequencing is the structural distinction between
	// client-attached (single-session hydrate) and client-session-
	// changed (the next session's hydrate).
	waitForSessionMarkerCleared(t, client, "alpha", 10*time.Second)

	markersMid, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers (mid-drive): %v", err)
	}
	if len(markersMid) != 1 {
		t.Errorf("after alpha-only signal, expected 1 marker still set (beta); got %d (%v)",
			len(markersMid), restoretest.SortedKeySet(markersMid))
	}

	// Step 2 — simulate `client-session-changed` for beta. This is the
	// argv tmux's `client-session-changed` hook fires when an attached
	// client switches from alpha to beta via `tmux switch-client -t
	// beta`. Same binary, same subcommand, same per-session arg —
	// only the trigger event differs in production, which is opaque to
	// the binary and therefore not exercised differently here.
	restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(),
		stateDir, hooksPath, []string{"beta"}, env)

	// Both sessions must now have all skeleton markers cleared — the
	// two-step pipeline drove every restored pane to hydration
	// completion.
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second, 50*time.Millisecond)

	// Live structure must still be intact under the production-binary
	// drive path: alpha + beta both live, each with a single pane at
	// the default base-index 0 / pane-base-index 0 we exercise here.
	verifySwitchClientLiveStructure(t, ts)
}

// waitForSessionMarkerCleared polls until every skeleton marker whose
// paneKey belongs to session is gone. Used by the switch-client sub-
// test to assert sequencing — alpha's marker clears after step 1,
// while beta's stays set until step 2 fires.
//
// Why a session-scoped predicate (not WaitForSkeletonMarkersCleared):
// the latter waits for *all* markers to clear, which is the wrong
// barrier mid-sequence — the test deliberately drives one session at a
// time.
//
// Match shape: SanitizePaneKey produces `<sanitized>__<win>.<pane>`,
// so we test for the exact session prefix followed by the literal
// double-underscore separator. Substring matching against just the
// session name would false-positive on a hypothetical session like
// `alphabet` when watching `alpha`.
func waitForSessionMarkerCleared(t *testing.T, client *tmux.Client, session string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	prefix := session + "__"
	for time.Now().Before(deadline) {
		markers, err := state.ListSkeletonMarkers(client)
		if err != nil {
			t.Fatalf("ListSkeletonMarkers: %v", err)
		}
		stillSet := false
		for k := range markers {
			if strings.HasPrefix(k, prefix) {
				stillSet = true
				break
			}
		}
		if !stillSet {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	markers, _ := state.ListSkeletonMarkers(client)
	t.Fatalf("session %q skeleton markers still set after %s; markers=%v",
		session, timeout, restoretest.SortedKeySet(markers))
}

// verifySwitchClientLiveStructure asserts both alpha and beta sessions
// are live with a single window/pane each at default base-index 0 /
// pane-base-index 0. Mirrors the structural-equivalence check
// verifyLiveStructure makes in the primary round-trip, scoped to the
// trivial topology this sub-test uses.
func verifySwitchClientLiveStructure(t *testing.T, ts *tmuxtest.Socket) {
	t.Helper()
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("session %q missing post-hydrate; got %q", want, out)
		}
	}
	for _, sess := range []string{"alpha", "beta"} {
		panes := ts.Run(t, "list-panes", "-s", "-t", sess,
			"-F", "#{window_index}:#{pane_index}")
		if !strings.Contains(panes, "0:0") {
			t.Errorf("%s live pane 0:0 missing; got %q", sess, panes)
		}
	}
}

// TestRebootRoundTrip_LeadingDashSessionName is the integration regression
// guard for spec § "Acceptance Criteria" items 1 (the `--` separator) and
// 5 (legacy hook migration). It performs a full reboot round-trip on an
// isolated tmuxtest socket using a session whose name begins with `-`
// (`-dotfiles-test`), which is the failure mode the production fix targets.
//
// Why this test cannot be replaced by the alpha/beta round-trips:
//   - Existing TestPhase5RebootRoundTripEndToEnd / *BaseIndexDrift use
//     "alpha" and "beta" — neither begins with `-`. tmux's `run-shell`
//     never resolves a leading-dash session name into the hook command
//     for those tests, so a regression of either the `--` separator
//     (Task 1-1) or the migration eviction (Task 1-2) would not surface.
//   - This test wires PRODUCTION hook-registration adapters via
//     bootstrapadapter.HookRegistrar so the migration code from Task 1-2
//     and the `--` separator from Task 1-1 actually run.
//
// Coverage:
//   - For each event in HydrationTriggerEvents, exactly one entry contains
//     `portal state signal-hydrate` and contains the `-- ` separator.
//   - signal-hydrate exec'd via the built portal binary against the
//     leading-dash session reaches runSignalHydrate, writes the FIFO, and
//     unblocks the hydrate helper.
//   - All skeleton markers for the leading-dash session clear within the
//     budget.
//   - portal.log contains zero `hydrate timeout` WARN lines.
//   - ANSI scrollback fixture survives on the restored pane.
func TestRebootRoundTrip_LeadingDashSessionName(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	const sessionName = "-dotfiles-test"
	const saveBase, savePaneBase = 1, 1
	const restoreBase, restorePaneBase = 1, 1

	binDir := restoretest.BuildPortalBinaryDir(t)
	restoretest.PrependPATH(t, binDir)

	stateDir := newIntegrationStateDir(t)

	// Isolated baseline env for DriveSignalHydrateBinary subprocess
	// spawns — see Component G of
	// slow-open-empty-previews-and-zombie-sessions.
	env, _ := portaltest.IsolateStateForTest(t)

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	cwd := t.TempDir()

	ts := tmuxtest.New(t, "ptl-rt-leadingdash-")
	client := ts.Client()

	// Bootstrap with a `_seed` session so we can configure base-index
	// BEFORE creating the leading-dash session. The seed is underscore-
	// prefixed so capture excludes it (mirrors the daemon's
	// `_portal-saver` discipline).
	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)
	tmuxtest.ApplyBaseIndices(t, ts, saveBase, savePaneBase)

	// Create the leading-dash session. The positional `-s -dotfiles-test`
	// form may be rejected by tmux's argv parser if it interprets the
	// leading dash as a flag; fall back to `-s -- -dotfiles-test`. If
	// even that fails, abort with a clear diagnostic — the test cannot
	// silently degrade into a no-op against an absent session.
	createLeadingDashSession(t, ts, sessionName, cwd)
	ts.WaitForSession(t, sessionName, 2*time.Second)

	// Drive the save path inline (no daemon). This produces sessions.json
	// + per-pane scrollback files exactly as production save would.
	idx := runDaemonTick(t, client, stateDir, withoutSkipGuard(), withEmptyScrollback())
	if got := len(idx.Sessions); got != 1 {
		t.Fatalf("captured %d sessions; want 1", got)
	}
	if idx.Sessions[0].Name != sessionName {
		t.Fatalf("captured session name = %q; want %q", idx.Sessions[0].Name, sessionName)
	}

	// Override the pane's scrollback file with a known ANSI fixture so
	// post-restore verification is deterministic (capture-pane output is
	// timing- and terminal-dependent otherwise).
	ansiFixture := []byte("\x1b[31mred\x1b[0m\nbefore reboot\n")
	paneKey := state.SanitizePaneKey(sessionName, saveBase+0, savePaneBase+0)
	scrollbackPath := state.ScrollbackFile(stateDir, paneKey)
	if err := os.WriteFile(scrollbackPath, ansiFixture, 0o600); err != nil {
		t.Fatalf("write fixture scrollback: %v", err)
	}

	// Kill the server. The next list-sessions MUST error so we know
	// Restore operates on a fresh server.
	ts.KillServer()
	if _, err := ts.TryRun("list-sessions"); err == nil {
		t.Fatalf("list-sessions succeeded after kill-server; expected error")
	}

	// Restart with `_seed` and apply restore-time base indices.
	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)
	tmuxtest.ApplyBaseIndices(t, ts, restoreBase, restorePaneBase)

	logger := restoretest.OpenTestLogger(t, stateDir)

	// Wire PRODUCTION hook-registration adapters this time — this is the
	// load-bearing difference vs the existing alpha/beta round-trips.
	// HookRegistrar runs migrateHydrationHooks (Task 1-2) and registers
	// the new `--`-separated signalHydrateCommand (Task 1-1) end-to-end.
	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Hooks:   &bootstrapadapter.HookRegistrar{Client: client, Logger: logger},
		Restore: bootstrapadapter.NewRestoreAdapter(client, stateDir, logger),
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		// Opt out of the integration builder's real-EagerSignaler default
		// (task 4-2): this round-trip drives signal-hydrate via the
		// production binary (DriveSignalHydrateBinary) AFTER Run returns
		// — the same manual-harness pattern as runRebootRoundTrip's
		// alpha/beta variant. With the real eager signaler firing during
		// Run, the helper would already have consumed its FIFO byte
		// before the binary-driven driver runs, defeating the test's
		// goal of exercising the registered hook → binary argv path.
		EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{},
	})
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	// Hook table assertion — for each hydration-trigger event there must
	// be exactly one entry containing `portal state signal-hydrate` AND
	// that entry must contain the `-- ` end-of-flags separator. Catches
	// both Task 1-1 (separator) and Task 1-2 (migration evicted any
	// stale un-separated entry; only one entry remains per event).
	verifyHydrationHookEntries(t, client)

	// Drive signal-hydrate via the built portal binary — argv-identical
	// to what the registered tmux client-attached hook fires via
	// run-shell. DriveSignalHydrateBinary wraps the session arg with `--`
	// internally so the leading-dash session reaches runSignalHydrate.
	restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(),
		stateDir, hooksPath, []string{sessionName}, env)

	// Wait for every helper's @portal-skeleton-<paneKey> marker to
	// clear. A timeout means the helper either crashed before unsetting
	// its marker, or never reached its open(O_RDONLY) — both regressions
	// we want to catch.
	restoretest.WaitForSkeletonMarkersCleared(t, client, 10*time.Second, 50*time.Millisecond)

	// portal.log must contain ZERO `hydrate timeout` WARN lines for any
	// pane in the leading-dash session. A regression of Task 1-1 or 1-2
	// would leave the helper waiting on the FIFO until its own budget
	// expires, emitting the WARN line — this assertion is the spec's
	// observable signal that the fix held end-to-end.
	verifyNoHydrateTimeoutWarns(t, filepath.Join(stateDir, "portal.log"), sessionName)

	// ANSI scrollback bytes must survive: capture-pane -e -p -S - against
	// the live restored pane should contain the red SGR sequence and the
	// literal "before reboot" line we wrote pre-kill.
	verifyANSIScrollback(t, ts, sessionName, restoreBase+0, restorePaneBase+0)
}

// createLeadingDashSession creates a session whose name begins with `-`.
// It tries the positional `-s <name>` form first; on failure it falls back
// to the `-s -- <name>` end-of-flags form. If both shapes fail the test
// is fatalled — silently skipping would leave the round-trip a no-op.
func createLeadingDashSession(t *testing.T, ts *tmuxtest.Socket, name, cwd string) {
	t.Helper()
	if _, err := ts.TryRun("new-session", "-d", "-s", name, "-c", cwd, "sleep", "infinity"); err == nil {
		return
	}
	if _, err := ts.TryRun("new-session", "-d", "-s", "--", name, "-c", cwd, "sleep", "infinity"); err == nil {
		return
	}
	t.Fatalf("could not create leading-dash session %q via positional or `--` form; tmux CLI rejected both shapes", name)
}

// verifyHydrationHookEntries asserts that for each event in
// HydrationTriggerEvents, the live tmux server's global hook table contains
// exactly one entry whose body contains `portal state signal-hydrate`, and
// that entry also contains the `-- ` end-of-flags separator.
//
// This is the structural assertion that proves both Task 1-1 (separator
// present in the new entry) and Task 1-2 (migration evicted any pre-
// existing un-separated entry, leaving exactly one entry) held under the
// production HookRegistrar adapter wiring.
func verifyHydrationHookEntries(t *testing.T, client *tmux.Client) {
	t.Helper()
	raw, err := client.ShowGlobalHooks()
	if err != nil {
		t.Fatalf("ShowGlobalHooks: %v", err)
	}
	parsed := tmux.ParseShowHooks(raw)
	for _, event := range tmux.HydrationTriggerEvents {
		entries := parsed[event]
		var matching []string
		for _, e := range entries {
			if strings.Contains(e.Command, "portal state signal-hydrate") {
				matching = append(matching, e.Command)
			}
		}
		if len(matching) != 1 {
			t.Errorf("event %q: %d entries contain `portal state signal-hydrate`; want 1\nentries:\n%s",
				event, len(matching), strings.Join(matching, "\n"))
			continue
		}
		if !strings.Contains(matching[0], "portal state signal-hydrate -- ") {
			t.Errorf("event %q: entry missing `-- ` separator before #{session_name}; got %q",
				event, matching[0])
		}
	}
}

// verifyNoHydrateTimeoutWarns reads portal.log and asserts no line
// contains `hydrate timeout` referencing a pane in session. Substring
// matching against the literal `hydrate timeout` is appropriate — the
// state package emits the canonical phrase verbatim, and a leading-dash
// session can only appear in a pane key built from its own name.
//
// Returns silently on a missing log file: an absent log means no WARN
// was ever emitted, which is the success state.
func verifyNoHydrateTimeoutWarns(t *testing.T, logPath, session string) {
	t.Helper()
	// SanitizePaneKey produces `<sanitized>__<win>.<pane>`; the
	// sanitized form of "-dotfiles-test" preserves the dashes (only
	// path-illegal characters are replaced). We match on the session
	// substring rather than reconstructing the exact pane key so a
	// regression in pane-key formatting still surfaces as a failure.
	pred := func(line string) bool {
		return strings.Contains(line, "hydrate timeout") && strings.Contains(line, session)
	}
	assertNoLogLineMatches(t, logPath, pred,
		"portal.log contains `hydrate timeout` WARN for session %q: %s", session)
}
