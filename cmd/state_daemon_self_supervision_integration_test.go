//go:build integration

// The daemon is spawned directly rather than through `portal open` or the
// orchestrator: a bootstrap-time orphan sweep would preempt it before its
// tick loop runs the self-check.

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

const selfEjectExitBudget = 6 * time.Second

const selfEjectExitPollTick = 50 * time.Millisecond

const selfEjectLogMarker = "daemon: self-eject"

func TestSelfEject_PortalSaverAbsent_ExitsCleanly(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	// No _portal-saver session: the saver-membership probe must fail on
	// every tick for the self-eject path to fire.
	sock := tmuxtest.New(t, "ptl-selfeject-")

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
		t.Fatalf("daemon did not exit within %s of Start (pid=%d); the daemon must "+
			"self-eject within (N+1)*TickerPeriod = ~4 s for N=3, "+
			"TickerPeriod=1 s\n--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			selfEjectExitBudget, daemonPID, logBlob, stderr.String())
	}

	exitLatency := exitInstant.Sub(startInstant)
	t.Logf("daemon self-eject latency: %s (budget=%s, pid=%d)",
		exitLatency, selfEjectExitBudget, daemonPID)

	logBlob := portaltest.ReadPortalLogSafe(stateDir)

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
		t.Errorf("daemon ProcessState not successful: %s (ExitCode=%d); the self-eject "+
			"path must exit(0)\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			stateStr, exitCode, logBlob, stderr.String())
	}

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

	if !strings.Contains(logBlob, selfEjectLogMarker) {
		t.Errorf("portal.log missing self-eject marker %q\n"+
			"  the eject path MUST emit this INFO line\n"+
			"--- portal.log ---\n%s",
			selfEjectLogMarker, logBlob)
	}

	// daemon.pid stale-stays-stale: osExit(0) deliberately skips any
	// cleanup. An absent file is also legal — the daemon may have ejected
	// before WritePIDFile completed, which cannot be a deletion.
	pidPath := state.DaemonPID(stateDir)
	pidData, readErr := os.ReadFile(pidPath)
	switch {
	case errors.Is(readErr, os.ErrNotExist):
		t.Logf("daemon.pid absent post-exit (acceptable: daemon may have ejected "+
			"before WritePIDFile completed); the stale-pidfile invariant is "+
			"satisfied trivially\n  pidPath=%s", pidPath)
	case readErr != nil:
		t.Errorf("read daemon.pid post-exit: %v\n"+
			"  unexpected stat failure other than ENOENT — staging may be corrupted\n"+
			"--- portal.log ---\n%s",
			readErr, logBlob)
	default:
		recordedPID, parseErr := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if parseErr != nil {
			t.Errorf("parse daemon.pid contents %q: %v\n"+
				"--- portal.log ---\n%s", string(pidData), parseErr, logBlob)
		} else if recordedPID != daemonPID {
			t.Errorf("daemon.pid post-exit = %d; want subprocess PID %d (the stale "+
				"pidfile MUST retain the ejecting daemon's PID, "+
				"NOT be rewritten by any cleanup logic)\n"+
				"--- portal.log ---\n%s",
				recordedPID, daemonPID, logBlob)
		} else {
			t.Logf("daemon.pid post-exit = %d (stale-stays-stale, matches subprocess PID); "+
				"the stale-pidfile invariant is satisfied", recordedPID)
		}
	}

	// Floor, not just a ceiling: a sub-2 s exit would mean the hysteresis
	// counter incremented faster than the ticker fires.
	if exitLatency < 2*time.Second {
		t.Errorf("daemon exit latency %s < 2 s floor; the daemon requires "+
			"N=3 consecutive failing ticks before eject, so exit cannot fire "+
			"in less than ~2-3 s of Start\n--- portal.log ---\n%s",
			exitLatency, logBlob)
	}
}

func TestSelfEject_PortalSaverPaneMismatch_ExitsCleanly(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	sock := tmuxtest.New(t, "ptl-selfeject-mismatch-")

	// Stage daemon.pid with a known-dead PID so the lock-acquire pre-check
	// resolves it as dead and proceeds. Run both waits for and reaps the
	// child, so the kernel has released the PID by the time it returns.
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatalf("stage dead PID via exec.Command(true).Run: %v", err)
	}
	deadPID := dead.Process.Pid
	if err := state.WritePIDFile(stateDir, deadPID); err != nil {
		t.Fatalf("stage daemon.pid with dead PID %d: %v", deadPID, err)
	}

	// A long-lived placeholder pane process makes the saver-membership
	// probe see HasSession=true with a pane pid that is not the daemon's —
	// the mismatch branch under test.
	sock.Run(t, "new-session", "-d", "-s", "_portal-saver",
		"sh", "-c", "exec tail -f /dev/null")
	sock.Run(t, "set-option", "-t", "_portal-saver", "destroy-unattached", "off")

	panePIDStr := strings.TrimSpace(sock.Run(t, "list-panes",
		"-t", "_portal-saver", "-F", "#{pane_pid}"))
	panePID, err := strconv.Atoi(panePIDStr)
	if err != nil {
		t.Fatalf("parse placeholder pane pid %q: %v", panePIDStr, err)
	}

	daemonEnv := append([]string{}, envSlice...)
	daemonEnv = append(daemonEnv,
		"PORTAL_STATE_DIR="+stateDir,
		fmt.Sprintf("TMUX=%s,%d,0", sock.SocketPath(), liveServerPID(t, sock)),
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
			"(post-poll recorded=%d); the daemon must reach its tick loop "+
			"before self-eject can fire\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			daemonPID, pidPath, lockAcquireBudget, recordedPID,
			portaltest.ReadPortalLogSafe(stateDir), stderr.String())
	}

	if daemonPID == panePID {
		t.Fatalf("PID coincidence: daemon subprocess PID (%d) == _portal-saver "+
			"pane PID (%d); the pid-mismatch path requires "+
			"structural divergence between daemon PID and saver pane PID "+
			"(re-run the test to break the coincidence)", daemonPID, panePID)
	}

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
			"the daemon must self-eject within (N+1)*TickerPeriod "+
			"= ~4 s for N=3, TickerPeriod=1 s\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			selfEjectExitBudget, daemonPID, panePID, logBlob, stderr.String())
	}

	exitLatency := exitInstant.Sub(startInstant)
	t.Logf("daemon self-eject latency: %s (budget=%s, daemonPID=%d, panePID=%d)",
		exitLatency, selfEjectExitBudget, daemonPID, panePID)

	logBlob := portaltest.ReadPortalLogSafe(stateDir)

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
		t.Errorf("daemon ProcessState not successful: %s (ExitCode=%d); the self-eject "+
			"path must exit(0)\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			stateStr, exitCode, logBlob, stderr.String())
	}

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

	if !strings.Contains(logBlob, selfEjectLogMarker) {
		t.Errorf("portal.log missing self-eject marker %q\n"+
			"  the eject path MUST emit this INFO line\n"+
			"--- portal.log ---\n%s",
			selfEjectLogMarker, logBlob)
	}

	if out, hasErr := sock.TryRun("has-session", "-t", "=_portal-saver"); hasErr != nil {
		t.Errorf("_portal-saver session missing post-eject: %v\n"+
			"  the eject path is osExit(0); the saver "+
			"session MUST NOT be killed as a side effect\n"+
			"--- tmux has-session output ---\n%s\n--- portal.log ---\n%s",
			hasErr, out, logBlob)
	}

	if exitLatency < 2*time.Second {
		t.Errorf("daemon exit latency %s < 2 s floor; the daemon requires "+
			"N=3 consecutive failing ticks before eject, so exit cannot fire "+
			"in less than ~2-3 s of Start\n--- portal.log ---\n%s",
			exitLatency, logBlob)
	}
}

var _ = selfEjectExitPollTick

// Lands after the first failing probe (~1 tick) but before the hysteresis
// counter reaches N; the extra 200 ms absorbs process-start latency.
const firstFailingTickObservationWindow = 1200 * time.Millisecond

// Both snapshots empty is a legitimate pass: the assertion is "no delta",
// and writing nothing even when there is nothing to write is the stronger
// shape — a defensive "flush only if non-empty" branch cannot mask it.
func TestSelfEject_NoScrollbackDeltaAcrossEject(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	// No _portal-saver session: the saver-membership probe fails from the
	// first tick onward.
	sock := tmuxtest.New(t, "ptl-selfeject-noflush-")

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

	// The reaper goroutine starts before the snapshot window so the
	// subprocess is reaped the moment it self-ejects.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- daemon.Wait()
	}()

	// Sleeping is more deterministic than polling portal.log: the log is
	// buffered and no INFO line is guaranteed at this point in the loop.
	time.Sleep(firstFailingTickObservationWindow)

	scrollbackDir := state.ScrollbackDir(stateDir)
	snapBefore, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("snapBefore SnapshotStateDir(%s): %v", scrollbackDir, err)
	}

	var waitErr error
	var exitInstant time.Time
	deadline := time.NewTimer(selfEjectExitBudget)
	defer deadline.Stop()
	select {
	case waitErr = <-waitDone:
		exitInstant = time.Now()
	case <-deadline.C:
		logBlob := portaltest.ReadPortalLogSafe(stateDir)
		t.Fatalf("daemon did not exit within %s of Start (pid=%d); the daemon must "+
			"self-eject within (N+1)*TickerPeriod = ~4 s for N=3, "+
			"TickerPeriod=1 s\n--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			selfEjectExitBudget, daemonPID, logBlob, stderr.String())
	}

	exitLatency := exitInstant.Sub(startInstant)
	t.Logf("daemon self-eject latency: %s (budget=%s, pid=%d)",
		exitLatency, selfEjectExitBudget, daemonPID)

	logBlob := portaltest.ReadPortalLogSafe(stateDir)

	// Safe to snapshot the moment Wait returns: the kernel has finalized
	// teardown, so any flush the daemon attempted has already landed.
	snapAfter, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("snapAfter SnapshotStateDir(%s): %v\n--- portal.log ---\n%s",
			scrollbackDir, err, logBlob)
	}

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
		t.Errorf("daemon ProcessState not successful: %s (ExitCode=%d); the self-eject "+
			"path must exit(0)\n"+
			"--- portal.log ---\n%s\n--- daemon stderr ---\n%s",
			stateStr, exitCode, logBlob, stderr.String())
	}

	if deltas := portaltest.DiffFingerprints(snapBefore, snapAfter); len(deltas) > 0 {
		lines := make([]string, len(deltas))
		for i, d := range deltas {
			lines[i] = "  " + portaltest.FormatDelta(d)
		}
		t.Fatalf("scrollback dir mutated between snapBefore (post-first-failing-tick) "+
			"and snapAfter (post-self-eject) — the self-eject path must perform "+
			"NO final flush\n"+
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

// Mirrors the unexported cmd.selfSupervisionHysteresisTicks, which this
// external test package cannot reach. Must track production if N is
// revised; drift only shrinks headroom, it cannot produce a false failure.
const legitimateColdStartHysteresisMirror = 3

// Strictly longer than the hysteresis threshold, so a false-positive eject
// would already have fired by the time the window closes.
const legitimateColdStartObservationWindow = (legitimateColdStartHysteresisMirror + 2) * time.Second

// Absorbs a readiness barrier that WARN-timed-out rather than succeeding.
const legitimateColdStartLockAcquireBudget = 1500 * time.Millisecond

func TestSelfEject_LegitimateColdStartDoesNotFalsePositive(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	if _, err := exec.LookPath("portal"); err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	_, stateDir := portaltest.IsolateStateForTest(t)

	// These must be on the test process, not a per-command env slice: the
	// daemon is started by tmux's respawn-pane and inherits the tmux
	// server's env, which is inherited in turn from the first sock.Run.
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_LOG_LEVEL", "INFO")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	sock := tmuxtest.New(t, "ptl-selfeject-legit-")
	client := sock.Client()

	// Registered after tmuxtest's kill-server cleanup and therefore runs
	// before it (LIFO), so the daemon gets its SIGHUP and flushes cleanly.
	t.Cleanup(func() {
		_, _ = sock.TryRun("kill-session", "-t", tmux.PortalSaverName)
	})

	if err := tmux.BootstrapPortalSaver(client, stateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver: %v\n--- portal.log ---\n%s",
			err, portaltest.ReadPortalLogSafe(stateDir))
	}

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

	t.Logf("opening observation window: %s ((N+2) * TickerPeriod, N=%d)",
		legitimateColdStartObservationWindow, legitimateColdStartHysteresisMirror)
	time.Sleep(legitimateColdStartObservationWindow)

	logBlob := portaltest.ReadPortalLogSafe(stateDir)

	// An ejected daemon leaves daemon.pid behind, so presence is not
	// proof of liveness — the IdentifyDaemon check below is. Absence is a
	// separate regression: some path deleted the pidfile.
	pidData, readErr := os.ReadFile(pidPath)
	if errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("daemon.pid absent post-window; no cleanup logic may delete "+
			"daemon.pid — file absence here "+
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

	// Degrades to a trivial pass if PORTAL_LOG_LEVEL propagation ever
	// breaks; the checks above stand independently of it.
	if strings.Contains(logBlob, selfEjectLogMarker) {
		t.Errorf("portal.log contains self-eject marker %q during legitimate cold-start "+
			"observation window — the daemon self-ejected when it should not have "+
			"(the first-tick self-check is legitimate here)\n"+
			"--- portal.log ---\n%s",
			selfEjectLogMarker, logBlob)
	}

	t.Logf("legitimate cold-start completed without self-eject: "+
		"daemon alive (pid=%d), pane PID matches, no self-supervision marker in log",
		daemonPID)

	// Guards the check above from being trivially satisfied by an
	// env-propagation regression routing logs somewhere else.
	expectedLogPath := filepath.Join(stateDir, "portal.log")
	if state.PortalLog(stateDir) != expectedLogPath {
		t.Errorf("internal: state.PortalLog(%s) = %s; want %s",
			stateDir, state.PortalLog(stateDir), expectedLogPath)
	}
}
