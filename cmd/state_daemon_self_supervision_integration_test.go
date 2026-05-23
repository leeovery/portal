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
//  1. SkipIfNoTmux + StagePortalBinary + applyHostNoiseMitigation +
//     isolated state dir via portaltest.NewIsolatedStateEnv. The
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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
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

	// Host-noise mitigation MUST run BEFORE portaltest.NewIsolatedStateEnv
	// so the backstop targets a quiet tempdir rather than the
	// developer's live ~/.config/portal/state/. See the twin helper in
	// cmd/bootstrap/orphan_sweep_integration_test.go for the full
	// rationale.
	applyHostNoiseMitigation(t)

	envSlice, stateDir := portaltest.NewIsolatedStateEnv(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// Stand up the isolated tmux server. We do NOT bootstrap the
	// _portal-saver session — the daemon's saver-membership probe must
	// return false on every tick for the self-eject path to fire.
	sock := tmuxtest.New(t, "ptl-selfeject-")

	// Pre-state staging assertions (spec § Component D Test staging
	// note): daemon.pid must be absent so Component C's pre-check
	// proceeds, and daemon.lock must be absent so the daemon's
	// AcquireDaemonLock acquires cleanly (no contention with a stale
	// fixture). portaltest.NewIsolatedStateEnv creates the stateDir
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
		logBlob := readPortalLogSafe(stateDir)
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
	logBlob := readPortalLogSafe(stateDir)

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

// readPortalLogSafe reads portal.log under stateDir and returns its
// contents as a string, or a placeholder describing the read failure.
// Used in failure diagnostics so the daemon's audit trail is always
// surfaced in test output without forcing the call site to branch on
// the read error.
func readPortalLogSafe(stateDir string) string {
	data, err := os.ReadFile(state.PortalLog(stateDir))
	if err != nil {
		return fmt.Sprintf("(read portal.log failed: %v)", err)
	}
	return string(data)
}

// applyHostNoiseMitigation re-points HOME at a fresh tempdir and clears
// XDG_CONFIG_HOME from the test process env BEFORE
// portaltest.NewIsolatedStateEnv runs its pre-snapshot. Without this,
// a developer's live `portal state daemon` writing to
// ~/.config/portal/state/ during the test window would mutate the
// snapshot's pre-state and false-positive-trip the backstop's
// post-test delta check.
//
// Inlined here rather than imported because the canonical twin helpers
// live in `package bootstrap_test` and `package tmux_test`
// (cmd/bootstrap/orphan_sweep_integration_test.go,
// internal/tmux/portal_saver_endstate_integration_test.go) — both
// unexported, neither accessible from `package cmd_test`. This file is
// the first cmd_test consumer of portaltest.NewIsolatedStateEnv and
// owns its own copy of the helper.
func applyHostNoiseMitigation(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
}

// Compile-time guard: ensure selfEjectExitPollTick is referenced so a
// future refactor that adds a poll-loop body cannot silently drop it.
var _ = selfEjectExitPollTick
