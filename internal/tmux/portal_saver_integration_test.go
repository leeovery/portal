package tmux_test

// Real-tmux integration test for the singleton-invariant acceptance
// criterion of the multiple-state-daemons-running-concurrently fix
// (specification.md § Acceptance Criteria → Singleton invariant). This
// file is the load-bearing verification that the kill barrier (Task
// 2-1, 2-2) plus the daemon-side flock (Task 1-1, 1-2) compose to
// guarantee "at most one `portal state daemon` per state directory"
// across a real recycle.
//
// Why this test exists despite seam-level coverage:
//
//   - Existing BootstrapAliveCheck seam-level unit tests can fix a
//     pidfile and probe it, but cannot model "what happens when the
//     pidfile is overwritten while the prior daemon still runs."
//   - Kill-barrier unit tests stub IsProcessAlive and never observe a
//     real daemon process. They prove the barrier polls correctly;
//     they do not prove a respawned daemon actually displaces the
//     prior one without leaving an orphan attached to the tmux server.
//   - This test reproduces the original-bug shape (back-to-back
//     EnsurePortalSaverVersion with a forced version mismatch between
//     them) and asserts N==1 daemons per stateDir after both calls
//     return. Pre-fix, the recycle would leave an orphan attached to
//     the tmux server's PID; post-fix, the lock + barrier hold N==1.
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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/restoretest"
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
//     the production portalSaverVersionMismatch comparison).
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
	binDir := t.TempDir()
	if err := restoretest.BuildPortalBinary(binDir); err != nil {
		t.Skipf("portal binary build failed; skipping real-daemon integration test: %v", err)
	}
	// PATH inheritance: t.Setenv guarantees PATH is restored on test
	// exit. tmuxtest's exec.Command("tmux", ...) inherits the test
	// process's PATH, and the tmux server inherits that, so the
	// daemon resolves on PATH when the shell-command "portal state
	// daemon" is exec'd inside the saver session.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Verify portal is actually resolvable after the PATH prepend.
	// Belt-and-braces: if anything is wrong with the build or PATH
	// wiring, fail the skip path with a clear message rather than
	// blowing up later inside the recycle loop.
	if _, err := exec.LookPath("portal"); err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

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
	// This exercises the real portalSaverVersionMismatch comparison
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
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pid, err := state.ReadPIDFile(dir)
		if err == nil && state.IsProcessAlive(pid) {
			return pid
		}
		time.Sleep(daemonPidPollInterval)
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
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pid, err := state.ReadPIDFile(dir)
		if err == nil && pid != prior && state.IsProcessAlive(pid) {
			return pid
		}
		time.Sleep(daemonPidPollInterval)
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
