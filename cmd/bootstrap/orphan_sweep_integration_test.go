//go:build integration

// Real-tmux integration tests for spec § Component B acceptance bullets
// 1–3 — the bootstrap-time orphan-daemon sweep that closes the multi-
// orphan gap Component A cannot reach on its own.
//
// Three scenarios are pinned end-to-end against a real tmux server,
// real `portal state daemon` subprocesses, and the production
// `*bootstrap.OrphanSweepCore` wired by `bootstrapadapter.NewOrphanSweeper`:
//
//   1. ThreeDaemonsConvergeToOne — bootstrap `_portal-saver` (one
//      legitimate daemon), spawn two additional orphan daemons against
//      the same isolated state dir, invoke SweepOrphanDaemons, and
//      verify `pgrep -fxc '^portal state daemon( |$)'` converges to 1
//      with the survivor PID equal to the saver pane's pane_pid.
//      Mirrors the reporter's "three concurrent daemons" install shape.
//   2. CleanStateZeroSignals — bootstrap `_portal-saver` only (no
//      orphans), invoke SweepOrphanDaemons against a recording
//      Logger, and verify zero `"sweep: killed orphan daemon"` INFO
//      entries plus a still-singleton pgrep count. The clean-state
//      audit-log invariant.
//   3. RecycledPIDRefusal — spawn a non-daemon `sleep` process,
//      override the production Pgrep seam to inject the sleep PID
//      into the candidate set, and verify the real
//      `state.IdentifyDaemon` returns IdentifyNotPortalDaemon and the
//      sweep does NOT SIGKILL the sleep process. Pins the recycled-PID
//      safety invariant.
//
// Host-noise mitigation:
//   `portaltest.IsolateStateForTest` registers a backstop that
//   snapshots the developer's real state directory on entry and
//   re-snapshots on test exit to catch any leakage from the spawned
//   daemons. On a dev box running a live `portal state daemon`
//   against `~/.config/portal/state/`, that live daemon mutates
//   files in the snapshot window — producing a false-positive
//   backstop failure that has nothing to do with the test. The
//   helper folds the HOME=<tempdir> / XDG_CONFIG_HOME="" scrub in
//   internally (BEFORE its pre-snapshot) so the backstop targets a
//   quiet tempdir. PORTAL_STATE_DIR is then set on the test process
//   so every subprocess (tmux server, daemons) inherits the per-test
//   isolated dir.
//
// No t.Parallel: the cmd-package convention (mock-injection via
// package-level mutable state cleaned up by t.Cleanup) applies here
// too — there are no package-level mocks in this file, but the
// shared tmuxtest/portaltest infrastructure is built around the
// same convention.

package bootstrap_test

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// pgrepConvergenceTimeout bounds the poll window for the pgrep count to
// reach the expected target value (3 at setup time, 1 after sweep). 3 s
// matches the existing integration-test budget pattern in
// internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go
// and is comfortably above the observed SIGKILL→reap latency on
// darwin/arm64 (sub-50 ms) plus daemon-start latency (~hundreds of ms).
const pgrepConvergenceTimeout = 3 * time.Second

// pgrepConvergencePollTick is the cadence at which the pgrep-count poll
// re-shells out to `pgrep`. 50 ms matches the readiness-barrier cadence
// in production wiring and is short enough to observe sub-second
// convergence without busy-spinning.
const pgrepConvergencePollTick = 50 * time.Millisecond

// recycledPIDSettleWindow is the observation window for Scenario C's
// "non-daemon PID is NOT SIGKILLed" assertion. After SweepOrphanDaemons
// returns we wait this long to let any erroneous kill syscall race
// against the sleep process before checking liveness via
// syscall.Kill(pid, 0). 200 ms mirrors the scrollback no-final-flush
// settle window — long enough to catch a delayed kill, short enough
// not to burn budget.
const recycledPIDSettleWindow = 200 * time.Millisecond

// TestSweepOrphanDaemons_Integration_ThreeDaemonsConvergeToOne exercises
// spec § Component B acceptance bullet 1: with N=3 concurrent daemons
// (1 saver-pane + 2 orphans), SweepOrphanDaemons kills the 2 orphans
// and the system converges to 1 daemon — the saver-pane PID.
func TestSweepOrphanDaemons_Integration_ThreeDaemonsConvergeToOne(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	skipIfNoPgrep(t)
	_ = portalbintest.StagePortalBinary(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-sweep3-")
	client := sock.Client()

	// 1. Bootstrap the legitimate saver-pane daemon. After this returns
	//    the daemon-host pane is running `portal state daemon` against
	//    the isolated stateDir.
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v", err)
	}
	saverPID := waitForSaverPanePID(t, sock)

	// 2. Spawn two ADDITIONAL orphan daemons as direct test children.
	//    These are NOT bound to the saver pane — Component B must
	//    detect and kill them based on argv alone (pgrep is
	//    system-wide; the legitimate set is defined by saver-pane PID,
	//    not by state-dir membership).
	//
	//    Each orphan runs against its OWN isolated state dir. Reason:
	//    with three daemons against a single stateDir, Component C's
	//    pre-acquire check and the kernel's flock-per-inode semantics
	//    converge the daemon population to 1 within milliseconds —
	//    the second and third entrants exit cleanly as lock-losers
	//    BEFORE we can observe pgrep -fx == 3. Per-orphan state dirs
	//    decouple lock acquisition from pgrep visibility: each daemon
	//    acquires its own daemon.lock, all three stay live, and
	//    `pgrep -fx '^portal state daemon( |$)'` matches them all
	//    (pgrep argv-matches at the process level, not by environment).
	//
	//    This faithfully models the reporter's broken-install shape —
	//    multiple `portal state daemon` processes the orchestrator
	//    must see and kill — while staying compatible with Component
	//    C's singleton guarantee on the saver's state dir.
	orphan1, _ := portaltest.SpawnIsolatedDaemon(t, envSlice)
	orphan2, _ := portaltest.SpawnIsolatedDaemon(t, envSlice)

	// 3. Precondition: pgrep -fxc must reach 3 before we invoke the
	//    sweep. Without this barrier the sweep can race the orphan
	//    spawns (an orphan's argv may not yet be visible to pgrep when
	//    the sweep runs) and silently green-pass against fewer than N
	//    candidates.
	if !waitForPgrepCount(t, 3, pgrepConvergenceTimeout) {
		// Diagnostic — print live daemon PIDs and the current pgrep
		// output so a partial convergence is debuggable.
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("precondition: pgrep -fxc did not reach 3 within %s\n"+
			"  saverPID: %d\n"+
			"  orphan1.PID: %d (alive=%v)\n"+
			"  orphan2.PID: %d (alive=%v)\n"+
			"  pgrep snapshot: %v\n"+
			"  hint: an orphan may have exited before the sweep — see test diagnostic above",
			pgrepConvergenceTimeout,
			saverPID,
			orphan1.Process.Pid, pidAlive(orphan1.Process.Pid),
			orphan2.Process.Pid, pidAlive(orphan2.Process.Pid),
			pids)
	}

	// 4. Construct the production OrphanSweeper via the adapter and
	//    invoke it.
	sweeper := bootstrapadapter.NewOrphanSweeper(client, nil)
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error (best-effort step must return nil): %v", err)
	}

	// 5. Convergence: pgrep -fxc must reach 1 within the bound.
	if !waitForPgrepCount(t, 1, pgrepConvergenceTimeout) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("post-sweep: pgrep -fxc did not converge to 1 within %s; current pgrep=%v",
			pgrepConvergenceTimeout, pids)
	}

	// 6. Survivor identity: the sole remaining daemon must be the
	//    saver-pane PID. Re-read pane_pid here in case the saver pane
	//    was respawned between setup and now (the "saver pane dies
	//    between setup and sweep" edge case).
	survivors, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("post-sweep pgrep snapshot: %v", err)
	}
	if len(survivors) != 1 {
		t.Fatalf("post-sweep: expected exactly 1 daemon, got %d: %v", len(survivors), survivors)
	}
	currentSaverPID := readSaverPanePID(t, sock)
	if survivors[0] != currentSaverPID {
		t.Fatalf("post-sweep: survivor pid=%d does not match saver pane pid=%d",
			survivors[0], currentSaverPID)
	}
}

// TestSweepOrphanDaemons_Integration_CleanStateZeroSignals exercises
// spec § Component B acceptance bullet 2: on a clean-state bootstrap
// (only the legitimate saver-pane daemon present), SweepOrphanDaemons
// sends zero kill signals and the audit log records zero
// "sweep: killed orphan daemon" entries.
func TestSweepOrphanDaemons_Integration_CleanStateZeroSignals(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	skipIfNoPgrep(t)
	_ = portalbintest.StagePortalBinary(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-sweepclean-")
	client := sock.Client()

	// 1. Bootstrap the legitimate saver-pane daemon — the ONLY daemon
	//    in this scenario.
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v", err)
	}
	_ = waitForSaverPanePID(t, sock)

	// Precondition: pgrep -fxc must already be 1 before the sweep so
	// the post-sweep assertion is unambiguous.
	if !waitForPgrepCount(t, 1, pgrepConvergenceTimeout) {
		pids, _ := portaltest.PgrepPortalDaemons()
		t.Fatalf("precondition: pgrep -fxc did not reach 1 within %s; pgrep=%v",
			pgrepConvergenceTimeout, pids)
	}

	// 2. Wire the production adapter and inject a recording Logger via
	//    type-assertion on the underlying *bootstrap.OrphanSweepCore.
	//    The adapter returns an interface but exposes the concrete
	//    type for exactly this test-time override pattern.
	sweeper := bootstrapadapter.NewOrphanSweeper(client, nil)
	core, ok := sweeper.(*bootstrap.OrphanSweepCore)
	if !ok {
		t.Fatalf("NewOrphanSweeper returned %T; want *bootstrap.OrphanSweepCore "+
			"(needed to inject a recording Logger)", sweeper)
	}
	logger := &bootstrap.RecordingLogger{}
	core.Logger = logger

	// 3. Invoke the sweep.
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error: %v", err)
	}

	// 4. Audit-log invariant: zero "sweep: killed orphan daemon"
	//    entries. The spec is explicit — this is the load-bearing
	//    clean-state assertion.
	const forbidden = "sweep: killed orphan daemon"
	for _, entry := range logger.AllEntries() {
		if strings.Contains(entry, forbidden) {
			t.Fatalf("clean-state sweep emitted forbidden log entry containing %q\n"+
				"  entry: %s\n"+
				"  all entries:\n%s",
				forbidden, entry, strings.Join(logger.AllEntries(), "\n"))
		}
	}

	// 5. Singleton invariant: pgrep -fxc still == 1 (the saver-pane
	//    daemon was not erroneously killed).
	pids, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("post-sweep pgrep: %v", err)
	}
	if len(pids) != 1 {
		t.Fatalf("post-sweep: pgrep returned %d daemons, want 1: %v", len(pids), pids)
	}
}

// TestSweepOrphanDaemons_Integration_RecycledPIDRefusal exercises spec
// § Component B acceptance bullet 3: the identity check prevents
// signalling an unrelated process if the PID has been recycled. We
// stage a non-daemon `sleep` process and override the production
// Pgrep seam to inject its PID into the candidate set; the real
// `state.IdentifyDaemon` must classify it as IdentifyNotPortalDaemon
// and the sweep must NOT SIGKILL it.
func TestSweepOrphanDaemons_Integration_RecycledPIDRefusal(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	skipIfNoPgrep(t)
	_ = portalbintest.StagePortalBinary(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-sweeprecycle-")
	client := sock.Client()

	// 1. Bootstrap the legitimate saver-pane daemon — the
	//    identity-check baseline ("known portal daemon" to skip).
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v", err)
	}
	saverPID := waitForSaverPanePID(t, sock)

	// 2. Spawn the non-daemon process — `sh -c 'exec sleep 30'`. Use
	//    `exec` so the sh wrapper hands off to sleep directly: the
	//    PID of sleep is the PID we inject into the candidate set,
	//    and ps -o args= on that PID returns `sleep 30` (NOT
	//    `portal state daemon`), so state.IdentifyDaemon will return
	//    IdentifyNotPortalDaemon.
	sleeper := exec.Command("sh", "-c", "exec sleep 30")
	if err := sleeper.Start(); err != nil {
		t.Fatalf("start sleep process: %v", err)
	}
	sleeperPID := sleeper.Process.Pid
	reaped := portaltest.RegisterSubprocessCleanup(t, sleeper)

	// 3. Wire the production adapter, then override Pgrep to return
	//    a candidate set that includes both the saver PID (legit) and
	//    the sleeper PID (non-daemon). The Identify seam is left
	//    untouched so the REAL state.IdentifyDaemon runs against
	//    sleeperPID.
	sweeper := bootstrapadapter.NewOrphanSweeper(client, nil)
	core, ok := sweeper.(*bootstrap.OrphanSweepCore)
	if !ok {
		t.Fatalf("NewOrphanSweeper returned %T; want *bootstrap.OrphanSweepCore", sweeper)
	}
	core.Pgrep = func() ([]int, error) {
		return []int{saverPID, sleeperPID}, nil
	}

	// 4. Invoke the sweep.
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error: %v", err)
	}

	// 5. Settle window — any erroneous kill syscall would have landed
	//    by now. Then check liveness via kill(pid, 0).
	time.Sleep(recycledPIDSettleWindow)

	killErr := syscall.Kill(sleeperPID, 0)
	if errors.Is(killErr, syscall.ESRCH) {
		t.Fatalf("sleeper PID %d was killed by SweepOrphanDaemons; "+
			"the identity check failed to refuse the non-daemon PID\n"+
			"  this is a Component B recycled-PID safety violation",
			sleeperPID)
	}
	if killErr != nil {
		// Other errors (EPERM, etc.) are diagnostic noise but the
		// "not killed" invariant still holds — kill(pid, 0) returning
		// anything other than ESRCH means the process exists.
		t.Logf("sleeper PID %d: kill(pid, 0) returned %v (proceeding; non-ESRCH means process still exists)",
			sleeperPID, killErr)
	}

	// Belt-and-braces: the cleanup hook below will SIGKILL the
	// sleeper. Force-close via the reaper channel to ensure no leak.
	_ = reaped
}

// skipIfNoPgrep skips the test cleanly when `pgrep` is not on PATH.
// Linux and darwin both ship pgrep in base; this guard exists for
// minimal containers where procps may be absent.
func skipIfNoPgrep(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pgrep"); err != nil {
		t.Skipf("pgrep not available; skipping orphan-sweep integration test: %v", err)
	}
}

// waitForSaverPanePID polls `_portal-saver`'s pane_pid until it is
// non-zero, bounded by pgrepConvergenceTimeout. Fails the test on
// timeout. Used at setup to assert the saver-pane daemon is live
// before any sweep runs.
func waitForSaverPanePID(t *testing.T, sock *tmuxtest.Socket) int {
	t.Helper()
	var pid int
	ok := tmuxtest.PollUntil(t, pgrepConvergenceTimeout, pgrepConvergencePollTick, func() bool {
		out, err := sock.TryRun("list-panes", "-t", tmux.PortalSaverName, "-F", "#{pane_pid}")
		if err != nil {
			return false
		}
		p, perr := strconv.Atoi(strings.TrimSpace(out))
		if perr != nil || p <= 0 {
			return false
		}
		pid = p
		return true
	})
	if !ok {
		t.Fatalf("saver pane PID did not become observable within %s", pgrepConvergenceTimeout)
	}
	return pid
}

// readSaverPanePID re-reads the saver pane's pane_pid AFTER sweep, with
// a single retry to cover the edge case "saver pane was respawned
// between setup and sweep" called out in the task's edge-case list.
// Fails the test if the second attempt also fails.
func readSaverPanePID(t *testing.T, sock *tmuxtest.Socket) int {
	t.Helper()
	for attempt := 0; attempt < 2; attempt++ {
		out, err := sock.TryRun("list-panes", "-t", tmux.PortalSaverName, "-F", "#{pane_pid}")
		if err == nil {
			p, perr := strconv.Atoi(strings.TrimSpace(out))
			if perr == nil && p > 0 {
				return p
			}
		}
		// Small re-probe gap before retry — covers a transient
		// respawn-pane window.
		time.Sleep(pgrepConvergencePollTick)
	}
	t.Fatalf("saver pane PID unreadable after retry; saver pane may have died between setup and sweep")
	return 0
}

// waitForPgrepCount polls `pgrep -fxc <pattern>` until the count
// reaches the target value or the timeout elapses. Returns true on
// observed match. Diagnostic-free — the caller owns failure shape.
func waitForPgrepCount(t *testing.T, target int, timeout time.Duration) bool {
	t.Helper()
	return tmuxtest.PollUntil(t, timeout, pgrepConvergencePollTick, func() bool {
		pids, err := portaltest.PgrepPortalDaemons()
		if err != nil {
			return false
		}
		return len(pids) == target
	})
}

// pidAlive reports whether the supplied PID exists in the kernel's
// process table via syscall.Kill(pid, 0). Used by failure diagnostics
// in Scenario A to surface the orphan-exit edge case ("orphan
// subprocess exited before sweep — clear diagnostic, not silent skip").
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return !errors.Is(err, syscall.ESRCH)
}

// Compile-time guard: bootstrap.RecordingLogger must satisfy
// bootstrap.Logger so the Scenario B injection compiles even if the
// interface gains methods.
var _ bootstrap.Logger = (*bootstrap.RecordingLogger)(nil)
