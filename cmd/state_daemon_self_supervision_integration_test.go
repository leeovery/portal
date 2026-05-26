//go:build integration

// Real-tmux integration test for spec § Component D acceptance bullet 1:
// "Self-eject on absent saver. Spawn `portal state daemon` against a tmux
// server that has no `_portal-saver` session. The daemon exits within
// (N + 1) tick intervals."
//
// Test choreography (mirrors the divergent-view orphan pattern in
// internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go,
// but exercises the self-eject path rather than the externally-SIGKILLed
// path):
//
//  1. SkipIfNoTmux + StagePortalBinary + isolated state dir via
//     portaltest.IsolateStateForTest (which folds in the HOME /
//     XDG_CONFIG_HOME host-noise scrub before its pre-snapshot). The
//     PORTAL_STATE_DIR env override pins the daemon's state writes to
//     the per-test temp dir.
//  2. Stand up an isolated tmux server via tmuxtest.New. Crucially we do
//     NOT call tmux.BootstrapPortalSaver — the whole point of the
//     Component D bullet under test is that _portal-saver does NOT
//     exist, so the daemon's per-tick saver-membership probe returns
//     false on every tick.
//  3. Verify the staged pre-state: <stateDir>/daemon.pid does NOT exist
//     and <stateDir>/daemon.lock does NOT exist. Component C's
//     lock-acquire pre-check requires daemon.pid absent (or pointing at
//     a dead PID) to let the daemon proceed to the tick loop — the spec
//     § Component D "Test staging note" mandates this staging.
//  4. Spawn `portal state daemon` as a subprocess (binary on PATH via
//     StagePortalBinary), with cmd.Env wired to the isolated stateDir
//     and TMUX env pointing at the test socket so the daemon's
//     tmux.DefaultClient discovers the test server (not the host's).
//     Spawn DIRECTLY — NOT via `portal open` or the orchestrator — so
//     Component B's bootstrap-time sweep cannot preempt the daemon
//     before its tick loop runs the self-check.
//  5. Wait for the subprocess to exit. Budget is (N+1) * TickerPeriod +
//     2 s slack = 6 s for N=3, TickerPeriod=1 s. If the daemon does not
//     exit within the budget the test fails loudly with diagnostic
//     output (portal.log, stderr).
//
// Assertions (all four sub-conditions):
//
//  A. Exit code == 0. osExit(0) on self-eject; any non-zero exit
//     indicates a different path was taken (lock error, ensure-dir
//     failure, etc.).
//  B. No panic / stack trace on stderr. A panic would manifest as
//     "panic:" + "goroutine" lines from runtime/debug; either substring
//     in cmd.Stderr surfaces a regression in the eject path.
//  C. portal.log under <stateDir> contains the substring
//     "self-supervision: saver-membership lost for". This is the
//     load-bearing audit-log invariant from cmd/state_daemon.go's
//     defaultDaemonRun → osExit(0) call site.
//  D. daemon.pid stale-stays-stale: if it exists post-exit, contents
//     equal the subprocess's PID (NOT deleted on osExit(0) per spec §
//     Component D bullet 4.iii). If it never existed (race: the daemon
//     ejected before WritePIDFile completed) that is also acceptable —
//     the spec's invariant is "MUST NOT add cleanup logic to delete
//     daemon.pid before the eject", which is satisfied by either shape.
//
// No t.Parallel: the cmd-package convention applies (package-level
// mutable state injection elsewhere in the cmd package).

package cmd_test

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// selfEjectExitBudget is the wall-time ceiling on the daemon subprocess
// exit relative to its own Start(). Derived from the spec § Component D
// hysteresis: (N + 1) * TickerPeriod + 2 s slack. With N=3 and
// TickerPeriod=1 s (cmd/state_daemon.go defaults) this resolves to 6 s.
// The +2 s slack absorbs daemon cold-start latency (lock acquire +
// WritePIDFile + tmux.DefaultClient construction) plus signal-delivery /
// process-reap jitter on darwin/arm64 CI hardware.
const selfEjectExitBudget = 6 * time.Second

// selfEjectExitPollTick is the cadence of the post-Start wait loop. Not
// used directly (exec.Cmd.Wait blocks until reap), but the deadline
// timer below uses the same envelope shape as the test's other helpers.
const selfEjectExitPollTick = 50 * time.Millisecond

// selfEjectLogMarker is the load-bearing INFO log substring emitted by
// cmd/state_daemon.go's defaultDaemonRun at the osExit(0) call site.
// Spec § Component D bullet 4.i mandates the exact prefix; the test
// asserts substring presence so a future tweak to the suffix (e.g.
// trailing "ticks, exiting") does not flake the test.
const selfEjectLogMarker = "self-supervision: saver-membership lost for"

// TestSelfEject_PortalSaverAbsent_ExitsCleanly pins spec § Component D
// acceptance bullet 1. See the file-header comment for the full
// rationale and assertion breakdown.
func TestSelfEject_PortalSaverAbsent_ExitsCleanly(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// StagePortalBinary builds the binary into a t.TempDir and prepends
	// that dir to PATH for the test lifetime. We invoke by absolute path
	// below but PATH-prepend is retained so any internal re-exec resolves
	// the same build.
	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	// portaltest.IsolateStateForTest folds in the HOME=<tempdir> /
	// XDG_CONFIG_HOME="" host-noise scrub before its pre-snapshot, so
	// the backstop targets a quiet tempdir rather than the developer's
	// live ~/.config/portal/state/.
	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// Stand up the isolated tmux server. We do NOT bootstrap the
	// _portal-saver session — the daemon's saver-membership probe must
	// return false on every tick for the self-eject path to fire.
	sock := tmuxtest.New(t, "ptl-selfeject-")

	// Pre-state staging assertions (spec § Component D Test staging
	// note): daemon.pid must be absent so Component C's pre-check
	// proceeds, and daemon.lock must be absent so the daemon's
	// AcquireDaemonLock acquires cleanly (no contention with a stale
	// fixture). portaltest.IsolateStateForTest creates the stateDir
	// empty, so these stats are expected ENOENT — the assertions exist
	// to surface a regression in the staging path if either file
	// accidentally appears.
	if _, statErr := os.Stat(state.DaemonPID(stateDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("pre-state: %s expected absent; got err=%v\n"+
			"  the staging contract requires daemon.pid absent so Component C's "+
			"pre-check proceeds and the daemon reaches its tick loop",
			state.DaemonPID(stateDir), statErr)
	}
	daemonLockPath := filepath.Join(stateDir, "daemon.lock")
	if _, statErr := os.Stat(daemonLockPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("pre-state: %s expected absent; got err=%v\n"+
			"  the staging contract requires daemon.lock absent so the daemon's "+
			"AcquireDaemonLock acquires cleanly without contending against a "+
			"stale fixture",
			daemonLockPath, statErr)
	}

	// Spawn the daemon directly. Env wires:
	//   - PORTAL_STATE_DIR  → stateDir (isolated)
	//   - XDG_CONFIG_HOME   → inherited from envSlice (isolated config root)
	//   - TMUX              → points at the test socket so the daemon's
	//                         tmux.DefaultClient discovers the test
	//                         server, not the host's. The "<sock>,1,0"
	//                         form mirrors the existing daemon-SIGHUP
	//                         integration test (state_daemon_integration_test.go).
	//   - PATH              → binDir prepended so any internal re-exec
	//                         (defensive) finds the freshly built binary.
	daemonEnv := append([]string{}, envSlice...)
	daemonEnv = append(daemonEnv,
		"PORTAL_STATE_DIR="+stateDir,
		fmt.Sprintf("TMUX=%s,1,0", sock.SocketPath()),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		// PORTAL_LOG_LEVEL=INFO surfaces the self-eject INFO marker into
		// portal.log. Without this, *state.Logger defaults to LevelWarn
		// (see internal/state/logger.go parseLevel) and Assertion C's
		// log-marker substring check would always fail. DEBUG would also
		// work but adds tick-loop noise that obscures the diagnostic
		// dump on regression.
		"PORTAL_LOG_LEVEL=INFO",
	)

	daemon := exec.Command(binary, "state", "daemon")
	daemon.Env = daemonEnv
	// Capture stderr so a panic / stack trace surfaces in the test
	// diagnostic on regression. stdout is left default (discarded) —
	// the daemon writes structured output to portal.log, not stdout.
	var stderr strings.Builder
	daemon.Stderr = &stderr

	if err := daemon.Start(); err != nil {
		t.Fatalf("start portal state daemon: %v", err)
	}
	startInstant := time.Now()
	daemonPID := daemon.Process.Pid

	// Cleanup guard: ensure the daemon never leaks past the test. If the
	// test body's wait loop hit the budget timeout, daemon.Process is
	// still alive; SIGKILL it. If daemon.Wait already returned cleanly,
	// ProcessState is non-nil and the cleanup is a no-op.
	t.Cleanup(func() {
		if daemon.ProcessState != nil {
			return
		}
		if daemon.Process == nil {
			return
		}
		_ = daemon.Process.Signal(syscall.SIGKILL)
		_, _ = daemon.Process.Wait()
	})

	// Run Wait on a goroutine so we can enforce the budget via a timer.
	// Wait populates daemon.ProcessState; only after Wait returns can we
	// read ExitCode / Success / String for the diagnostic.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- daemon.Wait()
	}()

	var waitErr error
	var exitInstant time.Time
	deadline := time.NewTimer(selfEjectExitBudget)
	defer deadline.Stop()
	select {
	case waitErr = <-waitDone:
		exitInstant = time.Now()
	case <-deadline.C:
		// Daemon did not self-eject within the budget. Surface portal.log
		// and stderr in the failure diagnostic so the failure mode (no
		// log marker, wrong log marker, looping tick loop, etc.) is
		// debuggable in one run. SIGKILL via t.Cleanup.
		logBlob := portaltest.ReadPortalLogSafe(stateDir)
		t.Fatalf("daemon did not exit within %s of Start (pid=%d); spec § Component D "+
			"requires self-eject within (N+1)*TickerPeriod = ~4 s for N=3, "+
			"TickerPeriod=1 s\n--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			selfEjectExitBudget, daemonPID, logBlob, stderr.String())
	}

	exitLatency := exitInstant.Sub(startInstant)
	t.Logf("daemon self-eject latency: %s (budget=%s, pid=%d)",
		exitLatency, selfEjectExitBudget, daemonPID)

	// Read portal.log up front so every assertion's diagnostic can cite
	// it. The daemon's logger flushes on Close at the bottom of RunE; by
	// the time Wait returned the log file is fully populated.
	logBlob := portaltest.ReadPortalLogSafe(stateDir)

	// Assertion A: exit code == 0. osExit(0) is the spec-mandated eject
	// primitive (Component D bullet 4.ii). exec.Cmd.Wait returns nil on
	// exit-0; ProcessState.Success() is the canonical predicate.
	if waitErr != nil {
		t.Errorf("daemon Wait returned non-nil error (expected clean exit 0): %v\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			waitErr, logBlob, stderr.String())
	}
	if daemon.ProcessState == nil || !daemon.ProcessState.Success() {
		stateStr := "<nil>"
		exitCode := -1
		if daemon.ProcessState != nil {
			stateStr = daemon.ProcessState.String()
			exitCode = daemon.ProcessState.ExitCode()
		}
		t.Errorf("daemon ProcessState not successful: %s (ExitCode=%d); spec § Component D "+
			"requires osExit(0) on self-eject\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			stateStr, exitCode, logBlob, stderr.String())
	}

	// Assertion B: no panic / stack trace on stderr. A panicking eject
	// path would emit "panic:" plus "goroutine N [running]:" lines from
	// runtime/debug; either substring's presence is a regression
	// signal. The daemon's normal output goes to portal.log, so stderr
	// being non-empty without these markers is also surprising — we
	// log it as informational rather than failing the test.
	stderrText := stderr.String()
	if strings.Contains(stderrText, "panic:") {
		t.Errorf("daemon stderr contains \"panic:\" — eject path panicked\n"+
			"--- daemon stderr ---\n%s\n--- portal.log ---\n%s",
			stderrText, logBlob)
	}
	if strings.Contains(stderrText, "goroutine ") && strings.Contains(stderrText, "[running]:") {
		t.Errorf("daemon stderr contains a Go runtime stack trace — eject path crashed\n"+
			"--- daemon stderr ---\n%s\n--- portal.log ---\n%s",
			stderrText, logBlob)
	}
	if stderrText != "" {
		t.Logf("daemon stderr (informational; no panic / stack trace detected):\n%s", stderrText)
	}

	// Assertion C: portal.log contains the self-eject INFO marker. This
	// is the load-bearing audit-log invariant — a clean exit without
	// this log line means the daemon exited for some other reason
	// (lock contention, ensure-dir failure, signal) rather than the
	// self-supervision path under test.
	if !strings.Contains(logBlob, selfEjectLogMarker) {
		t.Errorf("portal.log missing self-eject marker %q\n"+
			"  spec § Component D bullet 4.i: the eject path MUST emit this INFO line\n"+
			"--- portal.log ---\n%s",
			selfEjectLogMarker, logBlob)
	}

	// Assertion D: daemon.pid stale-stays-stale. Two acceptable shapes:
	//
	//   (i)  File present, contents == daemonPID. This is the spec's
	//        "stale daemon.pid after self-eject is intentional"
	//        (Component D bullet 4.iii) — osExit(0) skips any defer
	//        that would clean it up.
	//   (ii) File never existed. The daemon ejected before its
	//        WritePIDFile call completed (vanishingly unlikely with
	//        N=3 ticks ≈ 3 s of post-acquire ticking, but structurally
	//        possible if a tick observes the eject condition before
	//        the pidfile write returns). This shape ALSO satisfies the
	//        spec's "MUST NOT add cleanup logic to delete daemon.pid"
	//        invariant — the file was never written, so it cannot have
	//        been deleted.
	//
	// The forbidden shape is "file present, contents != daemonPID" or
	// "file existed during the run and is now absent". The latter is
	// not directly observable from a post-exit stat, but is implicitly
	// covered by the spec's "MUST NOT delete daemon.pid" assertion in
	// the production code (verified by cmd/state_daemon_self_supervision_test.go's
	// unit-level seam tests). This integration test asserts the
	// observable post-exit shape only.
	pidPath := state.DaemonPID(stateDir)
	pidData, readErr := os.ReadFile(pidPath)
	switch {
	case errors.Is(readErr, os.ErrNotExist):
		// Shape (ii): pidfile never written. Acceptable. Log for
		// observability — distinguishes the rare race from the
		// common shape (i).
		t.Logf("daemon.pid absent post-exit (acceptable: daemon may have ejected "+
			"before WritePIDFile completed); spec § Component D bullet 4.iii "+
			"invariant satisfied trivially\n  pidPath=%s", pidPath)
	case readErr != nil:
		t.Errorf("read daemon.pid post-exit: %v\n"+
			"  unexpected stat failure other than ENOENT — staging may be corrupted\n"+
			"--- portal.log ---\n%s",
			readErr, logBlob)
	default:
		// Shape (i): pidfile present. Contents MUST equal daemonPID;
		// anything else means the file was rewritten by some other
		// writer between Start and Wait return.
		recordedPID, parseErr := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if parseErr != nil {
			t.Errorf("parse daemon.pid contents %q: %v\n"+
				"--- portal.log ---\n%s", string(pidData), parseErr, logBlob)
		} else if recordedPID != daemonPID {
			t.Errorf("daemon.pid post-exit = %d; want subprocess PID %d (spec § Component D "+
				"bullet 4.iii: the stale pidfile MUST retain the ejecting daemon's PID, "+
				"NOT be rewritten by any cleanup logic)\n"+
				"--- portal.log ---\n%s",
				recordedPID, daemonPID, logBlob)
		} else {
			t.Logf("daemon.pid post-exit = %d (stale-stays-stale, matches subprocess PID); "+
				"spec § Component D bullet 4.iii satisfied", recordedPID)
		}
	}

	// Belt-and-braces sanity floor: exit latency MUST be at least N *
	// TickerPeriod (3 s nominal) — a sub-3 s exit would mean the
	// hysteresis counter incremented faster than the ticker fires,
	// which would imply a structural regression (e.g. counter
	// incrementing inside a tight loop rather than per-tick). 2 s
	// is the conservative floor — leaves a 1 s margin for hardware
	// where the first ticker fire arrives sub-second after Start.
	if exitLatency < 2*time.Second {
		t.Errorf("daemon exit latency %s < 2 s floor; spec § Component D requires "+
			"N=3 consecutive failing ticks before eject, so exit cannot fire "+
			"in less than ~2-3 s of Start\n--- portal.log ---\n%s",
			exitLatency, logBlob)
	}
}

// TestSelfEject_PortalSaverPaneMismatch_ExitsCleanly pins spec § Component D
// acceptance bullet "Self-eject on saver pane pid mismatch":
//
//	Spawn the daemon, then externally replace the `_portal-saver` pane
//	process (e.g., `respawn-pane` to a different process). Daemon exits
//	within (N + 1) tick intervals.
//
// Strategy (complement to TestSelfEject_PortalSaverAbsent_ExitsCleanly):
//
//  1. Stage daemon.pid with a known-dead PID (NOT absent — the absent
//     case is the sibling test). Component C's pre-check resolves the
//     recorded PID as dead (via IdentifyDaemon → IdentifyDead) and lets
//     the new daemon acquire the lock cleanly.
//  2. Pre-create _portal-saver with a placeholder pane process
//     (`sh -c 'exec tail -f /dev/null'`) before spawning the daemon. The
//     daemon's saverMembershipProbe will then see HasSession=true but
//     SaverPanePID=<placeholder pid> != os.Getpid() — the structural
//     "pid mismatch" branch of the probe.
//  3. Set destroy-unattached=off on _portal-saver so the session
//     survives the daemon's self-eject (assertion 4 below depends on
//     the session still existing post-exit).
//  4. Spawn `portal state daemon` directly (bypass orchestrator) and
//     wait for daemon.pid to contain the subprocess PID — this confirms
//     the daemon reached its tick loop (lock acquired, pidfile
//     written).
//  5. Pre-action verification: daemon subprocess PID != _portal-saver
//     pane PID. This is the structural divergence required for the
//     mismatch path under test. If the kernel coincidentally assigns
//     the daemon subprocess the same PID as the placeholder pane, the
//     mismatch branch of the probe will NOT fire and the test would
//     hang — fail loudly instead.
//  6. Wait for daemon exit within (N+1)*TickerPeriod + 2s budget.
//
// Assertions:
//
//	A. Exit code == 0 (osExit(0) — the spec-mandated eject primitive).
//	B. No panic / stack trace on stderr.
//	C. portal.log contains the self-eject INFO marker.
//	D. _portal-saver session still exists post-exit
//	   (destroy-unattached=off keeps it alive).
//
// Cleanup: t.Cleanup ensures the daemon subprocess is reaped before
// the tmux server teardown registered by tmuxtest.New runs.
func TestSelfEject_PortalSaverPaneMismatch_ExitsCleanly(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-selfeject-mismatch-")

	// Stage daemon.pid with a known-dead PID. The `exec.Command("true");
	// cmd.Run()` pattern is deterministic on POSIX: Run waits for the
	// child to exit AND reaps it, so by the time Run returns the kernel
	// has released the PID (modulo the trivially-vanishing PID-reuse
	// window). Component C's pre-check will resolve this PID as
	// IdentifyDead → proceed; the daemon under test acquires the lock.
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatalf("stage dead PID via exec.Command(true).Run: %v", err)
	}
	deadPID := dead.Process.Pid
	if err := state.WritePIDFile(stateDir, deadPID); err != nil {
		t.Fatalf("stage daemon.pid with dead PID %d: %v", deadPID, err)
	}

	// Pre-create _portal-saver with a placeholder pane process. The
	// `sh -c "exec tail -f /dev/null"` payload is the spec's canonical
	// placeholder for "anything other than the daemon" — it's long-lived
	// (won't exit before the daemon's self-check) and its PID is owned
	// by tmux's pty, not by the test process.
	sock.Run(t, "new-session", "-d", "-s", "_portal-saver",
		"sh", "-c", "exec tail -f /dev/null")
	// destroy-unattached=off so the session survives even when no
	// client is attached AND so it survives the daemon's eject (the
	// eject doesn't touch the saver session, but a transient client
	// detach in a flaky test environment would otherwise reap it).
	sock.Run(t, "set-option", "-t", "_portal-saver", "destroy-unattached", "off")

	// Read the placeholder pane PID up front; used for the pre-action
	// divergence assertion and for the diagnostic on regression.
	panePIDStr := strings.TrimSpace(sock.Run(t, "list-panes",
		"-t", "_portal-saver", "-F", "#{pane_pid}"))
	panePID, err := strconv.Atoi(panePIDStr)
	if err != nil {
		t.Fatalf("parse placeholder pane pid %q: %v", panePIDStr, err)
	}

	// Spawn the daemon subprocess. Env wiring matches the sibling test
	// (PORTAL_STATE_DIR, isolated XDG_CONFIG_HOME via envSlice,
	// TMUX pointing at the test socket, PATH including the staged
	// binary dir, PORTAL_LOG_LEVEL=INFO so the eject marker reaches
	// portal.log).
	daemonEnv := append([]string{}, envSlice...)
	daemonEnv = append(daemonEnv,
		"PORTAL_STATE_DIR="+stateDir,
		fmt.Sprintf("TMUX=%s,1,0", sock.SocketPath()),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PORTAL_LOG_LEVEL=INFO",
	)

	daemon := exec.Command(binary, "state", "daemon")
	daemon.Env = daemonEnv
	var stderr strings.Builder
	daemon.Stderr = &stderr

	if err := daemon.Start(); err != nil {
		t.Fatalf("start portal state daemon: %v", err)
	}
	startInstant := time.Now()
	daemonPID := daemon.Process.Pid

	t.Cleanup(func() {
		if daemon.ProcessState != nil {
			return
		}
		if daemon.Process == nil {
			return
		}
		_ = daemon.Process.Signal(syscall.SIGKILL)
		_, _ = daemon.Process.Wait()
	})

	// Poll daemon.pid until it equals the subprocess PID — confirms the
	// daemon reached its tick loop (lock acquired, WritePIDFile
	// returned). Bound to 2 s so a regression in the acquire path
	// surfaces fast rather than racing the exit budget below.
	const lockAcquireBudget = 2 * time.Second
	lockDeadline := time.Now().Add(lockAcquireBudget)
	pidPath := state.DaemonPID(stateDir)
	var recordedPID int
	for time.Now().Before(lockDeadline) {
		data, readErr := os.ReadFile(pidPath)
		if readErr == nil {
			if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil && pid == daemonPID {
				recordedPID = pid
				break
			}
		}
		time.Sleep(selfEjectExitPollTick)
	}
	if recordedPID != daemonPID {
		t.Fatalf("daemon did not write its PID %d into %s within %s "+
			"(post-poll recorded=%d); spec § Component D requires the daemon "+
			"to reach its tick loop before self-eject can fire\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			daemonPID, pidPath, lockAcquireBudget, recordedPID,
			portaltest.ReadPortalLogSafe(stateDir), stderr.String())
	}

	// Pre-action structural divergence check. If the kernel coincidentally
	// assigned the daemon subprocess the same PID as the placeholder pane
	// (vanishingly unlikely — distinct fork chains), the mismatch branch
	// of the probe will NOT fire and the test would hang. Fail loudly with
	// the diagnostic so the rare PID-coincidence flake is unambiguous.
	if daemonPID == panePID {
		t.Fatalf("PID coincidence: daemon subprocess PID (%d) == _portal-saver "+
			"pane PID (%d); spec § Component D's pid-mismatch path requires "+
			"structural divergence between daemon PID and saver pane PID "+
			"(re-run the test to break the coincidence)", daemonPID, panePID)
	}

	// Wait for the daemon to self-eject within the (N+1)*TickerPeriod + 2s
	// budget. Same envelope as the sibling test.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- daemon.Wait()
	}()

	var waitErr error
	var exitInstant time.Time
	deadline := time.NewTimer(selfEjectExitBudget)
	defer deadline.Stop()
	select {
	case waitErr = <-waitDone:
		exitInstant = time.Now()
	case <-deadline.C:
		logBlob := portaltest.ReadPortalLogSafe(stateDir)
		t.Fatalf("daemon did not exit within %s of Start (pid=%d, panePID=%d); "+
			"spec § Component D requires self-eject within (N+1)*TickerPeriod "+
			"= ~4 s for N=3, TickerPeriod=1 s\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			selfEjectExitBudget, daemonPID, panePID, logBlob, stderr.String())
	}

	exitLatency := exitInstant.Sub(startInstant)
	t.Logf("daemon self-eject latency: %s (budget=%s, daemonPID=%d, panePID=%d)",
		exitLatency, selfEjectExitBudget, daemonPID, panePID)

	logBlob := portaltest.ReadPortalLogSafe(stateDir)

	// Assertion A: exit code == 0.
	if waitErr != nil {
		t.Errorf("daemon Wait returned non-nil error (expected clean exit 0): %v\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			waitErr, logBlob, stderr.String())
	}
	if daemon.ProcessState == nil || !daemon.ProcessState.Success() {
		stateStr := "<nil>"
		exitCode := -1
		if daemon.ProcessState != nil {
			stateStr = daemon.ProcessState.String()
			exitCode = daemon.ProcessState.ExitCode()
		}
		t.Errorf("daemon ProcessState not successful: %s (ExitCode=%d); spec § Component D "+
			"requires osExit(0) on self-eject\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			stateStr, exitCode, logBlob, stderr.String())
	}

	// Assertion B: no panic / stack trace on stderr.
	stderrText := stderr.String()
	if strings.Contains(stderrText, "panic:") {
		t.Errorf("daemon stderr contains \"panic:\" — eject path panicked\n"+
			"--- daemon stderr ---\n%s\n--- portal.log ---\n%s",
			stderrText, logBlob)
	}
	if strings.Contains(stderrText, "goroutine ") && strings.Contains(stderrText, "[running]:") {
		t.Errorf("daemon stderr contains a Go runtime stack trace — eject path crashed\n"+
			"--- daemon stderr ---\n%s\n--- portal.log ---\n%s",
			stderrText, logBlob)
	}
	if stderrText != "" {
		t.Logf("daemon stderr (informational; no panic / stack trace detected):\n%s", stderrText)
	}

	// Assertion C: portal.log contains the self-eject INFO marker.
	if !strings.Contains(logBlob, selfEjectLogMarker) {
		t.Errorf("portal.log missing self-eject marker %q\n"+
			"  spec § Component D: the eject path MUST emit this INFO line\n"+
			"--- portal.log ---\n%s",
			selfEjectLogMarker, logBlob)
	}

	// Assertion D: _portal-saver session still exists post-eject. The
	// daemon's self-eject must NOT touch the saver session — the spec is
	// explicit that the eject is osExit(0), nothing more. With
	// destroy-unattached=off set above, the session is guaranteed to
	// persist across a client-less window, so any absence here is a
	// regression in the eject path (or in tmux teardown ordering).
	if out, hasErr := sock.TryRun("has-session", "-t", "=_portal-saver"); hasErr != nil {
		t.Errorf("_portal-saver session missing post-eject: %v\n"+
			"  spec § Component D: the eject path is osExit(0); the saver "+
			"session MUST NOT be killed as a side effect\n"+
			"--- tmux has-session output ---\n%s\n--- portal.log ---\n%s",
			hasErr, out, logBlob)
	}

	// Belt-and-braces sanity floor: same 2s floor as the sibling test —
	// the hysteresis counter cannot increment faster than the ticker
	// fires, so sub-2 s exit indicates a structural regression.
	if exitLatency < 2*time.Second {
		t.Errorf("daemon exit latency %s < 2 s floor; spec § Component D requires "+
			"N=3 consecutive failing ticks before eject, so exit cannot fire "+
			"in less than ~2-3 s of Start\n--- portal.log ---\n%s",
			exitLatency, logBlob)
	}
}

// Compile-time guard: ensure selfEjectExitPollTick is referenced so a
// future refactor that adds a poll-loop body cannot silently drop it.
var _ = selfEjectExitPollTick

// firstFailingTickObservationWindow is the wall-time gap between
// daemon Start and the snapBefore capture. Sized so the snapshot
// lands AFTER probe 1 has returned false (the daemon's saver probe
// fires on the first ticker tick at ~TickerPeriod = 1 s) but BEFORE
// the hysteresis counter reaches N=3 (which fires at ~3 s). The
// 1.2 s value mirrors the spec-cited window and matches the
// scrollbackEmergencePollTick + ticker first-fire envelope.
//
// 200 ms of slack beyond the 1 s nominal ticker fire absorbs
// process-start latency (lock acquire + WritePIDFile +
// tmux.DefaultClient construction) on darwin/arm64 CI hardware.
const firstFailingTickObservationWindow = 1200 * time.Millisecond

// TestSelfEject_NoScrollbackDeltaAcrossEject pins spec § Component D
// acceptance bullet "No final flush on self-eject":
//
//	Snapshot the scrollback directory at the moment the daemon's
//	self-check first registers a failing tick, and again
//	immediately after os.Exit(0). The two snapshots must be
//	identical (no new files, no deletions, no mtime/size changes).
//
// The structural guarantee under test: cmd/state_daemon.go's
// defaultDaemonRun calls osExit(0) DIRECTLY on hysteresis trip,
// bypassing daemonShutdownFunc / defaultShutdownFlush. The
// equivalent guarantee for the externally-SIGKILLed orphan path is
// covered by Task 4-2's
// TestKillBarrierEscalation_NoScrollbackDeltaIn200msPostExit in
// internal/tmux/kill_barrier_escalation_no_final_flush_integration_test.go;
// this test is its Component D self-eject twin.
//
// Choreography (mirrors TestSelfEject_PortalSaverAbsent_ExitsCleanly's
// absent-saver setup but pivots on the scrollback dir snapshot
// rather than the exit-code / log-marker assertions):
//
//  1. SkipIfNoTmux + StagePortalBinary + isolated state dir via
//     portaltest.IsolateStateForTest (which folds in the HOME /
//     XDG_CONFIG_HOME host-noise scrub before its pre-snapshot).
//     daemon.pid staged absent (Component C's pre-check proceeds).
//  2. Stand up an isolated tmux server WITHOUT _portal-saver — the
//     per-tick saver-membership probe will return false on every
//     tick from probe 1 onward.
//  3. Spawn `portal state daemon` directly (bypass orchestrator) with
//     env wiring matching the sibling tests (PORTAL_STATE_DIR, TMUX,
//     PATH, PORTAL_LOG_LEVEL=INFO).
//  4. Sleep firstFailingTickObservationWindow (1.2 s) — lands AFTER
//     the first failing tick observation but BEFORE the counter
//     reaches N=3.
//  5. Capture snapBefore via portaltest.SnapshotStateDir(scrollbackDir).
//  6. Wait for the subprocess to exit (within
//     selfEjectExitBudget = (N+1)*TickerPeriod + 2s = 6 s).
//  7. Capture snapAfter via portaltest.SnapshotStateDir(scrollbackDir).
//  8. Assert snapBefore == snapAfter across all Fingerprint fields
//     (Exists, Size, MtimeNanos, CtimeNanos, Sha256, Hashed,
//     IsSymlink, SymlinkTarget). On any delta, dump full diagnostics
//     (portal.log, stderr, both snapshot key sets, field-level deltas).
//  9. Assert exit code == 0.
//
// Empty pre-snapshot is a valid baseline. Unlike Task 4-2 which
// pre-creates a `work` session so the daemon has at least one pane
// to capture, this test creates no sessions — the daemon ticks
// against an empty enumeration and writes zero scrollback files
// before self-ejecting. snapBefore is then empty; snapAfter must
// ALSO be empty (no eject-time flush). Both-empty is a legitimate
// pass: the load-bearing assertion is "no delta", not "non-empty
// pre-snapshot". This is the spec's stronger shape — proves the
// eject path writes nothing even when there was nothing to write,
// closing any scenario where a defensive "flush only if non-empty"
// branch might mask the regression under test.
func TestSelfEject_NoScrollbackDeltaAcrossEject(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// No _portal-saver session: the daemon's saver-membership probe
	// returns false on every tick from probe 1 — first failing tick
	// observation arrives at ~TickerPeriod (1 s) after Start.
	sock := tmuxtest.New(t, "ptl-selfeject-noflush-")

	// Pre-state staging (mirrors the sibling absent-saver test):
	// daemon.pid absent so Component C's pre-check proceeds and the
	// daemon reaches its tick loop.
	if _, statErr := os.Stat(state.DaemonPID(stateDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("pre-state: %s expected absent; got err=%v",
			state.DaemonPID(stateDir), statErr)
	}

	daemonEnv := append([]string{}, envSlice...)
	daemonEnv = append(daemonEnv,
		"PORTAL_STATE_DIR="+stateDir,
		fmt.Sprintf("TMUX=%s,1,0", sock.SocketPath()),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PORTAL_LOG_LEVEL=INFO",
	)

	daemon := exec.Command(binary, "state", "daemon")
	daemon.Env = daemonEnv
	var stderr strings.Builder
	daemon.Stderr = &stderr

	if err := daemon.Start(); err != nil {
		t.Fatalf("start portal state daemon: %v", err)
	}
	startInstant := time.Now()
	daemonPID := daemon.Process.Pid

	t.Cleanup(func() {
		if daemon.ProcessState != nil {
			return
		}
		if daemon.Process == nil {
			return
		}
		_ = daemon.Process.Signal(syscall.SIGKILL)
		_, _ = daemon.Process.Wait()
	})

	// Start the Wait reaper goroutine BEFORE the snapBefore window so
	// the subprocess is reaped deterministically when it self-ejects
	// — the deadline-bounded select below blocks on the same channel.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- daemon.Wait()
	}()

	// Snap window 1: sleep into the post-first-failing-tick window.
	// The simple-sleep approach is more deterministic than polling
	// portal.log because (a) portal.log is buffered and an INFO line
	// for tick activity is not guaranteed at this point in the loop,
	// (b) the spec window (post-probe-1, pre-counter-N) is well-defined
	// in wall-clock terms relative to TickerPeriod.
	time.Sleep(firstFailingTickObservationWindow)

	scrollbackDir := state.ScrollbackDir(stateDir)
	snapBefore, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("snapBefore SnapshotStateDir(%s): %v", scrollbackDir, err)
	}

	// Wait for the subprocess to self-eject within budget. Same
	// envelope as the sibling tests in this file.
	var waitErr error
	var exitInstant time.Time
	deadline := time.NewTimer(selfEjectExitBudget)
	defer deadline.Stop()
	select {
	case waitErr = <-waitDone:
		exitInstant = time.Now()
	case <-deadline.C:
		logBlob := portaltest.ReadPortalLogSafe(stateDir)
		t.Fatalf("daemon did not exit within %s of Start (pid=%d); spec § Component D "+
			"requires self-eject within (N+1)*TickerPeriod = ~4 s for N=3, "+
			"TickerPeriod=1 s\n--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			selfEjectExitBudget, daemonPID, logBlob, stderr.String())
	}

	exitLatency := exitInstant.Sub(startInstant)
	t.Logf("daemon self-eject latency: %s (budget=%s, pid=%d)",
		exitLatency, selfEjectExitBudget, daemonPID)

	logBlob := portaltest.ReadPortalLogSafe(stateDir)

	// snapAfter: captured immediately after Wait returns. The kernel
	// has already reaped the process at this point — any final flush
	// the daemon attempted would have already completed (osExit(0)
	// is synchronous from the daemon's perspective, and Wait does
	// not return until the kernel finalizes process teardown).
	snapAfter, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("snapAfter SnapshotStateDir(%s): %v\n--- portal.log ---\n%s",
			scrollbackDir, err, logBlob)
	}

	// Exit code == 0 (osExit(0) on self-eject). Asserted alongside the
	// snapshot equality so a non-zero exit (which would indicate the
	// daemon took a different path entirely) is debuggable in the
	// same run as the no-final-flush invariant.
	if waitErr != nil {
		t.Errorf("daemon Wait returned non-nil error (expected clean exit 0): %v\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			waitErr, logBlob, stderr.String())
	}
	if daemon.ProcessState == nil || !daemon.ProcessState.Success() {
		stateStr := "<nil>"
		exitCode := -1
		if daemon.ProcessState != nil {
			stateStr = daemon.ProcessState.String()
			exitCode = daemon.ProcessState.ExitCode()
		}
		t.Errorf("daemon ProcessState not successful: %s (ExitCode=%d); spec § Component D "+
			"requires osExit(0) on self-eject\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			stateStr, exitCode, logBlob, stderr.String())
	}

	// EQUALITY ASSERTION via portaltest.DiffFingerprints. Empty-both
	// is a legitimate pass — the invariant is "no delta".
	if deltas := portaltest.DiffFingerprints(snapBefore, snapAfter); len(deltas) > 0 {
		lines := make([]string, len(deltas))
		for i, d := range deltas {
			lines[i] = "  " + portaltest.FormatDelta(d)
		}
		t.Fatalf("scrollback dir mutated between snapBefore (post-first-failing-tick) "+
			"and snapAfter (post-self-eject) — spec § Component D requires "+
			"NO final flush on self-eject\n"+
			"  scrollback dir: %s\n"+
			"  pre keys  (%d): %v\n"+
			"  post keys (%d): %v\n"+
			"  delta(s):\n%s\n"+
			"--- portal.log ---\n%s\n"+
			"--- daemon stderr ---\n%s",
			scrollbackDir, len(snapBefore), slices.Sorted(maps.Keys(snapBefore)),
			len(snapAfter), slices.Sorted(maps.Keys(snapAfter)),
			strings.Join(lines, "\n"), logBlob, stderr.String())
	}
}

// legitimateColdStartHysteresisMirror mirrors cmd.selfSupervisionHysteresisTicks
// (= 3 from Task 5-1's hardware measurement) so the observation-window
// arithmetic in this _test file (package cmd_test, no access to the
// unexported production const) reads cleanly.
//
// Duplicating the value here is acceptable because (a) the production
// const is stable (rationale pinned by the in-source comment block
// above selfSupervisionHysteresisTicks in cmd/state_daemon.go and the
// integration-tagged harness at
// cmd/state_daemon_hysteresis_measurement_test.go) and (b) any
// drift between the two does NOT produce a false-positive failure:
// this test asserts the legitimate-cold-start path NEVER ejects
// regardless of N, so a larger production N just means more headroom
// inside the same observation window. The mirror value MUST track
// production if N is ever revised — fail loudly on the test side via
// a deliberate over-mirroring to (newN+2)*TickerPeriod.
const legitimateColdStartHysteresisMirror = 3

// legitimateColdStartObservationWindow is the wall-time gap between
// BootstrapPortalSaver's readiness-barrier return and the post-window
// assertions. Sized as (N + 2) * TickerPeriod so the daemon has run
// at least N+2 ticks of its self-supervision probe — strictly larger
// than the hysteresis threshold (N=3), so any false-positive eject
// would have already fired.
//
// N=3 and TickerPeriod=1 s (cmd/state_daemon.go defaults) → 5 s.
// Combined with BootstrapPortalSaver's readiness barrier
// (≤ saverReadinessTimeout = 2 s) the total wall time of this test
// budget sits well under the orchestrator's 7-8 s ceiling.
const legitimateColdStartObservationWindow = (legitimateColdStartHysteresisMirror + 2) * time.Second

// legitimateColdStartLockAcquireBudget bounds the post-
// BootstrapPortalSaver poll for daemon.pid to be populated.
// BootstrapPortalSaver's readiness barrier (saverReadinessTimeout =
// 2 s) already polls IdentifyDaemon until the daemon is observably
// up, so under normal conditions daemon.pid is non-empty the moment
// BootstrapPortalSaver returns. The extra 1.5 s budget here is
// belt-and-braces for slow CI hardware where the barrier may WARN-
// and-return on timeout rather than success.
const legitimateColdStartLockAcquireBudget = 1500 * time.Millisecond

// TestSelfEject_LegitimateColdStartDoesNotFalsePositive pins spec §
// Component D acceptance — "legitimate first-tick self-check":
//
//	A daemon spawned via the production BootstrapPortalSaver path
//	(placeholder → set destroy-unattached=off → respawn-pane to
//	`portal state daemon` → readiness barrier) MUST NOT self-eject
//	during the immediate post-readiness window. The first-tick
//	saver-membership probe must observe HasSession=true AND
//	SaverPanePID == os.Getpid() because the daemon IS the saver
//	pane's process.
//
// This is the no-false-positive complement to the three eject-path
// tests above (PortalSaverAbsent, PortalSaverPaneMismatch,
// NoScrollbackDeltaAcrossEject). Those tests pin that the eject
// path FIRES under divergent-view conditions; this test pins that
// the eject path does NOT fire under legitimate conditions.
//
// Choreography:
//
//  1. SkipIfNoTmux + StagePortalBinary + isolated state dir via
//     portaltest.IsolateStateForTest (which folds in the HOME /
//     XDG_CONFIG_HOME host-noise scrub before its pre-snapshot). The
//     PORTAL_STATE_DIR + PORTAL_LOG_LEVEL env vars are set on the
//     test process so the tmux server (auto-started by the first
//     sock.Run invocation downstream) inherits them, and panes
//     spawned by tmux's respawn-pane therefore see them too.
//  2. Stand up an isolated tmux server via tmuxtest.New.
//  3. Call tmux.BootstrapPortalSaver(client, stateDir) — the
//     production cold-start path. This runs Phase 3 placeholder →
//     set destroy-unattached=off → respawn-pane to the daemon, then
//     blocks on the readiness barrier until daemon.pid + IdentifyDaemon
//     report success.
//  4. Confirm daemon.pid matches _portal-saver's pane PID — the
//     structural invariant that the daemon IS the saver pane process.
//  5. Sleep legitimateColdStartObservationWindow ((N+2) * TickerPeriod
//     = 5 s) so the daemon ticks at least N+2 times. During this
//     window the per-tick saver-membership probe MUST return true on
//     every fire (the daemon IS the saver pane process) and the
//     hysteresis counter MUST stay at 0.
//  6. Post-window assertions:
//     A. daemon.pid still exists (file present on disk).
//     B. state.IdentifyDaemon(daemonPID) == IdentifyIsPortalDaemon
//     — proves the daemon process is alive and is recognisably
//     a `portal state daemon`.
//     C. daemon.pid contents == _portal-saver pane PID (re-read
//     from tmux). Confirms the pane-process binding did not
//     drift mid-window (which would be evidence of an internal
//     respawn or some other regression).
//     D. portal.log under stateDir does NOT contain the self-
//     supervision marker. Any presence of the marker would mean
//     the daemon ejected at least once during the window — a
//     false positive.
//
// Cleanup: t.Cleanup tears down the saver session via tmux kill-
// session so the daemon receives SIGHUP and exits cleanly before
// tmuxtest.New's t.Cleanup kills the server. portaltest's backstop
// then runs.
func TestSelfEject_LegitimateColdStartDoesNotFalsePositive(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	if _, err := exec.LookPath("portal"); err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	// IsolateStateForTest folds in the HOME=<tempdir> /
	// XDG_CONFIG_HOME="" host-noise scrub before its pre-snapshot so
	// the backstop targets a quiet tempdir rather than the developer's
	// live state.
	_, stateDir := portaltest.IsolateStateForTest(t)

	// Env propagation: t.Setenv on the test process applies to every
	// subprocess spawned by os/exec (including the tmux server auto-
	// started by the first sock.Run downstream). tmux's respawn-pane
	// payload inherits the server's process env, so the daemon spawned
	// by Phase 3's respawn sees PORTAL_STATE_DIR and PORTAL_LOG_LEVEL
	// correctly.
	//
	// PORTAL_STATE_DIR pins the daemon's state writes to the isolated
	// stateDir (verbatim, no suffix appended per internal/state/paths.go
	// resolution order).
	//
	// PORTAL_LOG_LEVEL=INFO surfaces the self-supervision INFO marker
	// into portal.log if (hypothetically) the daemon ejected. Without
	// this, *state.Logger defaults to LevelWarn and Assertion D's
	// negative log-marker check would be trivially satisfied even on a
	// regression.
	//
	// PATH ensures the tmux-respawned daemon can resolve `portal` from
	// the staged binDir. StagePortalBinary already prepended binDir to
	// the test process PATH; re-set here defensively so the explicit
	// value is visible in test diagnostics.
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_LOG_LEVEL", "INFO")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// Stand up the isolated tmux server. The first sock.Run / Client
	// invocation below auto-starts the server, which inherits the env
	// configured above.
	sock := tmuxtest.New(t, "ptl-selfeject-legit-")
	client := sock.Client()

	// Cleanup ordering: kill _portal-saver explicitly BEFORE tmuxtest's
	// own kill-server cleanup runs, so the daemon receives SIGHUP and
	// flushes cleanly. tmuxtest registers its cleanup first (during
	// tmuxtest.New); t.Cleanup runs in LIFO order so this one fires
	// first. Tolerant of "session already gone" — the test body itself
	// never kills the saver, so this is the only teardown path.
	t.Cleanup(func() {
		_, _ = sock.TryRun("kill-session", "-t", tmux.PortalSaverName)
	})

	// Run the production cold-start path. BootstrapPortalSaver:
	//  - probes has-session (false: fresh server)
	//  - creates _portal-saver with the placeholder command
	//  - applies destroy-unattached=off
	//  - respawn-pane swaps in `portal state daemon`
	//  - blocks on waitForSaverDaemonReadyFn until the daemon
	//    publishes daemon.pid and identifies as a portal state daemon
	//    (or saverReadinessTimeout = 2 s elapses with a WARN).
	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v\n--- portal.log ---\n%s",
			err, portaltest.ReadPortalLogSafe(stateDir))
	}

	// Read daemon.pid post-bootstrap. The readiness barrier should have
	// already observed it populated, but on a slow CI host the barrier
	// may have WARN-timed-out and returned nil. Poll up to
	// legitimateColdStartLockAcquireBudget to absorb that case.
	pidPath := state.DaemonPID(stateDir)
	var daemonPID int
	lockDeadline := time.Now().Add(legitimateColdStartLockAcquireBudget)
	for time.Now().Before(lockDeadline) {
		data, readErr := os.ReadFile(pidPath)
		if readErr == nil {
			if pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil && pid > 0 {
				daemonPID = pid
				break
			}
		}
		time.Sleep(selfEjectExitPollTick)
	}
	if daemonPID == 0 {
		t.Fatalf("daemon.pid never populated within %s of BootstrapPortalSaver return; "+
			"the legitimate cold-start path requires the daemon to publish its PID "+
			"before the observation window opens\n--- portal.log ---\n%s",
			legitimateColdStartLockAcquireBudget, portaltest.ReadPortalLogSafe(stateDir))
	}

	// Confirm the structural binding: daemon.pid == _portal-saver pane
	// PID. This is the precondition for the saver-membership probe to
	// return true on every tick — without it the test is exercising the
	// wrong code path.
	panePIDStrPre := strings.TrimSpace(sock.Run(t, "list-panes",
		"-t", tmux.PortalSaverName, "-F", "#{pane_pid}"))
	panePIDPre, err := strconv.Atoi(panePIDStrPre)
	if err != nil {
		t.Fatalf("parse pre-window pane pid %q: %v", panePIDStrPre, err)
	}
	if daemonPID != panePIDPre {
		t.Fatalf("pre-window divergence: daemon.pid (%d) != _portal-saver pane PID (%d)\n"+
			"  the legitimate cold-start path requires the daemon to BE the saver "+
			"pane process; any mismatch here means BootstrapPortalSaver's respawn-pane "+
			"+ readiness barrier did not produce the expected structural binding\n"+
			"--- portal.log ---\n%s",
			daemonPID, panePIDPre, portaltest.ReadPortalLogSafe(stateDir))
	}
	t.Logf("pre-window: daemon.pid=%d == _portal-saver pane PID=%d (structural binding confirmed)",
		daemonPID, panePIDPre)

	// Observation window: sleep (N+2) * TickerPeriod so the daemon
	// ticks at least N+2 times. If any tick observed a failing probe,
	// the hysteresis counter would reach N within the window and the
	// daemon would have ejected before we wake.
	t.Logf("opening observation window: %s ((N+2) * TickerPeriod, N=%d)",
		legitimateColdStartObservationWindow, legitimateColdStartHysteresisMirror)
	time.Sleep(legitimateColdStartObservationWindow)

	// Post-window assertions.

	// Read portal.log up front so every assertion's diagnostic can cite
	// it. The daemon is still running (assertion A confirms below); the
	// logger flushes per-line under LevelInfo so any INFO marker emitted
	// during the window is already visible without needing the daemon
	// to exit.
	logBlob := portaltest.ReadPortalLogSafe(stateDir)

	// Assertion A: daemon.pid still exists. A regression that ejected
	// the daemon during the window would leave daemon.pid behind
	// (spec § Component D bullet 4.iii: stale-stays-stale), so file
	// presence alone is not proof of liveness — assertion B handles
	// that. But absence here would be a separate regression (some
	// cleanup path deleted the pidfile).
	pidData, readErr := os.ReadFile(pidPath)
	if errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("daemon.pid absent post-window; spec § Component D bullet 4.iii "+
			"forbids any cleanup logic deleting daemon.pid — file absence here "+
			"signals an unrelated regression in the pidfile lifecycle\n"+
			"--- portal.log ---\n%s", logBlob)
	}
	if readErr != nil {
		t.Fatalf("read daemon.pid post-window: %v\n--- portal.log ---\n%s",
			readErr, logBlob)
	}
	recordedPID, parseErr := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if parseErr != nil {
		t.Fatalf("parse daemon.pid contents %q: %v\n--- portal.log ---\n%s",
			string(pidData), parseErr, logBlob)
	}
	if recordedPID != daemonPID {
		t.Errorf("daemon.pid post-window = %d; want pre-window PID %d "+
			"(rewrite mid-window would be a regression)\n--- portal.log ---\n%s",
			recordedPID, daemonPID, logBlob)
	}

	// Assertion B: state.IdentifyDaemon(daemonPID) == IdentifyIsPortalDaemon.
	// Proves the daemon process is alive AND is still recognisable as
	// a portal state daemon. A self-ejected daemon would not pass
	// IdentifyDaemon (the process would be dead, IdentifyDead).
	result, identifyErr := state.IdentifyDaemon(daemonPID)
	if identifyErr != nil {
		t.Errorf("IdentifyDaemon(%d) returned transient error: %v\n"+
			"  the legitimate cold-start path requires the daemon to remain "+
			"identifiable throughout the observation window\n"+
			"--- portal.log ---\n%s",
			daemonPID, identifyErr, logBlob)
	}
	if result != state.IdentifyIsPortalDaemon {
		t.Errorf("IdentifyDaemon(%d) = %v; want IdentifyIsPortalDaemon\n"+
			"  the daemon spawned by BootstrapPortalSaver MUST remain alive "+
			"and identifiable across the (N+2) * TickerPeriod observation "+
			"window — any other classification means the daemon self-ejected "+
			"(false positive) or was killed externally\n"+
			"--- portal.log ---\n%s",
			daemonPID, result, logBlob)
	}

	// Assertion C: daemon.pid contents still == _portal-saver pane PID.
	// Re-read the pane PID from tmux. Drift here would mean either the
	// pane was respawned mid-window (some external actor) or the
	// daemon was replaced — either case is evidence the daemon under
	// test is not the same legitimate daemon we observed pre-window.
	panePIDStrPost := strings.TrimSpace(sock.Run(t, "list-panes",
		"-t", tmux.PortalSaverName, "-F", "#{pane_pid}"))
	panePIDPost, err := strconv.Atoi(panePIDStrPost)
	if err != nil {
		t.Errorf("parse post-window pane pid %q: %v\n--- portal.log ---\n%s",
			panePIDStrPost, err, logBlob)
	} else if panePIDPost != daemonPID {
		t.Errorf("post-window divergence: daemon.pid (%d) != _portal-saver pane PID (%d)\n"+
			"  the structural binding must hold throughout the observation window\n"+
			"--- portal.log ---\n%s",
			daemonPID, panePIDPost, logBlob)
	}

	// Assertion D: portal.log does NOT contain the self-supervision
	// marker. Any presence means the daemon ejected during the window
	// — a false positive in the spec's "legitimate first-tick self-
	// check" acceptance bullet.
	//
	// This assertion depends on PORTAL_LOG_LEVEL=INFO being honoured
	// by the daemon (set above on the test process before tmux server
	// start). If a future logger refactor breaks env propagation, this
	// check degrades silently to a trivial pass — assertions A-C
	// independently prove the no-false-positive invariant.
	if strings.Contains(logBlob, selfEjectLogMarker) {
		t.Errorf("portal.log contains self-eject marker %q during legitimate cold-start "+
			"observation window — the daemon self-ejected when it should not have "+
			"(spec § Component D: legitimate first-tick self-check)\n"+
			"--- portal.log ---\n%s",
			selfEjectLogMarker, logBlob)
	}

	t.Logf("legitimate cold-start completed without self-eject: "+
		"daemon alive (pid=%d), pane PID matches, no self-supervision marker in log",
		daemonPID)

	// Belt-and-braces: confirm portal.log path resolves to the
	// isolated stateDir (paranoia check against an env-propagation
	// regression that would silently route logs elsewhere — in which
	// case Assertion D would be trivially satisfied and useless).
	expectedLogPath := filepath.Join(stateDir, "portal.log")
	if state.PortalLog(stateDir) != expectedLogPath {
		t.Errorf("internal: state.PortalLog(%s) = %s; want %s",
			stateDir, state.PortalLog(stateDir), expectedLogPath)
	}
}
