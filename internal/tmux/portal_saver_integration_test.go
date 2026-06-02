package tmux_test

// Real-tmux integration tests for the saver/daemon contract. This file
// holds three load-bearing fixtures, each pinning a distinct invariant:
//
//  1. TestBootstrapPortalSaver_PreservesSingletonInvariantAcrossRecycle —
//     singleton invariant from the multiple-state-daemons-running-
//     concurrently fix; back-to-back EnsurePortalSaverVersion with a
//     forced version mismatch must leave N==1 daemons per stateDir.
//  2. TestEnsurePortalSaverVersion_AliveAndVersionAbsent_NoKill —
//     Defect 1 user-visible contract from saver-kill-respawn-loop-
//     leaks-daemons; alive daemon + absent daemon.version survives
//     bootstrap without firing the kill barrier and the file is
//     repaired defensively.
//  3. TestBootstrapPortalSaver_LockContention_CascadeChainReachable —
//     fault-injection regression guard for the lock-loser-pane-exit →
//     session-destroy → SetSessionOption cascade; only the natural
//     trigger is eliminated by the fix.
//
// Why these tests exist despite seam-level coverage:
//
//   - Existing BootstrapAliveCheck seam-level unit tests can fix a
//     pidfile and probe it, but cannot model "what happens when the
//     pidfile is overwritten while the prior daemon still runs."
//   - Kill-barrier unit tests stub IsProcessAlive and never observe a
//     real daemon process. They prove the barrier polls correctly;
//     they do not prove a respawned daemon actually displaces the
//     prior one without leaving an orphan attached to the tmux server.
//   - The singleton test reproduces the original-bug shape (back-to-
//     back EnsurePortalSaverVersion with a forced version mismatch
//     between them) and asserts N==1 daemons per stateDir after both
//     calls return.
//
// Skip behaviour:
//
//   - tmuxtest.SkipIfNoTmux skips when tmux is not on PATH (CI lanes
//     without tmux installed).
//   - The test also requires a `portal` binary on PATH because tmux's
//     new-session shell-command "portal state daemon" needs to resolve
//     a real binary that opens the state directory and acquires the
//     lock. The test builds the binary into a t.TempDir and PATH-
//     prepends so the test-spawned tmux server inherits the modified
//     PATH and the daemon resolves at exec time.
//
// Cleanup:
//
//   - tmuxtest.New registers a t.Cleanup that issues kill-server on
//     the isolated socket, which SIGHUPs the daemon. Combined with
//     t.TempDir's auto-cleanup of stateDir, no test artefacts leak
//     beyond the test process.
//
// No t.Parallel: the cmd-package convention (mock-injection via
// package-level mutable state cleaned up by t.Cleanup) applies here
// too — t.Parallel is forbidden across the portal test suite.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// singletonRecycleTimeout is the upper bound for individual polling
// loops inside the test. Sized to comfortably exceed the barrier's
// 5-second worst-case timeout under default settings: if the daemon
// or the recycle path stalls longer than this, the test fails with a
// diagnostic dump rather than hanging indefinitely.
const singletonRecycleTimeout = 5 * time.Second

// daemonPidPollInterval is the cadence at which the test re-polls
// daemon.pid existence and daemon liveness. 50 ms is short enough to
// observe sub-second daemon-startup races without busy-spinning.
const daemonPidPollInterval = 50 * time.Millisecond

// TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle is the
// real-tmux integration test gating spec § Acceptance Criteria →
// Singleton invariant. It exercises the full kill-respawn cycle via
// EnsurePortalSaverVersion against an isolated tmux server and a
// real-built portal binary on PATH, then asserts exactly one
// `portal state daemon` process is alive per state directory after
// the recycle completes.
//
// Flow:
//
//  1. Skip if tmux is not on PATH (CI without tmux).
//  2. Skip if a portal binary cannot be built into a t.TempDir and
//     PATH-prepended (e.g. the dev-machine `go` toolchain is missing
//     or the build itself fails). This is a clean skip, not a
//     failure: the test exercises a real subprocess and is meaningful
//     only when the binary can actually be spawned.
//  3. Stand up an isolated tmux server via tmuxtest.New.
//  4. First EnsurePortalSaverVersion("v-test-1") — creates the saver
//     session, daemon spawns, lock is acquired, daemon.pid is
//     written, daemon.version records "v-test-1".
//  5. Wait for the _portal-saver session to appear and for
//     daemon.pid to exist.
//  6. Capture the first daemon's PID via state.ReadPIDFile so the
//     post-recycle assertion can prove the prior daemon exited.
//  7. Directly overwrite daemon.version with a different value
//     ("v-test-0-old") so the second EnsurePortalSaverVersion call
//     observes a real version mismatch (no new test seam — exercises
//     the production shouldKillSaverOnVersionDecision comparison).
//  8. Second EnsurePortalSaverVersion("v-test-1") — triggers the
//     mismatch branch, which invokes the synchronous kill barrier,
//     waits for the prior daemon to exit, kills the saver session,
//     and respawns. The new daemon acquires the lock cleanly because
//     the barrier waited for the prior daemon's lock release.
//  9. Wait for the _portal-saver session to reappear and for
//     daemon.pid to point at a new, live PID.
//  10. Assert the singleton invariant: the post-recycle daemon.pid
//     PID is alive, AND the prior PID is no longer alive. This is
//     equivalent to "exactly one daemon per stateDir" — the spec's
//     load-bearing structural property — and is more robust than a
//     host-wide pgrep that might race against shell wrappers or
//     concurrent test runs.
//
// On failure: dumps the captured server PID, prior and current
// daemon.pid contents, daemon.version contents, and a pgrep listing
// of `portal state daemon` processes anywhere on the system so the
// diagnostic surface mirrors the manual verification protocol in the
// spec.
func TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// Build a portal binary and PATH-prepend so the daemon resolves
	// at exec time inside the test-spawned tmux server. The build is
	// gated separately from the SkipIfNoTmux check because a dev
	// machine without `go` on PATH (or a broken build) should skip
	// cleanly rather than fail with a noisy build error — the
	// invariant under test is structural, not "go build works".
	//
	// PATH inheritance: t.Setenv guarantees PATH is restored on test
	// exit. tmuxtest's exec.Command("tmux", ...) inherits the test
	// process's PATH, and the tmux server inherits that, so the
	// daemon resolves on PATH when the shell-command "portal state
	// daemon" is exec'd inside the saver session.
	_ = portalbintest.StagePortalBinary(t)

	dir := t.TempDir()
	// The daemon resolves its state directory via PORTAL_STATE_DIR.
	// Setting it on the test process propagates to the tmux server
	// (forked from this process) and onward to the daemon.
	t.Setenv("PORTAL_STATE_DIR", dir)

	sock := tmuxtest.New(t, "ptl-saver-")
	client := sock.Client()

	// First invocation: create the saver session, daemon starts and
	// acquires the lock. currentVersion "v-test-1" is recorded in
	// daemon.version by the daemon.
	if err := tmux.EnsurePortalSaverVersion(client, dir, "v-test-1"); err != nil {
		t.Fatalf("first EnsurePortalSaverVersion: %v", err)
	}
	sock.WaitForSession(t, tmux.PortalSaverName, singletonRecycleTimeout)

	// Capture pre-recycle server PID for the on-failure diagnostic dump.
	serverPID := captureTmuxServerPID(t, sock)

	// Poll for daemon.pid to exist and point at a live process. The
	// daemon's startup is async wrt the tmux new-session return: the
	// shell-command is exec'd by the tmux server but `EnsureDir →
	// open log → acquire lock → write pid` is sequential inside the
	// daemon binary. 5 s comfortably absorbs cold-start jitter.
	priorPID := waitForLiveDaemon(t, dir, singletonRecycleTimeout)

	// Force a version mismatch by overwriting daemon.version directly.
	// This exercises the real shouldKillSaverOnVersionDecision comparison
	// in the second EnsurePortalSaverVersion call — no test seam, no
	// stubbed mismatch helper.
	if err := state.WriteVersionFile(dir, "v-test-0-old", nil); err != nil {
		t.Fatalf("WriteVersionFile (force mismatch): %v", err)
	}

	// Second invocation: triggers the mismatch branch in
	// EnsurePortalSaverVersion, which routes through the synchronous
	// kill barrier (Task 2-1), kills the saver session, and respawns
	// it via BootstrapPortalSaver. The new daemon acquires the lock
	// cleanly because the barrier waited for the prior daemon's lock
	// release.
	if err := tmux.EnsurePortalSaverVersion(client, dir, "v-test-1"); err != nil {
		t.Fatalf("second EnsurePortalSaverVersion: %v", err)
	}
	sock.WaitForSession(t, tmux.PortalSaverName, singletonRecycleTimeout)

	// Poll for daemon.pid to converge on a *new* live PID, distinct
	// from priorPID. This proves the recycle actually displaced the
	// prior daemon — a bug that left the prior daemon running while
	// the pidfile was overwritten by the new one (the original-bug
	// shape) would fail this assertion.
	currentPID := waitForNewLiveDaemon(t, dir, priorPID, singletonRecycleTimeout)

	// Singleton invariant: the prior daemon must be dead AND the
	// current daemon must be alive. This is equivalent to "exactly
	// one daemon per stateDir" — the structural property the lock
	// guarantees. Asserted here as the load-bearing acceptance for
	// spec § Singleton invariant.
	if state.IsProcessAlive(priorPID) {
		dumpDiagnostics(t, dir, serverPID, priorPID, currentPID,
			"prior daemon (pid=%d) is still alive after recycle — singleton invariant violated", priorPID)
	}
	if !state.IsProcessAlive(currentPID) {
		dumpDiagnostics(t, dir, serverPID, priorPID, currentPID,
			"current daemon (pid=%d) is not alive after recycle — recycle failed to spawn replacement", currentPID)
	}

	// Spec-mandated structural assertion (specification.md § Acceptance
	// Criteria → Singleton invariant; § Test Strategy → Integration test
	// — singleton invariant under real tmux): count children of the tmux
	// server matching the `portal state daemon` argv and assert exactly
	// one. This is the load-bearing assertion shape from the spec — the
	// alive/dead pair above is necessary but not sufficient (it cannot
	// observe orphans that are not pointed at by daemon.pid). pgrep -P
	// over the tmux server PID catches any extra daemon process the
	// tmux server has parented, whether the pidfile knows about it or
	// not.
	//
	// Note on serverPID: the `serverPID` captured at line ~170 is the
	// PID of the *pre-recycle* tmux server. Because the test's second
	// EnsurePortalSaverVersion call kills the `_portal-saver` session,
	// and that session is the only session on the isolated socket, the
	// tmux server itself exits and is respawned by the subsequent
	// BootstrapPortalSaver call. We re-capture the post-recycle server
	// PID here so the pgrep parent filter is keyed on the *currently
	// live* server. Verified by direct probing on darwin/tmux 3.6a:
	// using the pre-recycle serverPID gives `pgrep -P <stalePID>` an
	// exit-1 ("no match") because the PID no longer exists, which would
	// over-fail this assertion in a way unrelated to the singleton
	// invariant under test.
	postRecycleServerPID := captureTmuxServerPID(t, sock)
	count, raw := countDaemonChildren(t, postRecycleServerPID)
	if count != 1 {
		dumpDiagnostics(t, dir, postRecycleServerPID, priorPID, currentPID,
			"expected exactly 1 `portal state daemon` child of tmux server (pid=%d), got %d\npgrep raw output:\n%s",
			postRecycleServerPID, count, raw)
	}
}

// TestEnsurePortalSaverVersion_AliveAndVersionAbsent_NoKill is the
// real-tmux integration test gating spec § Acceptance Criteria →
// Steady-state bootstrap and § Testing Requirements → Integration tests
// #1: with an alive daemon and an absent daemon.version, bootstrap
// completes WITHOUT firing the kill barrier and _portal-saver survives.
//
// This pins the user-visible end-to-end contract of the Defect-1 fix:
// the matrix-and-ordering unit tests (Tasks 1-1, 1-3, 1-4) verify the
// kill-decision logic in isolation, but only a real tmux server with a
// real daemon proves the no-kill branch end-to-end — same daemon
// process before/after, daemon.version repaired defensively, no WARN
// substrings in portal.log.
//
// Flow:
//  1. Skip if tmux or the `portal` binary cannot be staged (mirrors the
//     existing singleton-invariant test's skip surface).
//  2. Stand up an isolated tmux server via tmuxtest.New and spawn the
//     saver via the first EnsurePortalSaverVersion call.
//  3. Wait for daemon liveness and daemon.version presence (the daemon
//     writes its own version on startup after acquiring the lock).
//  4. Capture the daemon PID, then DELETE daemon.version to set up the
//     alive+absent input shape.
//  5. Invoke EnsurePortalSaverVersion(client, dir, "0.5.0-test") — the
//     ACTION under test. With alive=true and readErr=ErrVersionFileAbsent
//     the production path takes the no-kill defensive-write branch.
//  6. Assert the survival contract: nil return, session present, version
//     repaired to exactly "0.5.0-test", PID unchanged (same process),
//     daemon still alive, and no kill/lock-loser/EnsureSaver-failure WARN
//     substrings in portal.log.
func TestEnsurePortalSaverVersion_AliveAndVersionAbsent_NoKill(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// Build portal binary and PATH-prepend so the daemon resolves at
	// exec time inside the test-spawned tmux server. Mirrors the skip
	// shape used by TestEnsurePortalSaverVersion_SingletonInvariantAcrossRecycle.
	_ = portalbintest.StagePortalBinary(t)

	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	sock := tmuxtest.New(t, "ptl-aliveabsent-")
	client := sock.Client()

	// Bootstrap the saver via EnsurePortalSaverVersion; the daemon
	// spawns, acquires the lock, and writes daemon.pid + daemon.version.
	// currentVersion here is the "before" version — the same value is
	// passed to the second invocation below so the matrix row exercised
	// is "alive + absent + (versions would match if present)".
	const currentVersion = "0.5.0-test"
	if err := tmux.EnsurePortalSaverVersion(client, dir, currentVersion); err != nil {
		t.Fatalf("initial EnsurePortalSaverVersion: %v", err)
	}
	sock.WaitForSession(t, tmux.PortalSaverName, singletonRecycleTimeout)

	// Poll until the daemon is alive (daemon.pid points at a live
	// process) AND daemon.version has been written. The version write
	// is async w.r.t. tmux's new-session return — the daemon writes it
	// after acquiring the lock, so this poll absorbs that gap before
	// the test's setup step deletes the file.
	priorPID := waitForLiveDaemon(t, dir, singletonRecycleTimeout)
	waitForVersionFile(t, dir, singletonRecycleTimeout)

	// Set up the alive+absent input shape by removing daemon.version.
	// The daemon stays alive (we did not touch its process), but
	// EnsurePortalSaverVersion's next call will observe
	// ErrVersionFileAbsent on the read and route through the no-kill
	// defensive-write branch.
	if err := os.Remove(state.DaemonVersion(dir)); err != nil {
		t.Fatalf("remove daemon.version: %v", err)
	}

	// ACTION: invoke EnsurePortalSaverVersion with the same daemon
	// alive and daemon.version absent.
	if err := tmux.EnsurePortalSaverVersion(client, dir, currentVersion); err != nil {
		t.Fatalf("EnsurePortalSaverVersion (alive+absent): %v", err)
	}

	// Assertion 2: tmux has-session for _portal-saver succeeds. The
	// session must survive — the no-kill branch must not have routed
	// through killSaverAndWaitForDaemonFn.
	if !client.HasSession(tmux.PortalSaverName) {
		t.Fatalf("_portal-saver session does not exist after EnsurePortalSaverVersion on alive+absent input")
	}

	// Assertion 3: daemon.version exists and equals currentVersion
	// exactly. This is the defensive-write contract from Task 1-4.
	stored, err := state.ReadVersionFile(dir)
	if err != nil {
		t.Fatalf("ReadVersionFile after defensive write: %v", err)
	}
	if stored != currentVersion {
		t.Fatalf("daemon.version contents = %q; want %q", stored, currentVersion)
	}

	// Assertion 4: same daemon PID before and after. If the kill
	// barrier had fired, the daemon would have been killed and a new
	// process spawned with a different PID.
	currentPID, err := state.ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("ReadPIDFile after action: %v", err)
	}
	if currentPID != priorPID {
		t.Fatalf("daemon PID changed: prior=%d current=%d (expected no respawn)",
			priorPID, currentPID)
	}

	// Assertion 5: daemon is still alive. Belt-and-braces alongside
	// the PID-equality check — proves the PID points at a live process
	// rather than a stale entry.
	if !state.DaemonAlive(dir) {
		t.Fatalf("DaemonAlive(%s) = false after action; want true", dir)
	}

	// Assertion 6: portal.log does NOT contain any of the three WARN
	// substrings. If the kill barrier ran, lock contention occurred,
	// or EnsureSaver failed, at least one of these substrings would
	// appear. Absent log file → assertion trivially holds.
	assertNoForbiddenLogSubstrings(t, dir)
}

// TestBootstrapPortalSaver_LockContention_CascadeChainReachable is the
// real-tmux fault-injection regression guard for spec § Testing
// Requirements → Integration tests #3 ("Lock-loser daemon's pane exit
// destroys _portal-saver session") and § Coordination with prior
// bugfix.
//
// Why fault injection: Phase 1 of this bugfix (Defect 1 — alive-check-
// first in EnsurePortalSaverVersion) eliminates the natural trigger
// for the lock-contention cascade (back-to-back bootstraps no longer
// kill-respawn unnecessarily, so the lock-loser path is no longer
// reachable through the production kill-respawn loop). This test
// keeps the cascade observable by forcing the contention via a
// sentinel goroutine that holds daemon.lock for the test duration.
// Post-fix, the test is a permanent regression guard on the cascade
// chain (lock-loser exits → pane exits → tmux destroys
// _portal-saver → SetSessionOption returns "no such session"), not
// on the conditions that trigger it.
//
// Flow:
//
//  1. Skip if tmux is not on PATH or a portal binary cannot be built.
//  2. Sentinel goroutine acquires daemon.lock via state.AcquireDaemonLock
//     and signals readiness on a closed-on-ready channel. The lock fd
//     is released via t.Cleanup BEFORE any code path that could
//     t.Fatal — otherwise lock leaks across tests in the same
//     binary.
//  3. Stand up an isolated tmux server via tmuxtest.New.
//  4. Invoke BootstrapPortalSaver(client, stateDir). The session is
//     created and tmux exec's `portal state daemon` as the pane
//     process. The daemon binary calls AcquireDaemonLock, fails with
//     ErrDaemonLockHeld (sentinel holds it), logs a single WARN line,
//     and exits 0 (the lock-loser path).
//  5. Poll state.DaemonAlive(stateDir) until it returns false within a
//     1s ceiling at 50ms tick. Lock-loser daemons exit before writing
//     daemon.pid, so DaemonAlive is typically false from the first
//     poll — that observation also satisfies the assertion.
//  6. Poll tmux has-session for _portal-saver until it returns
//     failure within a 2s ceiling at 100ms tick. The pane process
//     (the lock-loser daemon) exits → tmux destroys the window →
//     last-window-closed destroys the session.
//  7. Assert SetSessionOption(_portal-saver, destroy-unattached, off)
//     returns an error whose string contains BOTH the non-zero exit
//     fragment "exit 1" AND "no such session". Both substrings are
//     required so an unrelated exit-1 (e.g. permission denied on a
//     different surface) does not produce a false-positive pass. (The
//     boundary-class-2 commander wrap renders the exit code as
//     "exit <N>"; the rendered format is not a public contract.)
//
// Regression-watch suites that exercise adjacent daemon/restore
// surfaces and must remain green alongside this test (per spec
// § Risk & Rollout → Coordination):
//
//   - multiple-state-daemons-running-concurrently — daemon.lock flock
//     + killSaverAndWaitForDaemon barrier; this test's sentinel reuses
//     state.AcquireDaemonLock from that suite.
//   - daemon-merge-reintroduces-dead-sessions — structural-index merge
//     in the daemon's commit pipeline.
//   - killed-sessions-resurrect-on-restart — bootstrap-side restore
//     decisions for explicitly killed sessions.
func TestBootstrapPortalSaver_LockContention_CascadeChainReachable(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	_ = portalbintest.StagePortalBinary(t)

	dir := t.TempDir()
	// PORTAL_STATE_DIR propagates to the tmux server (forked from this
	// process) and onward to the daemon binary so AcquireDaemonLock
	// contends against the SAME daemon.lock path the sentinel holds.
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Sentinel: acquire daemon.lock BEFORE BootstrapPortalSaver so the
	// spawned daemon process is guaranteed to lose the lock race. The
	// closed-on-ready chan is the synchronisation primitive — no
	// time.Sleep, no time-based wait. t.Cleanup releases the lock fd;
	// register the cleanup BEFORE any code path that could t.Fatal so
	// the lock is never leaked across tests in the same binary.
	//
	// Memory ordering: sentinelErr and sentinelFile are written by the
	// goroutine and read by the parent. close(ready) establishes the
	// happens-before edge under Go's memory model (a receive from a
	// channel happens after the corresponding close), so no mutex is
	// needed on these single-shot writes.
	ready := make(chan struct{})
	var sentinelErr error
	var sentinelFile *os.File
	go func() {
		f, err := state.AcquireDaemonLock(dir)
		if err != nil {
			sentinelErr = err
			close(ready)
			return
		}
		sentinelFile = f
		close(ready)
	}()
	<-ready
	if sentinelErr != nil {
		t.Fatalf("sentinel AcquireDaemonLock: %v", sentinelErr)
	}
	if sentinelFile == nil {
		t.Fatal("sentinel returned nil *os.File without error")
	}
	t.Cleanup(func() {
		// Closing the fd releases the kernel-side flock, making the
		// daemon.lock acquirable by subsequent tests in the same
		// binary.
		_ = sentinelFile.Close()
	})

	sock := tmuxtest.New(t, "ptl-cascade-")
	client := sock.Client()

	// Start a long-lived holder session FIRST so the tmux server
	// survives _portal-saver's destruction. Without a second session,
	// tmux shuts the server down when the only session dies, and
	// subsequent SetSessionOption calls hit "no server running" rather
	// than the "no such session" failure the cascade chain is supposed
	// to surface. The holder runs `sleep infinity` so its pane never
	// exits during the test; tmuxtest.New's kill-server cleanup tears
	// it down at test end.
	if err := client.NewDetachedSessionNoCwd("_cascade-holder", "sleep infinity"); err != nil {
		t.Fatalf("create holder session: %v", err)
	}

	// ACTION: BootstrapPortalSaver creates _portal-saver with
	// `portal state daemon` as the pane command. The daemon attempts
	// AcquireDaemonLock, fails with ErrDaemonLockHeld (sentinel holds
	// it), logs a single WARN and exits 0. BootstrapPortalSaver itself
	// returns nil — its contract is "session created and
	// destroy-unattached set", which both succeed before the pane
	// process completes its lock contention. There is, however, a
	// race where the lock-loser daemon exits AND tmux destroys the
	// session BEFORE SetSessionOption inside BootstrapPortalSaver
	// runs — in which case BootstrapPortalSaver itself returns the
	// cascade-chain error. Both outcomes are acceptable for this
	// regression guard (the cascade chain is observable either way).
	_ = tmux.BootstrapPortalSaver(client, dir)

	// Assertion 1: daemon's process is not alive within 1s. Lock-loser
	// daemons typically exit before writing daemon.pid; DaemonAlive
	// returns false from the first poll in that case. The poll
	// tolerates both shapes (file absent → false; file present
	// pointing at dead PID → false).
	if !waitForDaemonNotAlive(t, dir, 1*time.Second, 50*time.Millisecond) {
		t.Fatalf("state.DaemonAlive(%s) did not return false within 1s "+
			"(lock-loser daemon should exit before writing daemon.pid or "+
			"exit shortly after writing it)", dir)
	}

	// Assertion 2: tmux has-session for _portal-saver returns failure
	// within 2s at 100ms tick. The pane process exiting → tmux
	// destroys the window → last-window-closed destroys the session.
	if !waitForSessionAbsent(t, client, tmux.PortalSaverName, 2*time.Second, 100*time.Millisecond) {
		t.Fatalf("_portal-saver session did not disappear within 2s " +
			"(pane process should have exited, destroying the session)")
	}

	// Assertion 3: SetSessionOption(_portal-saver, destroy-unattached,
	// off) returns an error containing BOTH the non-zero exit fragment
	// ("exit 1") AND "no such session". Asserting both substrings
	// independently rules out a false positive from any other exit-1
	// error (permission denied, malformed args, etc.). The boundary-class-2
	// commander wrap renders the exit code as "exit <N>" (derived from
	// *exec.ExitError.ExitCode()); the rendered format is not a public
	// contract — callers discriminate via errors.As/Stderr/Args.
	err := client.SetSessionOption(tmux.PortalSaverName, "destroy-unattached", "off")
	if err == nil {
		t.Fatalf("SetSessionOption returned nil; expected error after session destruction")
	}
	msg := err.Error()
	if !strings.Contains(msg, "exit 1") {
		t.Fatalf("SetSessionOption error %q does not contain %q", msg, "exit 1")
	}
	if !strings.Contains(msg, "no such session") {
		t.Fatalf("SetSessionOption error %q does not contain %q", msg, "no such session")
	}
}

// waitForDaemonNotAlive polls state.DaemonAlive(dir) until it returns
// false or the timeout elapses. Returns true if DaemonAlive became
// false within the deadline, false on timeout. Used by the lock-
// contention cascade test where the lock-loser daemon either never
// writes daemon.pid (DaemonAlive false from the start) or writes it
// and exits shortly after.
func waitForDaemonNotAlive(t *testing.T, dir string, timeout, tick time.Duration) bool {
	t.Helper()
	return tmuxtest.PollUntil(t, timeout, tick, func() bool {
		return !state.DaemonAlive(dir)
	})
}

// waitForSessionAbsent polls client.HasSession(name) until it returns
// false or the timeout elapses. Returns true if the session
// disappeared within the deadline. Used by the lock-contention
// cascade test to observe tmux destroying _portal-saver after the
// pane process exits.
func waitForSessionAbsent(t *testing.T, client *tmux.Client, name string, timeout, tick time.Duration) bool {
	t.Helper()
	return tmuxtest.PollUntil(t, timeout, tick, func() bool {
		return !client.HasSession(name)
	})
}

// waitForVersionFile polls until daemon.version exists in dir, or
// fatals the test on timeout. The daemon writes its version
// asynchronously after acquiring the lock, so callers that depend on
// the file being present must poll rather than read once.
func waitForVersionFile(t *testing.T, dir string, timeout time.Duration) {
	t.Helper()
	if tmuxtest.PollUntil(t, timeout, daemonPidPollInterval, func() bool {
		_, err := state.ReadVersionFile(dir)
		return err == nil
	}) {
		return
	}
	t.Fatalf("daemon.version did not appear within %s (state dir=%s)", timeout, dir)
}

// assertNoForbiddenLogSubstrings reads portal.log in dir and fails the
// test if any of the three forbidden WARN substrings (kill-barrier
// timeout, lock contention, or step-5 EnsureSaver failure) appear.
// Absence of portal.log is acceptable — the assertion holds trivially
// because none of the forbidden substrings can be present.
//
// Reads portal.log only — not state.PortalLogOld. The tests using this
// helper are short-lived and never trigger log rotation, so the rotated
// file cannot exist; if a future test exercises a longer-running scenario
// that rotates the log, the helper must be extended to scan both files.
func assertNoForbiddenLogSubstrings(t *testing.T, dir string) {
	t.Helper()
	data, err := os.ReadFile(state.PortalLog(dir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		t.Fatalf("read portal.log: %v", err)
	}
	contents := string(data)
	forbidden := []string{
		"prior daemon (pid=",
		"another daemon holds the lock; exiting",
		"step 5 (EnsureSaver) failed:",
	}
	for _, sub := range forbidden {
		if strings.Contains(contents, sub) {
			t.Fatalf("portal.log contains forbidden substring %q\n--- portal.log ---\n%s",
				sub, contents)
		}
	}
}

// captureTmuxServerPID reads the post-recycle tmux server PID via
// `display-message -p '#{pid}'`. Extracted as a helper because the test
// captures the server PID twice: once early (for the pre-recycle
// diagnostic surface) and once after the recycle (for the pgrep
// parent-filter assertion). Fatals the test if the value cannot be
// parsed as an integer.
func captureTmuxServerPID(t *testing.T, sock *tmuxtest.Socket) int {
	t.Helper()
	out := sock.Run(t, "display-message", "-p", "#{pid}")
	pid, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("parse tmux server PID %q: %v", out, err)
	}
	return pid
}

// countDaemonChildren executes `pgrep -P <serverPID> -f 'portal state
// daemon'` and returns the number of matched PIDs along with the raw
// output for diagnostic dumps. pgrep exit semantics: 0 = match, 1 = no
// match (empty stdout, not an error here), 2+ = pgrep error. Empty
// stdout (the no-match case) returns count 0, not an error; non-empty
// stdout counts newline-terminated PID lines.
func countDaemonChildren(t *testing.T, serverPID int) (int, string) {
	t.Helper()
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(serverPID), "-f", "portal state daemon").Output()
	if err != nil {
		// pgrep exit 1 means "no matches" — surface as count 0 with
		// empty raw output. Any other exit is a pgrep-side problem;
		// surface the error in the raw string so the diagnostic dump
		// makes the cause obvious.
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return 0, ""
		}
		return 0, fmt.Sprintf("pgrep error: %v\nstderr/stdout: %s", err, string(out))
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return 0, ""
	}
	return len(strings.Split(trimmed, "\n")), string(out)
}

// waitForLiveDaemon polls daemon.pid until the file exists, parses to
// a valid PID, and points at a live process; or fails the test on
// timeout. Mirrors the spec's "wait for daemon to be observable"
// primitive used in the kill-barrier unit tests, but against a real
// daemon process rather than a stubbed IsProcessAlive seam.
func waitForLiveDaemon(t *testing.T, dir string, timeout time.Duration) int {
	t.Helper()
	var livePID int
	if tmuxtest.PollUntil(t, timeout, daemonPidPollInterval, func() bool {
		pid, err := state.ReadPIDFile(dir)
		if err == nil && state.IsProcessAlive(pid) {
			livePID = pid
			return true
		}
		return false
	}) {
		return livePID
	}
	t.Fatalf("daemon.pid did not point at a live process within %s "+
		"(state dir=%s)", timeout, dir)
	return 0
}

// waitForNewLiveDaemon polls daemon.pid until the recorded PID is
// distinct from prior, parses cleanly, and points at a live process;
// or fails the test on timeout. The "distinct from prior" gate is
// what proves the recycle actually displaced the prior daemon (vs
// the bug shape where the pidfile is never rewritten because the new
// daemon was never spawned, or — pre-flock — the pidfile is
// overwritten but the prior daemon process lingers).
func waitForNewLiveDaemon(t *testing.T, dir string, prior int, timeout time.Duration) int {
	t.Helper()
	var newPID int
	if tmuxtest.PollUntil(t, timeout, daemonPidPollInterval, func() bool {
		pid, err := state.ReadPIDFile(dir)
		if err == nil && pid != prior && state.IsProcessAlive(pid) {
			newPID = pid
			return true
		}
		return false
	}) {
		return newPID
	}
	t.Fatalf("daemon.pid did not converge on a new live PID (prior=%d) "+
		"within %s (state dir=%s)", prior, timeout, dir)
	return 0
}

// dumpDiagnostics emits the manual-verification-protocol diagnostic
// surface from the spec and fatals the test. Includes:
//
//   - The captured tmux server PID (so the reader can re-run
//     `pgrep -P <server-pid> -f 'portal state daemon'` post-mortem).
//   - Current daemon.pid and daemon.version contents (filesystem
//     state at the moment of failure).
//   - A pgrep listing of all `portal state daemon` processes anywhere
//     on the system (best-effort — `pgrep -fl` exit code 1 = "no
//     match" is not an error here).
//
// Format args are passed through to t.Fatalf at the end so a
// rich-context failure surface reaches the test log in a single
// pass.
func dumpDiagnostics(t *testing.T, dir string, serverPID, priorPID, currentPID int, format string, args ...any) {
	t.Helper()
	var b strings.Builder

	fmt.Fprintf(&b, "tmux server PID: %d\n", serverPID)
	fmt.Fprintf(&b, "prior daemon PID: %d (alive=%v)\n",
		priorPID, state.IsProcessAlive(priorPID))
	fmt.Fprintf(&b, "current daemon PID: %d (alive=%v)\n",
		currentPID, state.IsProcessAlive(currentPID))

	if pidData, err := os.ReadFile(filepath.Join(dir, "daemon.pid")); err == nil {
		fmt.Fprintf(&b, "daemon.pid contents: %q\n", string(pidData))
	} else {
		fmt.Fprintf(&b, "daemon.pid read error: %v\n", err)
	}
	if verData, err := os.ReadFile(filepath.Join(dir, "daemon.version")); err == nil {
		fmt.Fprintf(&b, "daemon.version contents: %q\n", string(verData))
	} else {
		fmt.Fprintf(&b, "daemon.version read error: %v\n", err)
	}

	// pgrep -fl exit semantics: 0 = match (output present), 1 = no
	// match (no output, not an error here), 2+ = pgrep error. Treat
	// the no-match case as "nothing to dump" rather than a test
	// failure on top of the failure we're already reporting.
	out, _ := exec.Command("pgrep", "-fl", "portal state daemon").CombinedOutput()
	fmt.Fprintf(&b, "pgrep -fl 'portal state daemon':\n%s", string(out))

	t.Fatalf(format+"\n\nDiagnostics:\n%s", append(args, b.String())...)
}
