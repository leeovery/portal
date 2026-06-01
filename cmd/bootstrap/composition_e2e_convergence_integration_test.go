//go:build integration

// Composite end-to-end convergence test for spec § "Composite End-to-End
// Verification" bullet 5 — task 6-3.
//
// Consumes the shared compositeHarness (3-daemon pre-state: legitimate
// saver-pane daemon + 2 orphans; legitimate stateDir's daemon.pid
// references orphan1, simulating the reporter's "orphan-with-daemon.pid"
// case). Invokes the production bootstrap slice — `SweepOrphanDaemons`
// (Component B) then `BootstrapPortalSaver` (Component F + A's
// escalation path) in the same direct-adapter order the orchestrator
// runs them at steps 4–5 — and asserts:
//
//  1. `pgrep -fx '^portal state daemon( |$)'` converges to 1 within 6 s
//     of bootstrap-slice entry. Budget is measured from the `start`
//     instant immediately before SweepOrphanDaemons, matching the spec's
//     "6 s of bootstrap entering EnsureSaver" wording.
//  2. The sole surviving daemon PID equals the current `_portal-saver`
//     pane's pane_pid (via `tmux list-panes -F '#{pane_pid}'`). The
//     survivor could be the originally legitimate daemon OR a freshly
//     respawned one, but it MUST match the current pane PID.
//  3. The captured bootstrap logger emitted ZERO entries containing
//     `"no such session: _portal-saver"` and ZERO entries containing
//     `"prior daemon did not exit"` — the two cascade signatures the
//     spec's End-State Verification section forbids under steady state.
//
// Logger capture mechanism:
//   The production `bootstrapadapter.NewOrphanSweeper(client, *slog.Logger)`
//   takes a concrete `*slog.Logger` — so we mirror the pattern
//   from TestSweepOrphanDaemons_Integration_CleanStateZeroSignals (this
//   same _test package): call `NewOrphanSweeper(client, nil)`,
//   type-assert to `*bootstrap.OrphanSweepCore`, and overwrite the
//   `Logger` field with a capturing `bootstrap.RecordingLogger`'s
//   `*slog.Logger`. The production fields the adapter set (`Pgrep`,
//   `SaverPanePID`) are preserved — only the Logger seam is swapped.
//   `BootstrapPortalSaver` itself does not take a logger (it writes
//   to the central portal.log via the package handler) and
//   so contributes to the forbidden-string assertion only via the
//   captured-logger entries when its callee path emits via the same
//   logger sink. The forbidden-strings assertion here is therefore
//   focused on the Component B sweep's log emissions, which is where
//   the spec's "no such session: _portal-saver" cascade would manifest
//   if Component B observed a stale saver session.
//
// No t.Parallel — cmd-package convention.

package bootstrap_test

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmux"
)

// convergencePGrepTimeout is the 6 s post-bootstrap budget from spec
// § "Composite End-to-End Verification" bullet 5: "pgrep returns 1
// within 6 s of bootstrap entering EnsureSaver (Component A's
// escalation budget + Component B's sweep latency)". Sized verbatim
// to the spec.
const convergencePGrepTimeout = 6 * time.Second

// TestCompositeBootstrap_ConvergesPgrepToOneWithin6s exercises the
// composite end-to-end convergence assertion against the 3-daemon
// harness pre-state. See the file-header comment for the assertion
// shape and logger-capture rationale.
func TestCompositeBootstrap_ConvergesPgrepToOneWithin6s(t *testing.T) {
	h := setupCompositeHarness(t)

	// Wire the production Component B adapter and swap in a recording
	// Logger via type-assertion on the underlying *bootstrap.OrphanSweepCore.
	// The adapter exposes the concrete type for exactly this test-time
	// override pattern (mirrors the clean-state scenario test).
	sweeper := bootstrapadapter.NewOrphanSweeper(h.Client, nil)
	core, ok := sweeper.(*bootstrap.OrphanSweepCore)
	if !ok {
		t.Fatalf("NewOrphanSweeper returned %T; want *bootstrap.OrphanSweepCore "+
			"(needed to inject a recording Logger for the forbidden-string assertion)",
			sweeper)
	}
	logger := &bootstrap.RecordingLogger{}
	core.Logger = logger.Logger()

	// Capture `start` IMMEDIATELY before the bootstrap slice fires. The
	// 6 s convergence budget is measured against this instant — matches
	// the spec's "bootstrap entering EnsureSaver" reference point.
	start := time.Now()

	// Component B (orphan sweep) — kills the 2 orphans by argv + identity.
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error "+
			"(best-effort step must return nil): %v", err)
	}

	// Component F + A's escalation path — idempotently re-bootstraps the
	// saver session. The saver pane was up from harness setup;
	// BootstrapPortalSaver no-ops when healthy, or respawns if the kill
	// barrier or version guard escalated.
	if err := tmux.BootstrapPortalSaver(h.Client, h.StateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver (post-sweep idempotent re-run): %v", err)
	}

	// Convergence: pgrep -fx must reach 1 within the 6 s budget measured
	// from `start`. Compute the REMAINING budget at the poll site so the
	// assertion enforces "within 6 s of bootstrap entry" rather than
	// restarting a fresh 6 s window after the bootstrap slice returns.
	remaining := convergencePGrepTimeout - time.Since(start)
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
			convergencePGrepTimeout, time.Since(start), convergencePGrepTimeout,
			h.LegitimateDaemonPID, pidAlive(h.LegitimateDaemonPID),
			h.Orphan1PID, pidAlive(h.Orphan1PID),
			h.Orphan2PID, pidAlive(h.Orphan2PID),
			pids)
	}
	convergenceElapsed := time.Since(start)

	// Survivor identity: the sole remaining daemon must equal the
	// CURRENT `_portal-saver` pane's pane_pid. Re-read pane_pid here so
	// the comparison is against the live pane process — the saver may
	// have been respawned during the bootstrap slice (version-guard
	// escalation, kill-barrier escalation, etc.), in which case the
	// survivor is the freshly respawned pane PID rather than the
	// setup-time h.LegitimateDaemonPID.
	survivors, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("post-bootstrap pgrep snapshot: %v", err)
	}
	if len(survivors) != 1 {
		t.Fatalf("post-bootstrap: expected exactly 1 daemon, got %d: %v "+
			"(convergence elapsed: %s)",
			len(survivors), survivors, convergenceElapsed)
	}
	// Use waitForSaverPanePID (3 s budget) rather than readSaverPanePID
	// (50 ms × 2 retry) — the bootstrap slice may have respawned the
	// saver pane (e.g. version-guard kicked because daemon.pid was
	// reset to orphan1 in harness setup and BootstrapPortalSaver
	// rebuilt the pane), and the respawn race window can exceed the
	// 100 ms readSaverPanePID budget.
	currentSaverPID := waitForSaverPanePID(t, h.Sock)
	if survivors[0] != currentSaverPID {
		t.Fatalf("post-bootstrap: survivor PID %d != current saver pane PID %d\n"+
			"  harness saver PID (setup-time): %d\n"+
			"  harness orphan1 PID: %d\n"+
			"  harness orphan2 PID: %d\n"+
			"  the surviving daemon is NOT the saver-pane process — composition regression",
			survivors[0], currentSaverPID,
			h.LegitimateDaemonPID, h.Orphan1PID, h.Orphan2PID)
	}

	// Forbidden-strings assertion: the captured logger must contain
	// ZERO entries with `"no such session: _portal-saver"` or
	// `"prior daemon did not exit"`. Both substrings are cascade
	// signatures the spec's End-State Verification section forbids
	// under steady state.
	const forbiddenNoSuchSession = "no such session: _portal-saver"
	const forbiddenPriorDaemonExit = "prior daemon did not exit"
	for _, entry := range logger.AllEntries() {
		if strings.Contains(entry, forbiddenNoSuchSession) {
			t.Fatalf("bootstrap logger emitted forbidden entry containing %q\n"+
				"  entry: %s\n"+
				"  all entries:\n%s",
				forbiddenNoSuchSession, entry,
				strings.Join(logger.AllEntries(), "\n"))
		}
		if strings.Contains(entry, forbiddenPriorDaemonExit) {
			t.Fatalf("bootstrap logger emitted forbidden entry containing %q\n"+
				"  entry: %s\n"+
				"  all entries:\n%s",
				forbiddenPriorDaemonExit, entry,
				strings.Join(logger.AllEntries(), "\n"))
		}
	}
}
