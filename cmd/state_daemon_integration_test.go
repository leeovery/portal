package cmd_test

// Real-tmux integration test pinning Defect 2's daemon-responsiveness
// contract from the saver-kill-respawn-loop-leaks-daemons spec
// (specification.md § Acceptance Criteria → Daemon responsiveness; §
// Testing Requirements → Integration tests #2). This file is the end-
// to-end verification that a daemon mid-tick exits within "one pane's
// capture-pane wall time" after receiving SIGHUP — the structural
// contract Change 2 (ctx-aware captureAndCommit) was introduced to
// uphold.
//
// Why this test exists despite the unit-level coverage in
// state_daemon_run_test.go (Tasks 2-2 / 2-3 / 2-4): those tests
// exercise the cancellation observation points against in-process
// fakes. They prove the ctx-check is wired and observed at the three
// load-bearing points inside captureAndCommit, but they cannot model
// the full process lifecycle — signal delivery, signal.Notify
// goroutine plumbing, the cancel() → daemonRunFunc → return →
// daemonShutdownFunc chain, OS-level subprocess exit, or the actual
// wall-time cost of a real tmux capture-pane invocation against a
// representative scrollback fixture. This test pins all of those
// against a real binary spawned as a subprocess and a real tmux server
// hosting many-pane / many-MB scrollback.
//
// Skip behaviour mirrors the saver-side integration test in
// internal/tmux/portal_saver_integration_test.go:
//
//   - tmuxtest.SkipIfNoTmux skips when tmux is not on PATH.
//   - portalbintest.BuildPortalBinary failure is a clean Skip (not a
//     Fatal) — a dev machine without `go` or a broken build is not a
//     responsiveness-contract failure.
//
// No t.Parallel: the cmd-package convention (mock-injection via
// package-level mutable state cleaned up by t.Cleanup) applies here
// even though this test exercises a subprocess rather than in-process
// seams. t.Parallel is forbidden across the portal test suite.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// daemonAlivePollInterval is the cadence at which the test polls
// state.DaemonAlive while waiting for the freshly-spawned daemon to
// write its pidfile. 50ms is short enough to absorb sub-second cold-
// start jitter without busy-spinning.
const daemonAlivePollInterval = 50 * time.Millisecond

// daemonAliveTimeout is the ceiling on how long the test waits for
// the daemon subprocess to publish a live daemon.pid. 5s comfortably
// absorbs `go build` cold-start variability and CI scheduling jitter.
const daemonAliveTimeout = 5 * time.Second

// tickStartDelay is how long the test sleeps after touching
// save.requested to give the daemon's 1s ticker time to fire and the
// captureAndCommit loop time to enter the per-pane phase. The daemon's
// TickerPeriod is 1 * time.Second (see cmd/state_daemon.go); 1.2s
// covers the worst case of "save.requested arrives just after a tick
// fire" → "next tick fire 1s later" → "small margin to enter the
// per-pane loop". Larger margins risk the tick completing before
// SIGHUP arrives on machines with fast capture-pane wall time —
// because we set @portal-restoring AFTER this sleep, the worst-case
// failure mode is the tick completing the full per-tick aggregate
// (~aggregatePerTickEstimate) before SIGHUP, exposing the shutdown-
// flush-skip path rather than the per-pane-loop cancellation path.
// The 1.2s value keeps SIGHUP inside the [tick-fire, tick-fire +
// per-pane-wall-time] window with margin on both sides.
const tickStartDelay = 1200 * time.Millisecond

// panePopulationTimeout is the ceiling on per-pane synthetic-
// scrollback population. Each pane runs `sh -c 'seq 1 N; sleep
// infinity'` so the seq output deterministically lands in tmux's
// scrollback; we poll capture-pane line count until ≥ N. 10s absorbs
// shell startup + seq output rendering on CI hardware.
const panePopulationTimeout = 10 * time.Second

// panePopulationPollInterval is the cadence at which the test polls
// capture-pane line count while waiting for the synthetic seq output
// to fully render into the pane's scrollback buffer.
const panePopulationPollInterval = 100 * time.Millisecond

// paneCount is the number of panes loaded with synthetic scrollback.
// The task recommends "~8". 12 is chosen instead to give the pre-fix
// (no observation-point-3) regression latency clear headroom above
// the anchored 2s threshold: with singlePaneWallTime ≈ 0.3s on the
// development darwin host, 12 panes yield an aggregate per-tick wall
// time of ~3.6s. A SIGHUP arriving anywhere in the first ~1.6s of
// the tick still leaves ≥ 2s of remaining synchronous work — enough
// to push the pre-fix latency past the anchored threshold and surface
// a Change-2 regression as a clear failure rather than a borderline
// flake. 12 stays under 200/12 ≈ 16 cols/pane, which fits the tmux
// minimum pane width without triggering "pane too small" errors.
const paneCount = 12

// scrollbackLines is the per-pane seq upper bound. 500000 lines × ~7
// bytes/line ≈ 3.5MB of rendered text per pane (the tmux capture-pane
// -e -p -S - traversal is O(lines × cells), with ANSI escape rendering
// dominating wall time). Empirically this yields single-pane capture-
// pane wall time around 300ms on a development darwin host; aggregated
// across paneCount panes that reliably pushes a single tick above the
// 2s heuristic the spec calls out.
//
// On a slow machine where the single-pane capture wall time exceeds
// 2.5s (which would push the derived threshold past 5s), the test
// halves this value and re-measures — see the re-measurement loop in
// the test body.
const scrollbackLines = 500000

// historyLimit is set on the test's tmux server before any session is
// created so the seq output above is not truncated. tmux's default of
// 2000 is far below scrollbackLines.
const historyLimit = 1000000

// TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow is the real-tmux
// integration test gating spec § Acceptance Criteria → Daemon
// responsiveness (criterion 7) and § Testing Requirements →
// Integration tests #2. It exercises the full daemon process lifecycle
// against a real tmux server hosting many-pane / many-MB synthetic
// scrollback, then asserts that SIGHUP arriving mid-tick causes the
// daemon to exit within both (a) the anchored threshold derived from a
// fresh single-pane capture-pane measurement and (b) the 5s
// killBarrierTimeout ceiling.
//
// Flow:
//
//  1. Skip if tmux is not on PATH.
//  2. Skip if the portal binary cannot be built.
//  3. Stand up an isolated tmux server via tmuxtest.New.
//  4. Set history-limit high enough to retain scrollbackLines lines.
//  5. Create paneCount panes via new-session + split-window, each
//     running `sh -c 'seq 1 N; sleep infinity'` so the seq output
//     deterministically lands in scrollback.
//  6. Wait for each pane's capture-pane output to contain ≥ N lines.
//  7. Measure singlePaneWallTime by timing one capture-pane -e -p -S -
//     invocation against the first pane. Anchor threshold = max(2s,
//     2 × singlePaneWallTime). If derived threshold > 5s, halve N and
//     re-load all panes (one retry max).
//  8. Launch `portal state daemon` as a subprocess with TMUX env
//     pointing at the test socket and PORTAL_STATE_DIR pointing at the
//     test temp dir.
//  9. Wait until state.DaemonAlive reports true.
//  10. Trigger a tick by writing state.SaveRequested(dir).
//  11. Sleep tickStartDelay (1.2s) so the daemon's 1s ticker fires and
//     captureAndCommit enters the per-pane loop.
//  12. Capture tickStart, send SIGHUP, wait for Process.Wait via a
//     polling goroutine bounded by tmux.KillBarrierTimeoutCeiling.
//  13. Assert exit happened within the anchored threshold AND within
//     the 5s ceiling. Assert exit status zero (clean shutdown).
//
// On failure: the daemon process is SIGKILL'd in t.Cleanup with a
// descriptive message so a leaking daemon does not corrupt later
// fixtures.
func TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// Build a portal binary and PATH-prepend so the subprocess spawn
	// resolves the `portal` argv-0 cleanly. We invoke the binary by
	// absolute path below, but PATH prepend is retained so any internal
	// re-exec (none today, defensive) finds the same build.
	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	stateDir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-daemon-sighup-")

	// Bring the tmux server up by creating a throwaway session, then
	// raise the global history-limit BEFORE the throwaway is killed
	// and BEFORE the real perf session is created in populatePanes.
	// tmux's set-option -g must run against a live server (no implicit
	// start), and history-limit is captured at session-creation time
	// for that session's panes — so applying it globally first is the
	// only way to keep scrollback for sessions created later in
	// populatePanes.
	bootstrapTmuxServer(t, sock)

	// Step 5/6 (and re-measurement on slow hardware): populate the
	// panes with synthetic scrollback and measure single-pane wall
	// time. If the derived threshold exceeds tmux.KillBarrierTimeoutCeiling
	// we halve scrollbackLines and try again (max one retry).
	//
	// The measurement-anchored threshold contract from spec § Testing
	// Requirements → Integration tests #2: "Implementation should take
	// that measurement and either confirm 2s as appropriate or adjust
	// the threshold from the measurement." The measurement is recorded
	// as singlePaneWallTime and the test fails if either the anchored
	// threshold or the unconditional 5s ceiling is exceeded.
	lines := scrollbackLines
	var singlePaneWallTime time.Duration
	var threshold time.Duration
	for attempt := range 2 {
		// On retry, recreate the server so panes start from scratch
		// without inherited buffers. The bootstrap helper re-creates
		// the throwaway session and re-applies history-limit globally
		// before populatePanes spawns fresh sessions.
		if attempt > 0 {
			sock.KillServer()
			bootstrapTmuxServer(t, sock)
		}
		populatePanes(t, sock, lines)
		singlePaneWallTime = measureSinglePaneCapture(t, sock)
		threshold = anchorThreshold(singlePaneWallTime)
		if threshold <= tmux.KillBarrierTimeoutCeiling {
			break
		}
		// Halve and re-measure. If still over after one retry the test
		// will fall through with a derived threshold > 5s and the
		// assertions will surface that as a failure rather than masking
		// a slow-machine condition silently.
		lines /= 2
	}
	if threshold > tmux.KillBarrierTimeoutCeiling {
		t.Fatalf("derived threshold %s exceeds killBarrierTimeout ceiling %s after halving "+
			"per-pane scrollback to %d lines (singlePaneWallTime=%s); "+
			"test fixture cannot establish a meaningful responsiveness window on this hardware",
			threshold, tmux.KillBarrierTimeoutCeiling, lines, singlePaneWallTime)
	}

	// Sanity-check the aggregate per-tick wall time: the test only
	// meaningfully exercises Defect 2's contract if a full tick would
	// clearly exceed 2s. paneCount * singlePaneWallTime is the lower
	// bound on aggregate per-tick wall time (ignoring per-iteration
	// fixed costs).
	aggregate := time.Duration(paneCount) * singlePaneWallTime
	t.Logf("measurement: singlePaneWallTime=%s, aggregatePerTickEstimate=%s, anchoredThreshold=%s, "+
		"scrollbackLinesPerPane=%d", singlePaneWallTime, aggregate, threshold, lines)
	if aggregate < 2*time.Second {
		t.Skipf("skipping: aggregate per-tick wall time (%s) is below the 2s heuristic; "+
			"this host's capture-pane is too fast to exercise the tick-spans-kill-barrier "+
			"cancellation path (anchoredThreshold=%s)",
			aggregate, threshold)
	}

	// Step 8: launch the daemon subprocess. TMUX env points the
	// daemon's tmux.DefaultClient at the test socket; PORTAL_STATE_DIR
	// pins state.EnsureDir to the test temp dir.
	daemon := exec.Command(binary, "state", "daemon")
	daemon.Env = append(os.Environ(),
		fmt.Sprintf("TMUX=%s,1,0", sock.SocketPath()),
		"PORTAL_STATE_DIR="+stateDir,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		// PORTAL_LOG_LEVEL=DEBUG surfaces the daemon's tick / capture /
		// cancellation entries into portal.log so the post-test
		// diagnostic dump can confirm whether SIGHUP landed mid-tick or
		// in the idle select window. Without this the daemon's default
		// LevelWarn writes nothing for the happy path and the diagnostic
		// dump is empty.
		"PORTAL_LOG_LEVEL=DEBUG",
	)
	// Capture stdout/stderr so the failure surface includes the daemon's
	// own diagnostic output if anything goes wrong. The CombinedOutput
	// API can't be used because we need to send signals while reading;
	// pipe instead and read after Wait.
	var stderr strings.Builder
	daemon.Stderr = &stderr
	if err := daemon.Start(); err != nil {
		t.Fatalf("start portal state daemon: %v", err)
	}

	// Cleanup: ensure the daemon process never leaks past the test
	// regardless of pass/fail/panic. SIGKILL if the test body left it
	// alive (e.g. assertion failed before SIGHUP arrival).
	t.Cleanup(func() {
		if daemon.Process == nil {
			return
		}
		// ProcessState non-nil means Wait already returned — nothing
		// to clean up.
		if daemon.ProcessState != nil {
			return
		}
		if err := daemon.Process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("leaking daemon: SIGKILL failed: %v", err)
		}
		_, _ = daemon.Process.Wait()
		t.Errorf("daemon process leaked past test end and was SIGKILL'd; pid=%d", daemon.Process.Pid)
	})

	// Step 9: wait for DaemonAlive. The daemon publishes daemon.pid
	// asynchronously after acquiring the lock and writing the version
	// file (see cmd/state_daemon.go).
	waitForDaemonAlive(t, stateDir, daemonAliveTimeout)

	// Step 10: trigger a tick. The dirty flag clears the no-op fast
	// path in tick() so the next ticker fire enters captureAndCommit.
	if err := os.WriteFile(state.SaveRequested(stateDir), nil, 0o644); err != nil {
		t.Fatalf("touch save.requested: %v", err)
	}

	// Step 11: sleep tickStartDelay (1.2s) so the daemon's 1s ticker
	// has fired ONCE and captureAndCommit has just entered the per-
	// pane loop. The static-sleep shape is deliberate: anchoring SIGHUP
	// to a fast post-tick-start moment (rather than waiting for a
	// scrollback file to appear) maximises the remaining-tick budget
	// the pre-fix synchronous loop would have to grind through before
	// returning. The longer the in-flight tick when SIGHUP arrives,
	// the larger the pre-fix latency, and the more sharply the
	// anchored threshold catches a Change-2 regression.
	//
	// Worst-case timing analysis (daemon-start at t=0):
	//   - waitForDaemonAlive returns at t≈0.05-0.5s (depends on cold-
	//     cache `go build` + flock acquisition).
	//   - save.requested written immediately after.
	//   - SIGHUP fired at (waitAlive return) + tickStartDelay.
	//   - Daemon ticker fires at t=1s, 2s, 3s, ... (1s period).
	//   - First tick that observes dirty=true enters captureAndCommit.
	//
	// tickStartDelay = 1.2s pins SIGHUP to roughly t=1.25-1.7s of
	// daemon time, which is at most ~0.7s into the first tick and at
	// least ~0.25s past it — well inside the captureAndCommit per-pane
	// loop on the test fixture. Pre-fix this gives a remaining-tick
	// budget of ~2s+ to grind through; post-fix the loop exits at the
	// next observation point within one pane's wall time.
	time.Sleep(tickStartDelay)

	// Pre-SIGHUP filesystem state, captured for the test log so the
	// diagnostic dump on failure (or post-mortem inspection on pass)
	// shows whether the tick was mid-flight. save.requested still
	// present at this point means the tick has not yet completed (a
	// successful capture-commit cycle removes the dirty flag). This is
	// an informational signal — NOT a guard assertion — because a
	// future change that increases tick speed could flip this without
	// invalidating the latency contract.
	_, saveReqStatErr := os.Stat(state.SaveRequested(stateDir))
	t.Logf("pre-SIGHUP: save.requested exists=%v", saveReqStatErr == nil)

	// Step 11.5: set @portal-restoring on the server so the daemon's
	// defaultShutdownFlush skips the post-cancel final flush. Without
	// this, the daemon's exit chain on SIGHUP is:
	//
	//   captureAndCommit (cancellable) → ctx.Done() observed →
	//   daemonShutdownFunc → captureAndCommit (NON-cancellable, full
	//   per-tick aggregate)
	//
	// which would push exit latency to roughly 2× the per-tick
	// aggregate even with Change 2 fully wired. In production, the
	// kill-respawn cascade always sets @portal-restoring (bootstrap
	// step 3) before EnsureSaver issues the kill, so the shutdown-
	// flush-skip branch is the load-bearing path the spec's "bounded
	// by one pane's capture wall time" criterion describes. The
	// integration test mirrors that condition explicitly.
	//
	// Set via the same socketCommander the test's Client uses so the
	// option lands on the test's isolated server, not the user's.
	sock.Run(t, "set-option", "-s", state.RestoringMarkerName, "1")

	// Step 12: SIGHUP arrives mid-tick. tickStart is captured before
	// signal delivery so the exit-latency window includes signal
	// propagation + signal.Notify goroutine dispatch + cancel() + the
	// observed ctx.Done() at the next per-pane iteration.
	tickStart := time.Now()
	if err := daemon.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("send SIGHUP to daemon (pid=%d): %v", daemon.Process.Pid, err)
	}

	// exec.Cmd.Wait does not accept a deadline; run it on a goroutine
	// and bound the test's worst-case hang to
	// tmux.KillBarrierTimeoutCeiling. cmd.Wait (NOT cmd.Process.Wait) is the
	// load-bearing call here: only cmd.Wait populates cmd.ProcessState,
	// which the clean-exit assertion below reads. Calling
	// cmd.Process.Wait directly reaps the zombie but leaves
	// cmd.ProcessState nil, which would (a) flake the clean-exit
	// assertion and (b) make the t.Cleanup branch that gates on
	// ProcessState non-nil falsely conclude the process leaked.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- daemon.Wait()
	}()

	// Wait for either daemon exit or the 5s ceiling. Sample exitTime
	// immediately on the received signal to keep the latency
	// measurement tight.
	var exitErr error
	var exitTime time.Time
	deadline := time.NewTimer(tmux.KillBarrierTimeoutCeiling + 500*time.Millisecond)
	defer deadline.Stop()
	select {
	case exitErr = <-waitDone:
		exitTime = time.Now()
	case <-deadline.C:
		// The daemon has not exited within the ceiling + a small
		// margin. Fail hard with diagnostics; the t.Cleanup will
		// SIGKILL it.
		t.Fatalf("daemon did not exit within %s of SIGHUP (pid=%d); singlePaneWallTime=%s, "+
			"anchoredThreshold=%s\n--- daemon stderr ---\n%s",
			tmux.KillBarrierTimeoutCeiling+500*time.Millisecond, daemon.Process.Pid,
			singlePaneWallTime, threshold, stderr.String())
	}

	latency := exitTime.Sub(tickStart)
	t.Logf("daemon exit latency: %s (anchoredThreshold=%s, ceiling=%s, singlePaneWallTime=%s)",
		latency, threshold, tmux.KillBarrierTimeoutCeiling, singlePaneWallTime)

	// Post-exit diagnostics. The portal.log dump shows the daemon's
	// observed shutdown chain; the scrollback file count proves whether
	// the per-pane loop entered execution (count > 0 → mid-tick SIGHUP
	// was the load-bearing scenario the test exercised). On a passing
	// run this is informational only; on a regression it pinpoints the
	// failure mode (no scrollback files → daemon never entered tick;
	// all scrollback files → ctx-cancel was ignored and full tick ran).
	if data, derr := os.ReadFile(state.PortalLog(stateDir)); derr == nil {
		t.Logf("--- portal.log ---\n%s", string(data))
	}
	if entries, derr := os.ReadDir(state.ScrollbackDir(stateDir)); derr == nil {
		t.Logf("post-exit scrollback file count: %d (paneCount=%d)", len(entries), paneCount)
	}

	// Assertion 1: latency under the anchored measurement-derived
	// threshold. This is the load-bearing assertion for spec § Daemon
	// responsiveness — pre-fix, captureAndCommit would synchronously
	// iterate every remaining pane before returning to the outer ctx-
	// check, so a daemon mid-tick would block at least (paneCount-1) *
	// singlePaneWallTime before exiting.
	if latency >= threshold {
		t.Errorf("daemon exit latency %s >= anchored threshold %s "+
			"(singlePaneWallTime=%s, ceiling=%s)\n--- daemon stderr ---\n%s",
			latency, threshold, singlePaneWallTime, tmux.KillBarrierTimeoutCeiling, stderr.String())
	}

	// Assertion 2: latency under the production killBarrierTimeout
	// (5s). The spec mandates this ceiling stays unchanged; this
	// assertion is the regression guard against "the ctx-aware loop
	// was reverted" or "a future change reintroduced a synchronous
	// tick".
	if latency >= tmux.KillBarrierTimeoutCeiling {
		t.Errorf("daemon exit latency %s >= killBarrierTimeout ceiling %s "+
			"(singlePaneWallTime=%s)\n--- daemon stderr ---\n%s",
			latency, tmux.KillBarrierTimeoutCeiling, singlePaneWallTime, stderr.String())
	}

	// Assertion 3: clean exit. SIGHUP must take the daemon through
	// the cancel() → daemonRunFunc return → daemonShutdownFunc chain
	// and emerge with exit status 0. Any non-zero exit indicates the
	// shutdown path encountered an error worth surfacing.
	if exitErr != nil {
		t.Errorf("daemon exited non-zero after SIGHUP: %v\n--- daemon stderr ---\n%s",
			exitErr, stderr.String())
	}
	if daemon.ProcessState == nil || !daemon.ProcessState.Success() {
		state := "<nil>"
		if daemon.ProcessState != nil {
			state = daemon.ProcessState.String()
		}
		t.Errorf("daemon ProcessState not successful: %s\n--- daemon stderr ---\n%s",
			state, stderr.String())
	}
}

// populatePanes creates paneCount panes on the test socket, each
// running `sh -c 'seq 1 N; sleep infinity'` so the seq output
// deterministically lands in scrollback. Layout: one session ("perf")
// with paneCount panes split horizontally. After spawning, the helper
// polls each pane's capture-pane line count until ≥ N (or
// panePopulationTimeout elapses).
//
// The sleep-infinity tail keeps the pane alive after seq completes so
// tmux's capture-pane has stable scrollback to read.
func populatePanes(t *testing.T, sock *tmuxtest.Socket, lines int) {
	t.Helper()

	cmd := fmt.Sprintf("seq 1 %d; sleep infinity", lines)

	// Create the session with the first pane already running the seq
	// generator. -d is required so new-session does not try to attach
	// (no controlling terminal in `go test`).
	sock.Run(t, "new-session", "-d", "-s", "perf", "-x", "200", "-y", "50",
		"sh", "-c", cmd)

	// Split paneCount-1 more times in the same window. Use -h for
	// horizontal splits; the 200x50 base geometry gives ~25 columns
	// per pane at paneCount=8 which is enough for seq's output to
	// render without wrap distortion of line counts.
	for i := 1; i < paneCount; i++ {
		sock.Run(t, "split-window", "-h", "-t", "perf:0",
			"sh", "-c", cmd)
		// Re-balance after each split so subsequent splits do not hit
		// "pane too small". even-horizontal layout distributes width
		// evenly across all current panes.
		sock.Run(t, "select-layout", "-t", "perf:0", "even-horizontal")
	}

	// Poll each pane until capture-pane reports ≥ lines records. seq
	// runs to completion synchronously inside the spawned sh; the
	// rendering happens at sh's stdout rate. Polling on the line count
	// gives a deterministic readiness signal that survives shell-output
	// rate variability across hosts.
	deadline := time.Now().Add(panePopulationTimeout)
	for i := range paneCount {
		target := fmt.Sprintf("perf:0.%d", i)
		for {
			if time.Now().After(deadline) {
				t.Fatalf("pane %s did not accumulate %d scrollback lines within %s",
					target, lines, panePopulationTimeout)
			}
			out := sock.Run(t, "capture-pane", "-p", "-t", target, "-S", "-")
			// Count newline-terminated records. The final line may not
			// terminate with a newline (sleep infinity is rendering an
			// empty cursor line); strings.Count of '\n' is therefore a
			// lower bound, which is exactly what we want for the
			// readiness gate.
			if strings.Count(out, "\n") >= lines {
				break
			}
			time.Sleep(panePopulationPollInterval)
		}
	}
}

// measureSinglePaneCapture times one `capture-pane -e -p -S -`
// invocation against perf:0.0 — the same argv shape (modulo target)
// that state.CaptureAndHashPane uses inside captureAndCommit's per-
// pane loop. The measurement is the test's anchor for deriving a
// realistic latency threshold from the actual fixture cost on the
// host machine, rather than a hard-coded heuristic that would either
// over-fail on slow hardware or under-fail on fast hardware.
func measureSinglePaneCapture(t *testing.T, sock *tmuxtest.Socket) time.Duration {
	t.Helper()
	start := time.Now()
	// -e to render ANSI escapes (matches CaptureAndHashPane); -p to
	// stdout; -S - for the unbounded-history capture. The output is
	// discarded — only wall time is consumed here.
	_ = sock.Run(t, "capture-pane", "-e", "-p", "-t", "perf:0.0", "-S", "-")
	return time.Since(start)
}

// anchorThreshold derives the measurement-anchored exit-latency
// threshold from a fresh single-pane capture-pane wall time: a minimum
// floor of 2s, scaling up to 2 × singlePaneWallTime once the measured
// per-pane wall time exceeds 1s. The 2s floor comes from the heuristic
// anchor in spec § Testing Requirements → Integration tests #2
// ("target: under 2s on the test fixture"); the 2× multiplier gives the
// daemon enough margin to absorb signal delivery + ctx-Done observation
// overhead without flaking on hosts where capture-pane is already slow.
func anchorThreshold(singlePaneWallTime time.Duration) time.Duration {
	doubled := 2 * singlePaneWallTime
	if doubled < 2*time.Second {
		return 2 * time.Second
	}
	return doubled
}

// waitForDaemonAlive polls state.DaemonAlive until it reports true or
// the timeout elapses. The daemon subprocess writes daemon.pid
// asynchronously after acquiring the singleton lock; callers cannot
// race on Process.Pid alone because that PID is the daemon binary's
// pid, which exists before the pidfile is written.
func waitForDaemonAlive(t *testing.T, stateDir string, timeout time.Duration) {
	t.Helper()
	if tmuxtest.PollUntil(t, timeout, daemonAlivePollInterval, func() bool {
		return state.DaemonAlive(stateDir)
	}) {
		return
	}
	// On timeout, surface any portal.log output the daemon wrote so
	// startup failures (lock contention, ensure-dir errors, etc.) are
	// visible in the diagnostic.
	var logBlob string
	if data, err := os.ReadFile(state.PortalLog(stateDir)); err == nil {
		logBlob = string(data)
	}
	t.Fatalf("daemon did not become alive within %s (stateDir=%s)\n--- portal.log ---\n%s",
		timeout, stateDir, logBlob)
}

// bootstrapTmuxServer brings the test's tmux server up by creating an
// anchor session, then applies the global history-limit option so
// every session created subsequently inherits the larger buffer.
//
// tmux's set-option -g requires a live server (no implicit-start
// semantics), and history-limit is read at session-creation time. The
// anchor session is intentionally left alive: killing it would also
// kill the server (no remaining sessions → server exits), so the perf
// session populated later would start against a fresh server with
// default history-limit. The anchor pane's `sleep infinity` produces
// no scrollback so its capture cost is negligible against the
// daemon's per-tick aggregate; CaptureStructure simply enumerates one
// extra trivial pane.
func bootstrapTmuxServer(t *testing.T, sock *tmuxtest.Socket) {
	t.Helper()
	sock.Run(t, "new-session", "-d", "-s", "_anchor", "sh", "-c", "sleep infinity")
	sock.Run(t, "set-option", "-g", "history-limit", strconv.Itoa(historyLimit))
}
