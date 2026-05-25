//go:build integration

// Real-tmux integration tests for spec § Component C acceptance bullet
// "Upgrade-path scenario" — the two-binary landmine where a v(N) daemon
// holds the lock when a v(N+1) bootstrap fires. With Components A+B+C
// composed, the new bootstrap's daemon either acquires cleanly (because
// A/B swept the prior daemon and daemon.pid no longer points at a live
// holder) or refuses cleanly via the pre-check — no destructive
// coexistence.
//
// Three scenarios are pinned end-to-end against a real tmux server,
// real `portal state daemon` subprocesses, and the production
// `bootstrapadapter.NewOrphanSweeper` + `tmux.BootstrapPortalSaver`
// wiring:
//
//   1. TestUpgradePath_TwoBinary_AllComponentsCompose — Scenario A. Spawn
//      a v(N) `portal state daemon` directly (simulating the in-flight
//      daemon a prior binary left running). Wait until it writes
//      daemon.pid. Drive the v(N+1) bootstrap by calling the production
//      adapter's SweepOrphanDaemons + tmux.BootstrapPortalSaver in
//      sequence (the same shape the orchestrator runs steps 4-5). Poll
//      pgrep -fxc until it converges to 1 within 6 s, assert the
//      survivor PID differs from the original v(N) PID, assert
//      daemon.pid on disk references the survivor, and assert a fresh
//      AcquireDaemonLock from the test goroutine returns
//      ErrDaemonLockHeld (the pre-check fires on the live state).
//
//   2. TestUpgradePath_ComponentC_IsolatedRefusesCleanly — Scenario B.
//      Component C in isolation, no orphan sweep and no saver bootstrap.
//      Spawn v(N), wait for daemon.pid, then call AcquireDaemonLock
//      from the test goroutine. The pre-check must return
//      ErrDaemonLockHeld without opening daemon.lock or signalling
//      v(N). Wait 200 ms and assert v(N) is still alive — the refusal
//      is non-destructive.
//
//   3. TestUpgradePath_PostBootstrap_FreshAcquireDaemonLockRefuses —
//      The "subsequent test-bench invocation" check from spec § Component
//      C acceptance bullet "A subsequent test-bench invocation of
//      AcquireDaemonLock from a fresh process refuses with
//      ErrDaemonLockHeld". Clean bootstrap (no v(N) prior daemon),
//      then call AcquireDaemonLock from the test goroutine and assert
//      it refuses. The refusal is the layered-enforcement path: either
//      pre-check (daemon.pid references the live saver-pane daemon) or
//      flock EWOULDBLOCK (if daemon.pid is somehow stale; both paths
//      return ErrDaemonLockHeld so errors.Is is the load-bearing
//      assertion).
//
// Form chosen: in-process construction of the production-shape A+B+C
// wiring via `bootstrapadapter.NewOrphanSweeper` + the canonical
// `tmux.BootstrapPortalSaver` helper. The task brief permits either a
// direct orchestrator invocation or a portal-open subprocess; the
// direct path is cleaner because it does not need TMUX_SOCK env
// plumbing to point a child portal binary at the isolated tmux server.
// The orchestrator itself is not re-constructed here (the saver
// adapter is cmd-private), but the load-bearing production steps for
// the upgrade-path scenario are exactly SweepOrphanDaemons +
// BootstrapPortalSaver — invoking both directly faithfully models the
// v(N+1) bootstrap.
//
// Edge cases:
//   - v(N) exits between bootstrap entry and AcquireDaemonLock — the
//     pre-check sees IdentifyDead, proceeds, and acquire succeeds.
//     Scenario A tolerates this: the post-sweep convergence assertion
//     still holds (count = 1), and the daemon.pid-references-survivor
//     check still passes because the saver-pane daemon's WritePIDFile
//     overwrites the stale v(N) value. The fresh AcquireDaemonLock
//     check then refuses against the survivor.
//   - v(N) daemon.pid is stale (daemon exited earlier without cleanup) —
//     spec contract: pre-check sees dead PID, proceeds. Scenario B's
//     refusal is not exercised in this branch; this is documented but
//     not separately tested here because Scenario B requires a LIVE
//     v(N) to drive the pre-check path.
//   - pgrep not on PATH — every test invokes skipIfNoPgrep (shared with
//     orphan_sweep_integration_test.go) and clean-skips with a
//     diagnostic reason.
//
// Host-noise mitigation, isolated state env, and pgrep helpers are
// shared with orphan_sweep_integration_test.go in the same
// `bootstrap_test` package. Reap-cleanup uses
// portaltest.RegisterSubprocessCleanup (promoted from the formerly
// per-package helpers).
// Logger capture uses `bootstrap.RecordingLogger` (exported from the
// internal `package bootstrap` test file).
//
// No t.Parallel: the cmd-package convention (mock-injection via package-
// level mutable state cleaned up by t.Cleanup) applies here too.

package bootstrap_test

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// upgradePathPGrepConvergenceTimeout bounds the post-bootstrap pgrep
// convergence poll. Spec § Component C acceptance "Upgrade-path
// scenario" cites "6 s of bootstrap entering EnsureSaver (Component
// A's escalation budget + Component B's sweep latency)". Sized to
// match that budget verbatim.
const upgradePathPGrepConvergenceTimeout = 6 * time.Second

// upgradePathPIDFileTimeout bounds the wait for a spawned daemon to
// write daemon.pid. The daemon does this between flock acquire and
// the first tick (~ms), but slow CI machines need margin. 3 s matches
// the existing daemon-startup poll budget in
// internal/tmux/portal_saver_endstate_integration_test.go's
// endStateReadyTimeout pattern (with 1 s extra margin because the
// upgrade-path test stacks two daemon startups in series).
const upgradePathPIDFileTimeout = 3 * time.Second

// upgradePathPIDFilePollTick mirrors the readiness-barrier cadence used
// by the rest of the integration-test suite (50 ms).
const upgradePathPIDFilePollTick = 50 * time.Millisecond

// upgradePathNonDestructiveSettleWindow is the post-refusal observation
// window for Scenario B's "v(N) STILL alive after AcquireDaemonLock
// returns ErrDaemonLockHeld" assertion. 200 ms mirrors the scrollback
// no-final-flush settle window — long enough to catch a delayed kill
// syscall, short enough not to burn budget.
const upgradePathNonDestructiveSettleWindow = 200 * time.Millisecond

// TestUpgradePath_TwoBinary_AllComponentsCompose exercises spec §
// Component C acceptance "Upgrade-path scenario" with the full A+B+C
// composition. See the file-header comment for the scenario shape.
func TestUpgradePath_TwoBinary_AllComponentsCompose(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	skipIfNoPgrep(t)
	_ = portalbintest.StagePortalBinary(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-upgrade-abc-")
	client := sock.Client()

	// Spawn the v(N) daemon as a direct test child — NOT via tmux. This
	// simulates the "old daemon already running" shape the spec calls
	// out: the prior binary launched a saver-pane daemon, then the
	// _portal-saver session was torn down (or never existed on this
	// tmux server) while the daemon kept running. From the v(N+1)
	// bootstrap's perspective this is an orphan: a live `portal state
	// daemon` not bound to any saver pane on this server.
	vNEnv := append([]string{}, envSlice...)
	vNEnv = append(vNEnv, "PORTAL_STATE_DIR="+stateDir)
	vN := exec.Command("portal", "state", "daemon")
	vN.Env = vNEnv
	if err := vN.Start(); err != nil {
		t.Fatalf("start v(N) daemon: %v", err)
	}
	vNPID := vN.Process.Pid
	_ = portaltest.RegisterSubprocessCleanup(t, vN)

	// Wait until v(N) writes daemon.pid. This is the precondition the
	// upgrade-path scenario hinges on: the pre-acquire check in
	// Component C reads daemon.pid, so the file MUST exist before the
	// v(N+1) bootstrap fires for the scenario to faithfully model the
	// "two-binary landmine".
	waitForDaemonPID(t, stateDir, vNPID)

	// Drive the v(N+1) bootstrap. We invoke the load-bearing production
	// steps (SweepOrphanDaemons + BootstrapPortalSaver) directly rather
	// than reconstructing the full Orchestrator because the saver
	// adapter lives in cmd/ (unreachable from this package without a
	// circular import). The two steps in sequence are exactly what the
	// orchestrator runs at steps 4-5 of the bootstrap pipeline — see
	// cmd/bootstrap/bootstrap.go's Run for the canonical ordering.
	//
	// Component A (kill-barrier escalation) fires inside
	// BootstrapPortalSaver's saver-restart path when an unhealthy
	// saver is detected; B fires here via the orphan sweeper; C fires
	// inside the saver-pane daemon's AcquireDaemonLock call. All three
	// compose against the same isolated state dir.
	sweeper := bootstrapadapter.NewOrphanSweeper(client, nil)
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error: %v", err)
	}
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v", err)
	}

	// Poll pgrep -fxc until it converges to 1 within the 6 s budget.
	// On timeout, surface the current pgrep snapshot for diagnosis.
	if !waitForPgrepCount(t, 1, upgradePathPGrepConvergenceTimeout) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("pgrep -fxc did not converge to 1 within %s\n"+
			"  v(N) PID: %d (alive=%v)\n"+
			"  current pgrep snapshot: %v\n"+
			"  state dir: %s",
			upgradePathPGrepConvergenceTimeout,
			vNPID, pidAlive(vNPID),
			pids, stateDir)
	}

	// Survivor identity: the sole remaining daemon must NOT be the
	// original v(N) PID. The survivor is the saver-pane daemon
	// (re-respawned by BootstrapPortalSaver), so the test that the v(N)
	// orphan was swept reduces to PID inequality.
	survivors, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("post-bootstrap pgrep snapshot: %v", err)
	}
	if len(survivors) != 1 {
		t.Fatalf("post-bootstrap: expected exactly 1 daemon, got %d: %v", len(survivors), survivors)
	}
	survivorPID := survivors[0]
	if survivorPID == vNPID {
		t.Fatalf("post-bootstrap: survivor PID %d equals original v(N) PID %d — "+
			"the v(N) orphan was NOT swept (Component B regression)",
			survivorPID, vNPID)
	}

	// daemon.pid on disk must reference the survivor. The saver-pane
	// daemon's WritePIDFile runs as the next statement after the
	// successful AcquireDaemonLock return (spec § Component C, step 4
	// "Post-acquire daemon.pid write") — by the time pgrep converges,
	// the survivor's PID must be on disk.
	pidOnDisk, err := state.ReadPIDFile(stateDir)
	if err != nil {
		t.Fatalf("ReadPIDFile after bootstrap: %v", err)
	}
	if pidOnDisk != survivorPID {
		t.Fatalf("daemon.pid on disk = %d; want survivor PID %d "+
			"(stale daemon.pid from v(N) was not overwritten)",
			pidOnDisk, survivorPID)
	}

	// Fresh AcquireDaemonLock from the test goroutine must refuse with
	// ErrDaemonLockHeld. The pre-check fires on the survivor's live
	// PID — daemon.pid references it, IdentifyDaemon classifies it as
	// IdentifyIsPortalDaemon, and the pre-check returns
	// ErrDaemonLockHeld without opening daemon.lock.
	//
	// The returned fd is nil on the error path (per AcquireDaemonLock's
	// contract); no Close is needed.
	fd, acquireErr := state.AcquireDaemonLock(stateDir)
	if fd != nil {
		// Defensive: if some future regression returns a non-nil fd
		// alongside the error, close it so we do not leak the lock fd
		// past the test.
		_ = fd.Close()
	}
	if !errors.Is(acquireErr, state.ErrDaemonLockHeld) {
		t.Fatalf("AcquireDaemonLock from test goroutine = %v; want ErrDaemonLockHeld "+
			"(Component C pre-check should fire on the live survivor PID %d)",
			acquireErr, survivorPID)
	}
}

// TestUpgradePath_ComponentC_IsolatedRefusesCleanly exercises Component C
// in isolation: no orphan sweep, no saver bootstrap. The pre-acquire
// check must refuse cleanly against a live v(N) daemon without
// destroying it. See the file-header comment for the scenario shape.
func TestUpgradePath_ComponentC_IsolatedRefusesCleanly(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	skipIfNoPgrep(t)
	_ = portalbintest.StagePortalBinary(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// Note: a tmux server is not strictly required for this scenario
	// (we do not invoke any tmux-touching bootstrap step), but the
	// daemon's tick loop probes tmux for saver membership. Creating an
	// isolated server keeps the daemon's first-tick environment
	// consistent with the rest of the integration suite — without it,
	// the daemon's tmux probes fail and emit warnings that are
	// irrelevant to the pre-check assertion under test.
	_ = tmuxtest.New(t, "ptl-upgrade-c-iso-")

	// Spawn the v(N) daemon. No orphan sweeper runs against it; no
	// saver bootstrap fires. The daemon will tick indefinitely (or
	// self-eject via Component D after ~3 s of saver-absence), so the
	// AcquireDaemonLock + 200 ms settle window MUST land inside that
	// hysteresis budget.
	vNEnv := append([]string{}, envSlice...)
	vNEnv = append(vNEnv, "PORTAL_STATE_DIR="+stateDir)
	vN := exec.Command("portal", "state", "daemon")
	vN.Env = vNEnv
	if err := vN.Start(); err != nil {
		t.Fatalf("start v(N) daemon: %v", err)
	}
	vNPID := vN.Process.Pid
	_ = portaltest.RegisterSubprocessCleanup(t, vN)

	// Wait until v(N) writes daemon.pid AND IdentifyDaemon confirms its
	// identity. Without IdentifyDaemon confirmation the pre-check
	// would proceed (treating the PID as "no holder") and the
	// assertion would fail spuriously.
	waitForDaemonPID(t, stateDir, vNPID)
	waitForIdentifyDaemon(t, vNPID)

	// Call AcquireDaemonLock from the test goroutine. The pre-check
	// reads daemon.pid (= vNPID), identifies it as a live portal
	// daemon, and returns ErrDaemonLockHeld WITHOUT opening
	// daemon.lock.
	fd, acquireErr := state.AcquireDaemonLock(stateDir)
	if fd != nil {
		_ = fd.Close()
	}
	if !errors.Is(acquireErr, state.ErrDaemonLockHeld) {
		t.Fatalf("AcquireDaemonLock against live v(N) PID %d = %v; want ErrDaemonLockHeld "+
			"(Component C pre-check should fire)",
			vNPID, acquireErr)
	}

	// Non-destructive observation window. Any erroneous kill or other
	// destructive coexistence side-effect from the refused acquire
	// would have landed by now. Assert v(N) is still alive via
	// IsProcessAlive — the spec contract is that the refused
	// acquire does NOT signal the live holder.
	time.Sleep(upgradePathNonDestructiveSettleWindow)

	if !state.IsProcessAlive(vNPID) {
		t.Fatalf("v(N) PID %d is no longer alive %s after AcquireDaemonLock refusal — "+
			"the pre-check appears to have signalled the live holder "+
			"(Component C destructive-coexistence violation)",
			vNPID, upgradePathNonDestructiveSettleWindow)
	}
}

// TestUpgradePath_PostBootstrap_FreshAcquireDaemonLockRefuses exercises
// spec § Component C acceptance "A subsequent test-bench invocation of
// AcquireDaemonLock from a fresh process refuses with ErrDaemonLockHeld
// (Component C pre-check verifies on the live state)". The difference
// from Scenario A is the starting state: no v(N) orphan, just a clean
// bootstrap. The refusal here is the steady-state singleton invariant,
// not the upgrade-path landmine.
func TestUpgradePath_PostBootstrap_FreshAcquireDaemonLockRefuses(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	skipIfNoPgrep(t)
	_ = portalbintest.StagePortalBinary(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-upgrade-fresh-")
	client := sock.Client()

	// Clean bootstrap: no v(N), just BootstrapPortalSaver. After this
	// returns, the saver-pane daemon holds the lock and has written
	// daemon.pid with its own PID.
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v", err)
	}

	// Wait until the saver-pane daemon writes daemon.pid — the
	// readiness barrier inside BootstrapPortalSaver already waits for
	// this, but polling here is defensive against future barrier
	// changes that might shorten the wait.
	saverPID := waitForSaverPanePID(t, sock)
	waitForDaemonPID(t, stateDir, saverPID)

	// Fresh AcquireDaemonLock from the test goroutine. The pre-check
	// fires on the live saver-pane PID and returns ErrDaemonLockHeld
	// WITHOUT opening daemon.lock. If the pre-check is somehow
	// bypassed (e.g. daemon.pid not yet written), the flock
	// EWOULDBLOCK fallback returns the same sentinel — both paths
	// satisfy errors.Is(_, ErrDaemonLockHeld), so the assertion holds
	// under either layered-enforcement branch.
	fd, acquireErr := state.AcquireDaemonLock(stateDir)
	if fd != nil {
		_ = fd.Close()
	}
	if !errors.Is(acquireErr, state.ErrDaemonLockHeld) {
		t.Fatalf("AcquireDaemonLock from fresh test goroutine = %v; want ErrDaemonLockHeld "+
			"(saver PID = %d; layered-enforcement should refuse via pre-check or flock)",
			acquireErr, saverPID)
	}
}

// waitForDaemonPID polls <stateDir>/daemon.pid until it exists and
// contains the expected PID, bounded by upgradePathPIDFileTimeout. Fails
// the test on timeout with a diagnostic citing the last observed value.
// The expected-PID check guards against a stale daemon.pid from a prior
// test bleeding into the assertion.
func waitForDaemonPID(t *testing.T, stateDir string, expectedPID int) {
	t.Helper()
	var lastPID int
	var lastErr error
	ok := tmuxtest.PollUntil(t, upgradePathPIDFileTimeout, upgradePathPIDFilePollTick, func() bool {
		pid, err := state.ReadPIDFile(stateDir)
		lastPID = pid
		lastErr = err
		if err != nil {
			return false
		}
		return pid == expectedPID
	})
	if !ok {
		t.Fatalf("daemon.pid did not converge to expected PID %d within %s\n"+
			"  last read: pid=%d err=%v\n"+
			"  state dir: %s",
			expectedPID, upgradePathPIDFileTimeout, lastPID, lastErr, stateDir)
	}
}

// waitForIdentifyDaemon polls state.IdentifyDaemon(pid) until the
// classification is IdentifyIsPortalDaemon, bounded by
// upgradePathPIDFileTimeout. Fails the test on timeout. Used by
// Scenario B to ensure the pre-check will see an identity-checkable
// holder when it reads daemon.pid — without this, ps may briefly
// classify the daemon as "in-flight" during early startup and the
// pre-check would (correctly per spec) treat the PID as "no holder",
// causing the assertion to fail spuriously.
func waitForIdentifyDaemon(t *testing.T, pid int) {
	t.Helper()
	var lastResult state.IdentifyResult
	var lastErr error
	ok := tmuxtest.PollUntil(t, upgradePathPIDFileTimeout, upgradePathPIDFilePollTick, func() bool {
		result, err := state.IdentifyDaemon(pid)
		lastResult = result
		lastErr = err
		if err != nil {
			return false
		}
		return result == state.IdentifyIsPortalDaemon
	})
	if !ok {
		t.Fatalf("IdentifyDaemon(%d) did not converge to IdentifyIsPortalDaemon within %s\n"+
			"  last result: %v err=%v",
			pid, upgradePathPIDFileTimeout, lastResult, lastErr)
	}
}
