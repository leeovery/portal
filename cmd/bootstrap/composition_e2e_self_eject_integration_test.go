//go:build integration

// Composite end-to-end Component D self-eject test for spec § "Composite
// End-to-End Verification" bullet 8 — task 6-6.
//
// Consumes the shared compositeHarness (3-daemon pre-state: legitimate
// saver-pane daemon + 2 orphans; legitimate stateDir's daemon.pid
// references orphan1). Invokes the same production bootstrap slice as
// 6-3 / 6-4 / 6-5 (`SweepOrphanDaemons` + `BootstrapPortalSaver`),
// waits for pgrep convergence to 1, then exercises Component D's
// per-tick saver-membership self-check in the LIVE composite context
// by inducing a pane-pid mismatch WITHOUT killing the legitimate
// daemon process. Asserts the daemon self-ejects via `os.Exit(0)`
// within `(selfSupervisionHysteresisTicks + 1) * TickerPeriod + slack`,
// and that the eject is observably consistent with the spec contract
// (no scrollback delta across the eject window; stale daemon.pid
// retained; the self-supervision INFO marker reaches portal.log).
//
// External mismatch mechanism — `tmux new-window`:
//
//   The plan brief calls for `tmux break-pane` (or `move-pane` fallback)
//   to move the daemon pane out of `_portal-saver` into a sibling session,
//   leaving a placeholder pane behind so the saver session persists and its
//   pane_pid no longer matches the daemon's pid. The simpler, structurally
//   equivalent mechanism this test uses: `tmux new-window -t _portal-saver:`
//   with a placeholder shell command. The default tmux behaviour for
//   new-window is to make the new window the SESSION'S ACTIVE WINDOW;
//   `tmux.SaverPanePIDOrAbsent` runs `list-panes -t =_portal-saver -F
//   '#{pane_pid}'` which (without `-s`) enumerates panes in the SESSION'S
//   ACTIVE WINDOW. After new-window the active window's pane is the
//   placeholder, so `SaverPanePIDOrAbsent(c, "_portal-saver")` returns the
//   placeholder PID with present=true — not
//   the daemon's PID — which is exactly the pid-mismatch the daemon's
//   per-tick probe is designed to detect. The daemon process itself is
//   still alive in the original window 0; the saver session is still
//   present; `_portal-saver`'s window count grew from 1 to 2.
//
//   Why not `respawn-pane -k`: it would SIGKILL the daemon process directly,
//   defeating the `os.Exit(0)` self-eject path under test. The plan brief
//   explicitly forbids this mechanism.
//
//   Why not `kill-session _portal-saver`: tmux delivers SIGHUP to the
//   saver pane process (= the daemon). The daemon's signal handler then
//   enters the defaultShutdownFlush path, which violates the spec's
//   "no final flush on self-eject" invariant under test.
//
//   Why not `break-pane`: structurally identical effect (daemon ends up in
//   a sibling session, saver's active pane becomes a placeholder) but
//   requires pre-creating a destination session AND pre-creating a
//   placeholder pane in the source so the source survives break-pane
//   destroying the daemon's window. `new-window` achieves the same
//   first-pane-pid-mismatch end state with a single tmux call and no
//   sibling session bookkeeping.
//
// Hysteresis constant mirror:
//
//   `cmd.selfSupervisionHysteresisTicks` is unexported and lives in the
//   `cmd` package; this test sits in `bootstrap_test` and cannot
//   reference it directly. We mirror its value as a const in this file
//   so the budget arithmetic reads cleanly; mirrors the precedent set
//   by `legitimateColdStartHysteresisMirror` in
//   cmd/state_daemon_self_supervision_integration_test.go.
//
// No t.Parallel — cmd-package convention.
//
// Spec references:
//   - § Composite End-to-End Verification step 8: "After externally
//     killing the legitimate daemon's `_portal-saver` pane (simulating
//     an out-of-band saver loss), the daemon self-ejects within (N+1)
//     tick intervals (Component D in the live context)."
//   - § Component D — Self-check sequence (pid-mismatch branch).
//   - § Component D — No final flush on self-eject (snapBefore ==
//     snapAfter).
//   - § Component D — Stale `daemon.pid` after self-eject is intentional.
//   - § End-State Verification — daemon log INFO marker.

package bootstrap_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// selfEjectComposite_HysteresisTicksMirror mirrors
// cmd.selfSupervisionHysteresisTicks (= 3, pinned by Task 5-1's
// hardware measurement). Mirroring the value here keeps this file
// readable without importing the cmd package directly; any divergence
// is caught by spec-side review rather than test-runtime — a larger
// production N just gives more headroom inside the bounded budget
// computed below.
const selfEjectComposite_HysteresisTicksMirror = 3

// selfEjectComposite_TickerPeriodMirror mirrors the daemon's per-tick
// cadence (cmd/state_daemon.go: `TickerPeriod: 1 * time.Second`). Used
// by the (N+1)*TickerPeriod + slack budget below.
const selfEjectComposite_TickerPeriodMirror = 1 * time.Second

// selfEjectComposite_ExitBudget is the wall-time ceiling on the
// daemon's exit measured from the external-mismatch instant. Spec §
// Component D requires self-eject within (N+1) tick intervals; the
// 2 s slack absorbs probe-call latency and process-reap jitter on
// darwin/arm64 CI hardware. Mirrors the budget envelope used by the
// per-component tests in cmd/state_daemon_self_supervision_integration_test.go.
const selfEjectComposite_ExitBudget = (selfEjectComposite_HysteresisTicksMirror+1)*selfEjectComposite_TickerPeriodMirror + 2*time.Second

// selfEjectComposite_ExitPollTick is the cadence of the post-mismatch
// `kill(pid, 0)` poll loop. 100 ms is a common probe interval —
// frequent enough to bound the latency between actual eject and
// test-observed exit, cheap enough to not perturb scheduling.
const selfEjectComposite_ExitPollTick = 100 * time.Millisecond

// selfEjectComposite_ConvergenceTimeout is the 6 s post-bootstrap
// convergence budget from spec § "Composite End-to-End Verification"
// bullet 5. Matches convergencePGrepTimeout in
// composition_e2e_convergence_integration_test.go verbatim.
const selfEjectComposite_ConvergenceTimeout = 6 * time.Second

// selfEjectComposite_PlaceholderCommand is the long-lived placeholder
// shell command spawned as the new-window's initial process. Matches
// the canonical placeholder used by Phase 3's saver-creation tests
// and by Task 5-6's pid-mismatch test in
// cmd/state_daemon_self_supervision_integration_test.go.
const selfEjectComposite_PlaceholderCommand = `exec tail -f /dev/null`

// selfEjectComposite_LogMarker is the load-bearing INFO log substring
// emitted by cmd/state_daemon.go's tick loop at the osExit(0) call site.
// Task 5-10 replaced the ad-hoc "self-supervision: saver-membership lost,
// exiting" line with the cataloged "self-eject" lifecycle event (spec
// § Saver and daemon lifecycle event taxonomy — daemon "self-eject"),
// rendered under the daemon component as "daemon: self-eject ticks=N
// threshold=3". Mirrors `selfEjectLogMarker` in
// cmd/state_daemon_self_supervision_integration_test.go.
const selfEjectComposite_LogMarker = "daemon: self-eject"

// TestCompositeBootstrap_ExternalSaverKillTriggersSelfEject pins spec
// § Composite End-to-End Verification bullet 8 in the LIVE composite
// context. See the file-header comment for the assertion shape,
// mechanism rationale, and budget arithmetic.
func TestCompositeBootstrap_ExternalSaverKillTriggersSelfEject(t *testing.T) {
	// Set PORTAL_LOG_LEVEL=INFO BEFORE setupCompositeHarness so the
	// tmux server spawned inside the harness inherits the level
	// (tmuxtest.New does not set cmd.Env explicitly, so the server
	// inherits the test process env). Without this, the daemon
	// constructed inside _portal-saver defaults to LevelWarn (see
	// internal/state/logger.go parseLevel) and the self-eject INFO
	// marker assertion below would always fail. Mirrors the env-wiring
	// rationale in cmd/state_daemon_self_supervision_integration_test.go.
	t.Setenv("PORTAL_LOG_LEVEL", "INFO")

	h := setupCompositeHarness(t)

	// Drive the production bootstrap slice in orchestrator order
	// (steps 4-5). Nil Logger — this test does not assert on logger
	// emissions (6-3 covers that).
	sweeper := bootstrapadapter.NewOrphanSweeper(h.Client, nil)
	start := time.Now()
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error "+
			"(best-effort step must return nil): %v", err)
	}
	if err := tmux.BootstrapPortalSaver(h.Client, h.StateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver (post-sweep idempotent re-run): %v", err)
	}

	// Convergence: pgrep -fx must reach 1 within the 6 s budget
	// measured from `start`. Computing REMAINING budget at the poll
	// site enforces "within 6 s of bootstrap entry" rather than
	// restarting a fresh 6 s window after the bootstrap slice returns.
	remaining := selfEjectComposite_ConvergenceTimeout - time.Since(start)
	if remaining <= 0 {
		t.Fatalf("post-bootstrap: 6 s budget already exhausted by the bootstrap "+
			"slice itself (elapsed=%s) — cannot assert convergence",
			time.Since(start))
	}
	if !waitForPgrepCount(t, 1, remaining) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("post-bootstrap: pgrep -fx did not converge to 1 within %s of "+
			"bootstrap-slice entry (elapsed=%s)\n"+
			"  harness saver PID (setup-time): %d (alive=%v)\n"+
			"  harness orphan1 PID: %d (alive=%v)\n"+
			"  harness orphan2 PID: %d (alive=%v)\n"+
			"  current pgrep snapshot: %v",
			selfEjectComposite_ConvergenceTimeout, time.Since(start),
			h.LegitimateDaemonPID, pidAlive(h.LegitimateDaemonPID),
			h.Orphan1PID, pidAlive(h.Orphan1PID),
			h.Orphan2PID, pidAlive(h.Orphan2PID),
			pids)
	}

	// Re-read the current saver-pane PID — the bootstrap slice may
	// have respawned the saver pane (version-guard / kill-barrier
	// escalation), so the survivor may differ from
	// h.LegitimateDaemonPID. Use waitForSaverPanePID (3 s budget)
	// rather than readSaverPanePID for the same respawn-race reason
	// cited in the 6-3 convergence test.
	survivorPID := waitForSaverPanePID(t, h.Sock)

	// Survivor sanity: the convergence survivor MUST be alive at this
	// instant. Without this barrier, a survivor that died between
	// pgrep-converges-to-1 and this read would surface as a
	// hard-to-diagnose hang in the post-mismatch poll loop below.
	if !pidAlive(survivorPID) {
		t.Fatalf("convergence survivor PID %d not alive after pgrep convergence — "+
			"composition regression: the surviving daemon died before the "+
			"self-eject test sequence could start", survivorPID)
	}

	// Survivor identifies as a portal daemon — barrier mirrors the
	// 6-5 test's wait. Without it, the daemon may still be in its
	// startup window where the per-tick probe has not yet armed.
	waitForIdentifyDaemon(t, survivorPID)

	// Pre-eject scrollback fingerprint. The scrollback dir under
	// h.StateDir is the daemon's per-pane capture target; we
	// fingerprint it BEFORE inducing the mismatch so the post-eject
	// snapshot can prove no defer-driven final flush ran.
	scrollbackDir := state.ScrollbackDir(h.StateDir)
	snapBefore, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("snapBefore SnapshotStateDir(%s): %v", scrollbackDir, err)
	}

	// daemon.pid pre-eject sanity: must reference the survivor. After
	// the convergence path, BootstrapPortalSaver's freshly-spawned
	// daemon writes its own PID into daemon.pid. The post-eject
	// assertion below verifies the stale value is RETAINED (Component
	// D's "stale daemon.pid is intentional" invariant); to assert
	// stale-stays-stale, we first read the pre-eject value.
	preEjectDaemonPID, err := state.ReadPIDFile(h.StateDir)
	if err != nil {
		t.Fatalf("ReadPIDFile pre-eject: %v\n"+
			"  the post-eject assertion needs the pre-eject value to "+
			"verify stale-stays-stale", err)
	}
	if preEjectDaemonPID != survivorPID {
		t.Fatalf("pre-eject daemon.pid = %d; want survivor PID %d\n"+
			"  daemon.pid is not yet refreshed to reference the survivor — "+
			"the post-eject stale-stays-stale assertion would be ambiguous",
			preEjectDaemonPID, survivorPID)
	}

	// Induce the external pane-pid mismatch via `tmux new-window`.
	// See the file-header comment for the full rationale. The
	// placeholder command is wrapped in `sh -c` so tmux's
	// shell-command argument receives a single token; mirrors the
	// `sh -c "exec tail -f /dev/null"` form used by Task 5-6's
	// pid-mismatch test.
	if out, runErr := h.Sock.TryRun("new-window", "-t", tmux.PortalSaverName+":",
		"sh", "-c", selfEjectComposite_PlaceholderCommand); runErr != nil {
		t.Fatalf("tmux new-window -t %s: failed: %v\n  output: %s\n"+
			"  the external-mismatch mechanism requires a successful new-window "+
			"call; without it the daemon's saver-membership probe still observes "+
			"a matching pid and the self-eject path cannot fire",
			tmux.PortalSaverName, runErr, out)
	}
	mismatchInstant := time.Now()

	// Pre-poll structural sanity 1: the placeholder pane IS the new
	// active pane in `_portal-saver`. If this is not true (e.g. tmux
	// version drift in new-window's default-active behaviour), the
	// per-tick probe would still observe the daemon's pid and the
	// self-eject path would not fire — surface that diagnosis up
	// front rather than via a 6 s hang.
	newActivePID, present, err := tmux.SaverPanePIDOrAbsent(h.Client, tmux.PortalSaverName)
	if err != nil {
		t.Fatalf("tmux.SaverPanePIDOrAbsent post-new-window: %v\n"+
			"  the external-mismatch verification requires reading the saver "+
			"session's active-window pane pid; a read failure here means the "+
			"saver session was destroyed by the new-window call (unexpected)",
			err)
	}
	if !present {
		t.Fatalf("tmux.SaverPanePIDOrAbsent post-new-window: present=false\n" +
			"  the external-mismatch verification requires the saver session " +
			"to still host a pane after the new-window call; absent here means " +
			"the saver session was destroyed (unexpected)")
	}
	if newActivePID == survivorPID {
		t.Fatalf("post-new-window saver pane pid (%d) STILL equals survivor PID "+
			"(%d) — new-window did NOT switch the session's active window "+
			"to the placeholder. The daemon's saver-membership probe would "+
			"observe a matching pid every tick and never self-eject. "+
			"Likely cause: tmux version drift in new-window's default-active "+
			"behaviour. Re-evaluate the external-mismatch mechanism.",
			newActivePID, survivorPID)
	}

	// Pre-poll structural sanity 2: the daemon process is STILL alive
	// immediately after the external mismatch event. Mirrors the plan
	// brief's explicit acceptance bullet — distinguishes "daemon
	// killed by the mechanism" (defeats the test) from "daemon alive,
	// awaiting self-eject" (the path under test).
	if !pidAlive(survivorPID) {
		t.Fatalf("survivor PID %d not alive immediately after external mismatch — "+
			"the new-window mechanism appears to have killed the daemon (defeats "+
			"the os.Exit(0) self-eject path under test)", survivorPID)
	}

	// Poll for the daemon's exit via `kill(pid, 0) == ESRCH`. We
	// poll rather than Wait because the daemon process was started
	// by tmux (as the saver pane's process), not by this test — we
	// have only its PID, not a *os.Process to Wait on.
	exited, exitLatency := pollForPIDExit(survivorPID, mismatchInstant, selfEjectComposite_ExitBudget, selfEjectComposite_ExitPollTick)
	if !exited {
		logBlob := portaltest.ReadPortalLogSafe(h.StateDir)
		t.Fatalf("daemon (PID %d) did not exit within %s of external mismatch event; "+
			"spec § Component D requires self-eject within (N+1)*TickerPeriod "+
			"= %s for N=%d (TickerPeriod=%s) plus slack\n"+
			"  elapsed: %s\n"+
			"  budget: %s\n"+
			"  daemon still alive: %v\n"+
			"--- portal.log ---\n%s",
			survivorPID, selfEjectComposite_ExitBudget,
			time.Duration(selfEjectComposite_HysteresisTicksMirror+1)*selfEjectComposite_TickerPeriodMirror,
			selfEjectComposite_HysteresisTicksMirror, selfEjectComposite_TickerPeriodMirror,
			exitLatency, selfEjectComposite_ExitBudget,
			pidAlive(survivorPID), logBlob)
	}
	t.Logf("daemon self-eject latency: %s (budget=%s, pid=%d)",
		exitLatency, selfEjectComposite_ExitBudget, survivorPID)

	// Read portal.log once for the assertions below — they all cite
	// it on failure and re-reading would risk inconsistent diagnostic
	// dumps across multiple failed assertions.
	logBlob := portaltest.ReadPortalLogSafe(h.StateDir)

	// Post-eject scrollback fingerprint. snapAfter is captured
	// IMMEDIATELY after exit observation so the eject-window is as
	// narrow as possible — any post-exit ambient writer (none
	// expected; pgrep was 1 + survivor just died = 0) would
	// otherwise pollute the comparison.
	snapAfter, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("snapAfter SnapshotStateDir(%s): %v\n--- portal.log ---\n%s",
			scrollbackDir, err, logBlob)
	}

	// Assertion 1: scrollback bytes-identical pre/post the eject
	// window. Spec § Component D "No final flush on self-eject":
	// snapBefore == snapAfter across all Fingerprint fields. Empty-
	// both is a legitimate pass (the load-bearing invariant is "no
	// delta", not "non-empty pre-snapshot").
	if deltas := portaltest.DiffFingerprints(snapBefore, snapAfter); len(deltas) > 0 {
		lines := make([]string, len(deltas))
		for k, d := range deltas {
			lines[k] = "  " + portaltest.FormatDelta(d)
		}
		t.Fatalf("scrollback dir mutated between snapBefore (pre-external-mismatch) "+
			"and snapAfter (post-self-eject) — spec § Component D requires "+
			"NO final flush on self-eject\n"+
			"  scrollback dir: %s\n"+
			"  delta(s):\n%s\n"+
			"--- portal.log ---\n%s",
			scrollbackDir, strings.Join(lines, "\n"), logBlob)
	}

	// Assertion 2: daemon.pid file remains on disk post-eject and
	// retains the survivor's PID. Spec § Component D bullet 4.iii:
	// "os.Exit(0) skips any defer that would clean up daemon.pid"
	// — the file is intentionally left stale and Component C's
	// pre-check handles the dead PID on next acquire. Asserting both
	// presence and PID-retention guards against a regression where
	// a misguided cleanup defer rewrote the file before exit.
	pidPath := state.DaemonPID(h.StateDir)
	if _, statErr := os.Stat(pidPath); statErr != nil {
		t.Fatalf("daemon.pid missing post-eject: %v\n"+
			"  spec § Component D bullet 4.iii: the stale daemon.pid is "+
			"intentional — os.Exit(0) MUST NOT trigger any cleanup defer\n"+
			"--- portal.log ---\n%s", statErr, logBlob)
	}
	postEjectDaemonPID, err := state.ReadPIDFile(h.StateDir)
	if err != nil {
		t.Fatalf("ReadPIDFile post-eject: %v\n--- portal.log ---\n%s", err, logBlob)
	}
	if postEjectDaemonPID != survivorPID {
		t.Fatalf("post-eject daemon.pid = %d; want survivor PID %d (stale-stays-stale)\n"+
			"  the file was rewritten by some other writer between the daemon's "+
			"WritePIDFile and its self-eject — Component D bullet 4.iii violation\n"+
			"--- portal.log ---\n%s",
			postEjectDaemonPID, survivorPID, logBlob)
	}

	// Assertion 3: portal.log contains the self-eject INFO marker.
	// Spec § Component D bullet 4.i mandates the exact prefix
	// emission at the os.Exit(0) call site. Asserting substring
	// presence (not exact equality) so a future suffix tweak does
	// not flake this test.
	if !strings.Contains(logBlob, selfEjectComposite_LogMarker) {
		t.Errorf("portal.log missing self-eject marker %q\n"+
			"  spec § Component D bullet 4.i: the eject path MUST emit this INFO line\n"+
			"  observed eject (PID %d gone within %s), but the marker is absent — "+
			"the daemon may have exited via a different path (SIGHUP, lock loss, ...)\n"+
			"--- portal.log ---\n%s",
			selfEjectComposite_LogMarker, survivorPID, exitLatency, logBlob)
	}
}

// pollForPIDExit polls `syscall.Kill(pid, 0)` every tick until the
// PID is no longer alive (Kill returns ESRCH) or the budget elapses.
// Returns (exited, elapsed-since-startInstant). The first call fires
// IMMEDIATELY (no leading sleep) so a sub-tick eject is observed
// promptly; subsequent calls are spaced by `tick`.
//
// Inlined here rather than shared because the cmd/bootstrap _test
// package has no shared helper of this shape — the canonical
// liveness probe `pidAlive` returns a boolean, and the rest of the
// test suite uses goroutine-driven exec.Cmd.Wait for processes it
// owns (which this test does not — the daemon was spawned by tmux).
func pollForPIDExit(pid int, startInstant time.Time, budget, tick time.Duration) (bool, time.Duration) {
	deadline := startInstant.Add(budget)
	for {
		if !pidAlive(pid) {
			return true, time.Since(startInstant)
		}
		if time.Now().After(deadline) {
			return false, time.Since(startInstant)
		}
		time.Sleep(tick)
	}
}
