//go:build integration

// Composite integration test for spec § "Composite End-to-End Verification"
// (Phase 4 subset — A+B+C end-to-end).
//
// This test reconstructs the reporter's failure scenario at fixture
// granularity — three concurrent `portal state daemon` processes
// (1 legitimate saver-pane daemon + 2 orphans) against a real tmux
// server — and asserts the converged healthy end state after the
// production bootstrap pipeline runs. It is the structural regression
// guard for component-composition failures that per-component tests
// cannot catch.
//
// What it pins (per task brief, mapped to spec § "Composite End-to-End
// Verification" bullets 5-7):
//
//   1. PRE-STATE — pgrep -fx '^portal state daemon( |$)' == 3 before
//      bootstrap fires. Without this barrier the post-state assertion
//      could green-pass against an orphan that exited prematurely
//      (silent N=2 → 1 convergence).
//   2. POST-STATE — pgrep == 1 within 6 s of bootstrap entry. The 6 s
//      budget is load-bearing — it is measured against the same
//      `start` instant as the spec's "EnsureSaver entry" timestamp.
//   3. SURVIVOR IDENTITY — sole remaining daemon's PID equals
//      `tmux list-panes -t _portal-saver -F '#{pane_pid}'`. This is
//      the spec's "the survivor is the saver-pane process" invariant.
//   4. SCROLLBACK STABILITY — 10 consecutive 1 s snapshots of
//      <stateDir>/scrollback/ via portaltest.SnapshotStateDir, asserted
//      byte-identical to the first. Catches the daemon-oscillation
//      failure mode (Components A+B+E composition). Empty stays empty
//      is a valid stability proof — see comment at the assertion.
//   5. FRESH-PROCESS ACQUIRE REFUSAL — from the test goroutine,
//      state.AcquireDaemonLock(stateDir) must return ErrDaemonLockHeld
//      (Component C pre-check verifies on the live state).
//
// Composition shape:
//   The task brief permits either an Orchestrator invocation or a
//   subprocess `portal open`. We use the same direct-adapter form as
//   upgrade_path_integration_test.go: SweepOrphanDaemons (B) then
//   BootstrapPortalSaver (A's escalation path + F's saver creation).
//   The composition test pins the END STATE, not which component did
//   the work, so this is faithful to the spec's intent.
//
// Simplification (per task brief step 7): we do NOT write an orphan's
// PID into daemon.pid before bootstrap. The legitimate daemon's
// pre-existing daemon.pid points at the saver-pane daemon; the orphans
// are enumerated by pgrep and swept by Component B based on argv +
// saver-pane-PID legitimacy. The "orphan with daemon.pid reference"
// branch in the spec's Phase-4 setup would defeat Component C's
// pre-check during the in-test AcquireDaemonLock invocation, which is
// the assertion we WANT to pass. The pgrep-enumeration path covers the
// same Component B sweep behaviour without contaminating the C
// pre-check signal.
//
// Per-orphan PORTAL_STATE_DIR tempdirs: mirrors the 4-5 / orphan_sweep
// pattern. Without per-orphan stateDirs, Component C's pre-check
// converges the orphan population to 1 before pgrep can see all three.
// Per-orphan stateDirs let all three daemons stay alive long enough
// for pgrep to observe N=3, while pgrep's argv match (system-wide,
// not stateDir-scoped) still ensures Component B sweeps them.
//
// Cleanup: every spawned subprocess is registered with
// portaltest.RegisterSubprocessCleanup (invoked transitively via
// portaltest.SpawnIsolatedDaemon). Tmux teardown is automatic via
// tmuxtest.New's t.Cleanup hook.
// Fingerprint-diff backstop runs automatically via portaltest.
//
// Host-noise mitigation: portaltest.IsolateStateForTest folds the
// HOME=<tempdir> / XDG_CONFIG_HOME="" scrub in internally (BEFORE
// its pre-snapshot) so the backstop targets a quiet tempdir rather
// than the developer's live state dir.
//
// No t.Parallel: the cmd-package convention applies.

package bootstrap_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// compositionPGrepConvergenceTimeout is the 6 s post-bootstrap budget
// from spec § "Composite End-to-End Verification" bullet 5: "pgrep
// returns 1 within 6 s of bootstrap entering EnsureSaver (Component
// A's escalation budget + Component B's sweep latency)". Sized
// verbatim to the spec.
const compositionPGrepConvergenceTimeout = 6 * time.Second

// compositionPreStateTimeout bounds the pre-bootstrap poll waiting for
// pgrep to observe N=3 daemons. 3 s mirrors the existing orphan-sweep
// scaffolding's pgrepConvergenceTimeout — comfortably above the
// observed daemon-cold-start latency (~hundreds of ms × 3 daemons).
const compositionPreStateTimeout = 3 * time.Second

// compositionScrollbackObservations is the number of 1 s scrollback
// snapshots taken post-bootstrap for the stability assertion. Spec
// § "Composite End-to-End Verification" bullet 6 mandates 10
// consecutive 1 s observations.
const compositionScrollbackObservations = 10

// compositionScrollbackInterval is the gap between consecutive
// scrollback snapshots. 1 s matches the spec's literal wording.
const compositionScrollbackInterval = 1 * time.Second

// TestComposition_PhaseFour_ABC_EndToEnd reconstructs the reporter's
// 3-daemon broken-install scenario and asserts the converged healthy
// end state after the production bootstrap pipeline runs. See the
// file-header comment for the full rationale.
func TestComposition_PhaseFour_ABC_EndToEnd(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	skipIfNoPgrep(t)
	_ = portalbintest.StagePortalBinary(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-comp-abc-")
	client := sock.Client()

	// 1. Bootstrap the legitimate saver-pane daemon via Component F's
	//    placeholder→set-option→respawn ordering (the canonical
	//    BootstrapPortalSaver helper). After this returns the
	//    daemon-host pane is running `portal state daemon` against
	//    the isolated stateDir, and daemon.pid points at it.
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver (legitimate saver): %v", err)
	}
	legitimateSaverPID := waitForSaverPanePID(t, sock)
	waitForDaemonPID(t, stateDir, legitimateSaverPID)

	// 2. Spawn 2 ADDITIONAL orphan daemons as direct test children, each
	//    with its OWN isolated PORTAL_STATE_DIR (per-orphan tempdir).
	//    Per-orphan stateDirs prevent Component C's pre-check from
	//    converging them before pgrep can see N=3 — see the file-header
	//    comment for the full rationale (mirrors the 4-5 / orphan_sweep
	//    pattern). pgrep's argv match is system-wide, so all three
	//    daemons still appear in `pgrep -fx '^portal state daemon( |$)'`.
	orphan1, _ := portaltest.SpawnIsolatedDaemon(t, envSlice)
	orphan2, _ := portaltest.SpawnIsolatedDaemon(t, envSlice)

	// 3. Pre-state barrier: pgrep -fx must reach 3 before bootstrap
	//    fires. On timeout, surface a diagnostic citing all three PIDs
	//    and their liveness so an orphan that exited prematurely is
	//    not silently observed as N=2 → 1 (which would still pass the
	//    post-state assertion).
	if !waitForPgrepCount(t, 3, compositionPreStateTimeout) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("pre-state: pgrep -fx did not reach 3 within %s\n"+
			"  legitimate saver PID: %d (alive=%v)\n"+
			"  orphan1 PID: %d (alive=%v)\n"+
			"  orphan2 PID: %d (alive=%v)\n"+
			"  pgrep snapshot: %v\n"+
			"  hint: an orphan may have exited before bootstrap — composition test cannot fire",
			compositionPreStateTimeout,
			legitimateSaverPID, pidAlive(legitimateSaverPID),
			orphan1.Process.Pid, pidAlive(orphan1.Process.Pid),
			orphan2.Process.Pid, pidAlive(orphan2.Process.Pid),
			pids)
	}

	// 4. Bootstrap invocation. The 6 s post-bootstrap convergence
	//    budget is measured from THIS instant — the spec's "bootstrap
	//    entering EnsureSaver" reference point. Invoke the production
	//    A+B+F adapters in the same order the orchestrator runs them
	//    at steps 4-5 (see cmd/bootstrap/bootstrap.go Run):
	//      Component B (sweep) — kills the 2 orphans by argv.
	//      Component A (escalation) + Component F (saver create/respawn)
	//        — re-bootstrap the saver session if it was disturbed.
	//    The legitimate saver-pane daemon was already up from step 1;
	//    BootstrapPortalSaver is idempotent and will no-op when the
	//    saver pane is already healthy.
	start := time.Now()

	sweeper := bootstrapadapter.NewOrphanSweeper(client, nil)
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error: %v", err)
	}
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver (post-sweep idempotent re-run): %v", err)
	}

	// 5. Post-bootstrap pgrep convergence: pgrep -fx must reach 1
	//    within 6 s of `start`. The remaining budget is measured to
	//    enforce the spec's "6 s of bootstrap entering EnsureSaver"
	//    wording rather than restarting a fresh 6 s window here.
	remaining := compositionPGrepConvergenceTimeout - time.Since(start)
	if remaining <= 0 {
		t.Fatalf("post-bootstrap: 6 s budget already exhausted by bootstrap step itself "+
			"(elapsed=%s) — cannot assert convergence",
			time.Since(start))
	}
	if !waitForPgrepCount(t, 1, remaining) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("post-bootstrap: pgrep -fx did not converge to 1 within %s of bootstrap entry "+
			"(elapsed=%s, budget=%s)\n"+
			"  legitimate saver PID: %d (alive=%v)\n"+
			"  orphan1 PID: %d (alive=%v)\n"+
			"  orphan2 PID: %d (alive=%v)\n"+
			"  current pgrep snapshot: %v",
			compositionPGrepConvergenceTimeout, time.Since(start), compositionPGrepConvergenceTimeout,
			legitimateSaverPID, pidAlive(legitimateSaverPID),
			orphan1.Process.Pid, pidAlive(orphan1.Process.Pid),
			orphan2.Process.Pid, pidAlive(orphan2.Process.Pid),
			pids)
	}
	convergenceElapsed := time.Since(start)

	// 6. Survivor identity: the sole remaining daemon must equal the
	//    saver pane's pane_pid. Re-read the pane_pid post-bootstrap
	//    (it may have been respawned during the saver re-bootstrap).
	survivors, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("post-bootstrap pgrep snapshot: %v", err)
	}
	if len(survivors) != 1 {
		t.Fatalf("post-bootstrap: expected exactly 1 daemon, got %d: %v "+
			"(convergence elapsed: %s)",
			len(survivors), survivors, convergenceElapsed)
	}
	currentSaverPID := readSaverPanePID(t, sock)
	if survivors[0] != currentSaverPID {
		t.Fatalf("post-bootstrap: survivor PID %d != saver pane PID %d\n"+
			"  the surviving daemon is NOT the legitimate saver-pane process\n"+
			"  this is a composition regression — either B killed the wrong daemon "+
			"or A/F failed to recycle the saver pane",
			survivors[0], currentSaverPID)
	}

	// 7. Scrollback stability: 10 consecutive 1 s snapshots of
	//    <stateDir>/scrollback/ must be byte-identical to the first.
	//    Any per-key or per-field delta surfaces as a per-path failure
	//    diagnostic (assertSnapshotsEqual, shared with the kill-barrier
	//    escalation no-final-flush test).
	//
	//    Empty-scrollback edge case: with no user sessions on this tmux
	//    server (we only spawned the saver session), CaptureStructure
	//    may enumerate zero capture-worthy panes and the scrollback dir
	//    stays empty across all 10 observations. An empty-stays-empty
	//    snapshot sequence is a VALID stability proof — the invariant
	//    under test is "no oscillation", and an unchanging empty
	//    directory satisfies that as much as an unchanging populated
	//    one does. The assertion below treats this case as a pass and
	//    logs it as a t.Log so an operator can distinguish "populated
	//    and stable" from "empty and stable" in the run output.
	scrollbackDir := state.ScrollbackDir(stateDir)
	first, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("scrollback snapshot 1/%d: %v", compositionScrollbackObservations, err)
	}
	if len(first) == 0 {
		t.Logf("scrollback dir empty at observation 1 — empty-stays-empty is a valid "+
			"stability proof; remaining %d observations will assert no entries appear",
			compositionScrollbackObservations-1)
	}

	for i := 2; i <= compositionScrollbackObservations; i++ {
		time.Sleep(compositionScrollbackInterval)
		current, snapErr := portaltest.SnapshotStateDir(scrollbackDir)
		if snapErr != nil {
			t.Fatalf("scrollback snapshot %d/%d: %v",
				i, compositionScrollbackObservations, snapErr)
		}
		if deltas := portaltest.DiffFingerprints(first, current); len(deltas) > 0 {
			lines := make([]string, len(deltas))
			for k, d := range deltas {
				lines[k] = "  " + portaltest.FormatDelta(d)
			}
			t.Fatalf("scrollback dir oscillated between first snapshot and "+
				"observation %d/%d (spec § Composite End-to-End Verification "+
				"bullet 6: \"no .bin file deletions or unexpected new files\")\n"+
				"  scrollback dir: %s\n"+
				"  delta(s):\n%s",
				i, compositionScrollbackObservations, scrollbackDir,
				strings.Join(lines, "\n"))
		}
	}

	// 8. Fresh-process Component C pre-check: from the test goroutine,
	//    AcquireDaemonLock(stateDir) must return ErrDaemonLockHeld.
	//    The pre-check reads daemon.pid (== currentSaverPID), identifies
	//    it as a live portal daemon via IdentifyDaemon, and returns
	//    ErrDaemonLockHeld WITHOUT opening daemon.lock. The
	//    layered-enforcement contract means a stale daemon.pid would
	//    fall through to flock EWOULDBLOCK and return the same
	//    sentinel — errors.Is is the load-bearing assertion.
	fd, acquireErr := state.AcquireDaemonLock(stateDir)
	if fd != nil {
		// Defensive: AcquireDaemonLock's contract returns nil fd on
		// error, but close any non-nil fd to avoid leaking the lock
		// fd past the test if a future regression returns one.
		_ = fd.Close()
	}
	if !errors.Is(acquireErr, state.ErrDaemonLockHeld) {
		t.Fatalf("fresh-process AcquireDaemonLock = %v; want ErrDaemonLockHeld "+
			"(Component C pre-check should fire on the live survivor PID %d; "+
			"convergence elapsed: %s)",
			acquireErr, currentSaverPID, convergenceElapsed)
	}
}
