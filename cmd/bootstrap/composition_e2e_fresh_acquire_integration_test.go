//go:build integration

// Composite end-to-end fresh-process AcquireDaemonLock refusal test for
// spec § "Composite End-to-End Verification" — task 6-5.
//
// Consumes the shared compositeHarness (3-daemon pre-state: legitimate
// saver-pane daemon + 2 orphans; legitimate stateDir's daemon.pid
// references orphan1, simulating the reporter's "orphan-with-daemon.pid"
// case). Invokes the production bootstrap slice — `SweepOrphanDaemons`
// (Component B) then `BootstrapPortalSaver` (Component F + A's
// escalation path) in the same direct-adapter order the orchestrator
// runs them at steps 4-5 — waits for pgrep convergence to 1, and then
// asserts that a fresh `state.AcquireDaemonLock` call from the test
// goroutine refuses cleanly with `state.ErrDaemonLockHeld`.
//
// Form chosen: in-test-goroutine AcquireDaemonLock call. The task brief
// suggests a separate subprocess with sentinel exit code 42 as the
// "preferred" approach but explicitly permits the in-process variant if
// there is a clear precedent — `TestUpgradePath_ComponentC_IsolatedRefusesCleanly`
// and `TestUpgradePath_PostBootstrap_FreshAcquireDaemonLockRefuses` in
// upgrade_path_integration_test.go both call AcquireDaemonLock directly
// from the test goroutine and assert ErrDaemonLockHeld via errors.Is.
// The test goroutine is a fresh acquisition site within the same
// process; Go's flock semantics treat the call as a fresh fd → fresh
// inode check; and the pre-check path reads daemon.pid via
// lockAcquireReadPIDFile + lockAcquireIdentifyDaemon, neither of which
// short-circuits based on caller-process identity. The in-process call
// is therefore structurally equivalent to a subprocess call for this
// assertion's purposes.
//
// Layered enforcement reminder: ErrDaemonLockHeld is returned via either
// the pre-check (daemon.pid identifies the live saver-pane daemon — no
// fd opened) or the flock EWOULDBLOCK fallback (daemon.pid is stale but
// the saver-pane daemon already holds the flock on the same inode).
// Both paths satisfy errors.Is(_, state.ErrDaemonLockHeld) so the
// assertion holds under either branch.
//
// Edge case handled: the harness's orphan1-as-daemon.pid setup MAY have
// been overwritten by BootstrapPortalSaver's respawn (Component A's
// kill-barrier escalation rewrites daemon.pid with the survivor's PID).
// We therefore re-read daemon.pid post-convergence and assert it
// references the survivor — both as a precondition for the pre-check
// path to fire AND as the "no destructive coexistence" guarantee in
// task 6-5's edge-case list.
//
// No t.Parallel — cmd-package convention.

package bootstrap_test

import (
	"errors"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// freshAcquireConvergenceTimeout is the 6 s post-bootstrap convergence
// budget from spec § "Composite End-to-End Verification" bullet 5.
// Matches convergencePGrepTimeout in composition_e2e_convergence_integration_test.go
// verbatim — same spec citation, same budget.
const freshAcquireConvergenceTimeout = 6 * time.Second

// TestCompositeBootstrap_FreshAcquireDaemonLockRefusesPostBootstrap
// exercises the composite end-to-end fresh-process refusal assertion
// against the 3-daemon harness pre-state. See the file-header comment
// for the assertion shape and in-process-call rationale.
func TestCompositeBootstrap_FreshAcquireDaemonLockRefusesPostBootstrap(t *testing.T) {
	h := setupCompositeHarness(t)

	// Drive the production bootstrap slice in orchestrator order
	// (steps 4-5): Component B's orphan sweep, then Component F + A's
	// saver bootstrap. We pass `nil` for the Logger arg because this
	// test does not assert on logger emissions — the convergence test
	// (6-3) covers the forbidden-strings check, and stacking it here
	// would add no signal.
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
	// measured from `start`. Compute REMAINING budget at the poll site
	// so the assertion enforces "within 6 s of bootstrap entry" rather
	// than restarting a fresh 6 s window after the bootstrap slice
	// returns.
	remaining := freshAcquireConvergenceTimeout - time.Since(start)
	if remaining <= 0 {
		t.Fatalf("post-bootstrap: 6 s budget already exhausted by the bootstrap "+
			"slice itself (elapsed=%s) — cannot assert convergence",
			time.Since(start))
	}
	if !waitForPgrepCount(t, 1, remaining) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("post-bootstrap: pgrep -fx did not converge to 1 within %s of "+
			"bootstrap-slice entry (elapsed=%s, budget=%s)\n"+
			"  harness saver PID (setup-time): %d (alive=%v)\n"+
			"  harness orphan1 PID: %d (alive=%v)\n"+
			"  harness orphan2 PID: %d (alive=%v)\n"+
			"  current pgrep snapshot: %v",
			freshAcquireConvergenceTimeout, time.Since(start), freshAcquireConvergenceTimeout,
			h.LegitimateDaemonPID, pidAlive(h.LegitimateDaemonPID),
			h.Orphan1PID, pidAlive(h.Orphan1PID),
			h.Orphan2PID, pidAlive(h.Orphan2PID),
			pids)
	}

	// Re-read the current saver-pane PID — the bootstrap slice may have
	// respawned the saver pane (version-guard / kill-barrier
	// escalation), so the survivor may differ from h.LegitimateDaemonPID.
	// Use waitForSaverPanePID (3 s budget) rather than readSaverPanePID
	// (50 ms × 2 retry) for the same respawn-race reason cited in the
	// 6-3 convergence test.
	currentSaverPID := waitForSaverPanePID(t, h.Sock)

	// Verify daemon.pid is fresh post-convergence — task 6-5 edge case:
	// the harness's orphan1-as-daemon.pid setup MUST have been
	// overwritten by BootstrapPortalSaver's respawn for the pre-check
	// path to fire on the live survivor. This is also the spec's "no
	// destructive coexistence" guarantee: the survivor's daemon.pid
	// reference is what the next bootstrap (or the test-bench
	// AcquireDaemonLock call below) reads to detect the existing
	// holder.
	pidOnDisk, err := state.ReadPIDFile(h.StateDir)
	if err != nil {
		t.Fatalf("ReadPIDFile after bootstrap convergence: %v", err)
	}
	if pidOnDisk != currentSaverPID {
		t.Fatalf("post-bootstrap daemon.pid = %d; want current saver pane PID %d\n"+
			"  harness saver PID (setup-time): %d\n"+
			"  harness orphan1 PID: %d (alive=%v)\n"+
			"  harness orphan2 PID: %d (alive=%v)\n"+
			"  daemon.pid was not refreshed to reference the survivor — "+
			"the pre-check path cannot fire and the assertion below would "+
			"either fail spuriously or pass via the flock fallback only",
			pidOnDisk, currentSaverPID,
			h.LegitimateDaemonPID,
			h.Orphan1PID, pidAlive(h.Orphan1PID),
			h.Orphan2PID, pidAlive(h.Orphan2PID))
	}

	// Wait until the survivor identifies as a portal daemon via
	// IdentifyDaemon. Without this barrier the pre-check could (per
	// spec) treat a still-spawning PID as "no holder" and proceed to
	// the flock path. Both paths satisfy errors.Is, but routing
	// through the pre-check is the steady-state expected branch — and
	// converging on it here removes a race-on-startup flake source.
	waitForIdentifyDaemon(t, currentSaverPID)

	// Fresh AcquireDaemonLock from the test goroutine. The pre-check
	// reads daemon.pid (= currentSaverPID), identifies it as a live
	// portal daemon, and returns ErrDaemonLockHeld WITHOUT opening
	// daemon.lock. If the pre-check is somehow bypassed (e.g. a future
	// regression in the seam wiring), the flock EWOULDBLOCK fallback
	// returns the same sentinel — both paths satisfy
	// errors.Is(_, state.ErrDaemonLockHeld), so the assertion below
	// holds under either layered-enforcement branch.
	fd, acquireErr := state.AcquireDaemonLock(h.StateDir)

	// Acceptance: the returned fd MUST be nil on refusal (no leaked fd).
	// Check this BEFORE the errors.Is assertion so a non-nil fd is
	// closed even on a successful errors.Is match — defensive against
	// a future regression where the error path leaks an fd.
	if fd != nil {
		_ = fd.Close()
		t.Fatalf("AcquireDaemonLock returned non-nil fd alongside error %v — "+
			"the refusal path must not leak an fd "+
			"(survivor PID = %d)",
			acquireErr, currentSaverPID)
	}

	if !errors.Is(acquireErr, state.ErrDaemonLockHeld) {
		t.Fatalf("AcquireDaemonLock from test goroutine = %v; want ErrDaemonLockHeld "+
			"(Component C pre-check should fire on the live survivor PID %d; "+
			"daemon.pid on disk = %d)",
			acquireErr, currentSaverPID, pidOnDisk)
	}

	// No destructive coexistence: re-read daemon.pid AFTER the refused
	// acquire and confirm it still references the survivor. A refused
	// acquire MUST NOT mutate daemon.pid — neither pre-check nor flock
	// EWOULDBLOCK paths write the PID file. Catches a regression where
	// the refusal path accidentally calls WritePIDFile or unlinks the
	// file.
	pidOnDiskAfter, err := state.ReadPIDFile(h.StateDir)
	if err != nil {
		t.Fatalf("ReadPIDFile after refused AcquireDaemonLock: %v", err)
	}
	if pidOnDiskAfter != currentSaverPID {
		t.Fatalf("daemon.pid after refused AcquireDaemonLock = %d; want survivor PID %d\n"+
			"  the refusal mutated daemon.pid — destructive-coexistence violation",
			pidOnDiskAfter, currentSaverPID)
	}

	// Belt-and-braces: the survivor is still alive after the refused
	// acquire. The pre-check path never signals the holder (mirrors the
	// 4-9 Scenario B assertion), so the survivor must still be running.
	if !state.IsProcessAlive(currentSaverPID) {
		t.Fatalf("survivor PID %d is no longer alive after refused AcquireDaemonLock — "+
			"the pre-check appears to have signalled the live holder "+
			"(Component C destructive-coexistence violation)",
			currentSaverPID)
	}
}
