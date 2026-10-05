//go:build integration

package cmd_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const daemonAlivePollInterval = 50 * time.Millisecond

// liveServerPID is the pid of the socket's live server: the one tmux names in
// the TMUX a pane process or a hook's run-shell job inherits.
func liveServerPID(t *testing.T, sock *tmuxtest.Socket) int {
	t.Helper()
	pid, err := strconv.Atoi(strings.TrimSpace(sock.Run(t, "display-message", "-p", "#{pid}")))
	if err != nil {
		t.Fatalf("read server pid: %v", err)
	}
	return pid
}

// The daemon becomes alive through observable steps (daemon.pid appearing, then
// naming a process that answers), so Stall bounds how long the reading may sit
// unchanged rather than how long the whole start-up takes: a loaded host makes
// a starting daemon slower without making it broken.
var daemonAliveWait = harnesstest.ProgressWait{
	Stall:   8 * time.Second,
	Ceiling: 45 * time.Second,
	Tick:    daemonAlivePollInterval,
}

// tickStartDelay pins SIGHUP inside the first tick's per-pane loop: long
// enough for the daemon's 1s ticker to have fired, short enough that the tick
// has not yet completed.
const tickStartDelay = 1200 * time.Millisecond

// Scrollback accumulates continuously, so Stall bounds how long a pane may gain
// no lines rather than how long it may take to fill, and Ceiling only backstops
// a pane that keeps gaining lines without ever reaching the target: on a loaded
// host the seq writer is slower, not stuck.
var panePopulationWait = harnesstest.ProgressWait{
	Stall:   10 * time.Second,
	Ceiling: 120 * time.Second,
	Tick:    panePopulationPollInterval,
}

const panePopulationPollInterval = 100 * time.Millisecond

// paneCount is sized so the aggregate per-tick wall time clears the anchored
// threshold with headroom, while each pane stays above tmux's minimum width.
const paneCount = 12

// scrollbackLines yields roughly 3.5MB of rendered text per pane, enough to
// push a single tick well above the 2s threshold on a development host.
const scrollbackLines = 500000

// tmux's default history-limit of 2000 would truncate scrollbackLines.
const historyLimit = 1000000

func TestDaemon_MidTickSIGHUP_ExitsWithinBoundedWindow(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	sock := tmuxtest.New(t, "ptl-daemon-sighup-")

	bootstrapTmuxServer(t, sock)

	lines := scrollbackLines
	var singlePaneWallTime time.Duration
	var threshold time.Duration
	for attempt := range 2 {
		// Recreate the server on retry so panes start without inherited buffers.
		if attempt > 0 {
			sock.KillServer()
			bootstrapTmuxServer(t, sock)
		}
		populatePanes(t, sock, lines)
		singlePaneWallTime = measureSinglePaneCapture(t, sock)
		threshold = anchorThreshold(singlePaneWallTime)
		if threshold <= tmux.KillBarrierTimeoutCeiling {
			break
		}
		lines /= 2
	}
	if threshold > tmux.KillBarrierTimeoutCeiling {
		t.Fatalf("derived threshold %s exceeds killBarrierTimeout ceiling %s after halving "+
			"per-pane scrollback to %d lines (singlePaneWallTime=%s); "+
			"test fixture cannot establish a meaningful responsiveness window on this hardware",
			threshold, tmux.KillBarrierTimeoutCeiling, lines, singlePaneWallTime)
	}

	aggregate := time.Duration(paneCount) * singlePaneWallTime
	t.Logf("measurement: singlePaneWallTime=%s, aggregatePerTickEstimate=%s, anchoredThreshold=%s, "+
		"scrollbackLinesPerPane=%d", singlePaneWallTime, aggregate, threshold, lines)
	if aggregate < 2*time.Second {
		t.Skipf("skipping: aggregate per-tick wall time (%s) is below the 2s heuristic; "+
			"this host's capture-pane is too fast to exercise the tick-spans-kill-barrier "+
			"cancellation path (anchoredThreshold=%s)",
			aggregate, threshold)
	}

	daemon := exec.Command(binary, "state", "daemon")
	daemon.Env = append(os.Environ(),
		fmt.Sprintf("TMUX=%s,%d,0", sock.SocketPath(), liveServerPID(t, sock)),
		"PORTAL_STATE_DIR="+stateDir,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PORTAL_LOG_LEVEL=DEBUG",
	)
	// CombinedOutput is unusable here: the test signals the daemon while it runs.
	var stderr strings.Builder
	daemon.Stderr = &stderr
	if err := daemon.Start(); err != nil {
		t.Fatalf("start portal state daemon: %v", err)
	}

	t.Cleanup(func() {
		if daemon.Process == nil {
			return
		}
		if daemon.ProcessState != nil {
			return
		}
		if err := daemon.Process.Signal(syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Errorf("leaking daemon: SIGKILL failed: %v", err)
		}
		_, _ = daemon.Process.Wait()
		t.Errorf("daemon process leaked past test end and was SIGKILL'd; pid=%d", daemon.Process.Pid)
	})

	waitForDaemonAlive(t, stateDir)

	if err := os.WriteFile(state.SaveRequested(stateDir), nil, 0o644); err != nil {
		t.Fatalf("touch save.requested: %v", err)
	}

	time.Sleep(tickStartDelay)

	_, saveReqStatErr := os.Stat(state.SaveRequested(stateDir))
	t.Logf("pre-SIGHUP: save.requested exists=%v", saveReqStatErr == nil)

	// @portal-restoring makes the daemon skip its post-cancel final flush.
	// Without it the exit chain runs a second, non-cancellable captureAndCommit
	// and latency roughly doubles. Production always sets the marker before the
	// kill, so this mirrors the real path.
	sock.Run(t, "set-option", "-s", state.RestoringMarkerName, "1")

	tickStart := time.Now()
	if err := daemon.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("send SIGHUP to daemon (pid=%d): %v", daemon.Process.Pid, err)
	}

	// cmd.Wait, not cmd.Process.Wait: only cmd.Wait populates cmd.ProcessState,
	// which the clean-exit assertion and the leak-check cleanup both read.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- daemon.Wait()
	}()

	var exitErr error
	var exitTime time.Time
	deadline := time.NewTimer(tmux.KillBarrierTimeoutCeiling + 500*time.Millisecond)
	defer deadline.Stop()
	select {
	case exitErr = <-waitDone:
		exitTime = time.Now()
	case <-deadline.C:
		t.Fatalf("daemon did not exit within %s of SIGHUP (pid=%d); singlePaneWallTime=%s, "+
			"anchoredThreshold=%s\n--- daemon stderr ---\n%s",
			tmux.KillBarrierTimeoutCeiling+500*time.Millisecond, daemon.Process.Pid,
			singlePaneWallTime, threshold, stderr.String())
	}

	latency := exitTime.Sub(tickStart)
	t.Logf("daemon exit latency: %s (anchoredThreshold=%s, ceiling=%s, singlePaneWallTime=%s)",
		latency, threshold, tmux.KillBarrierTimeoutCeiling, singlePaneWallTime)

	if data, derr := os.ReadFile(state.PortalLog(stateDir)); derr == nil {
		t.Logf("--- portal.log ---\n%s", string(data))
	}
	if entries, derr := os.ReadDir(state.ScrollbackDir(stateDir)); derr == nil {
		t.Logf("post-exit scrollback file count: %d (paneCount=%d)", len(entries), paneCount)
	}

	if latency >= threshold {
		t.Errorf("daemon exit latency %s >= anchored threshold %s "+
			"(singlePaneWallTime=%s, ceiling=%s)\n--- daemon stderr ---\n%s",
			latency, threshold, singlePaneWallTime, tmux.KillBarrierTimeoutCeiling, stderr.String())
	}

	if latency >= tmux.KillBarrierTimeoutCeiling {
		t.Errorf("daemon exit latency %s >= killBarrierTimeout ceiling %s "+
			"(singlePaneWallTime=%s)\n--- daemon stderr ---\n%s",
			latency, tmux.KillBarrierTimeoutCeiling, singlePaneWallTime, stderr.String())
	}

	if exitErr != nil {
		t.Errorf("daemon exited non-zero after SIGHUP: %v\n--- daemon stderr ---\n%s",
			exitErr, stderr.String())
	}
	if daemon.ProcessState == nil || !daemon.ProcessState.Success() {
		state := "<nil>"
		if daemon.ProcessState != nil {
			state = daemon.ProcessState.String()
		}
		t.Errorf("daemon ProcessState not successful: %s\n--- daemon stderr ---\n%s",
			state, stderr.String())
	}
}

// populatePanes fills paneCount panes with synthetic scrollback and waits for
// it to render. The sleep-infinity tail keeps each pane alive after seq
// finishes so capture-pane has stable scrollback to read.
func populatePanes(t *testing.T, sock *tmuxtest.Socket, lines int) {
	t.Helper()

	cmd := fmt.Sprintf("seq 1 %d; sleep infinity", lines)

	// -d because `go test` has no controlling terminal to attach to.
	sock.Run(t, "new-session", "-d", "-s", "perf", "-x", "200", "-y", "50",
		"sh", "-c", cmd)

	for i := 1; i < paneCount; i++ {
		sock.Run(t, "split-window", "-h", "-t", "perf:0",
			"sh", "-c", cmd)
		// Re-balance so the next split does not hit "pane too small".
		sock.Run(t, "select-layout", "-t", "perf:0", "even-horizontal")
	}

	for i := range paneCount {
		target := fmt.Sprintf("perf:0.%d", i)
		res := harnesstest.AwaitProgress(t, panePopulationWait,
			func() int {
				// The final line may lack a newline, so this count is a lower
				// bound - which is what the readiness gate wants.
				return strings.Count(sock.Run(t, "capture-pane", "-p", "-t", target, "-S", "-"), "\n")
			},
			func(got int) bool { return got >= lines },
		)
		if !res.Reached {
			t.Fatalf("pane %s did not accumulate %d scrollback lines (%s)", target, lines, res)
		}
	}
}

// measureSinglePaneCapture times the same capture-pane shape the daemon's
// per-pane loop uses, so the derived threshold tracks this host's real cost.
func measureSinglePaneCapture(t *testing.T, sock *tmuxtest.Socket) time.Duration {
	t.Helper()
	start := time.Now()
	_ = sock.Run(t, "capture-pane", "-e", "-p", "-t", "perf:0.0", "-S", "-")
	return time.Since(start)
}

// anchorThreshold floors the exit-latency threshold at 2s, scaling to twice
// the measured per-pane wall time so slow hosts do not flake.
func anchorThreshold(singlePaneWallTime time.Duration) time.Duration {
	doubled := 2 * singlePaneWallTime
	if doubled < 2*time.Second {
		return 2 * time.Second
	}
	return doubled
}

// waitForDaemonAlive polls for the pidfile. Process.Pid is not a usable
// readiness signal: it exists before the daemon writes daemon.pid.
func waitForDaemonAlive(t *testing.T, stateDir string) {
	t.Helper()
	res := harnesstest.AwaitProgress(t, daemonAliveWait,
		func() portaltest.DaemonPIDObservation { return portaltest.ObserveDaemonPID(stateDir) },
		func(o portaltest.DaemonPIDObservation) bool { return o.Alive })
	if res.Reached {
		return
	}
	var logBlob string
	if data, err := os.ReadFile(state.PortalLog(stateDir)); err == nil {
		logBlob = string(data)
	}
	t.Fatalf("daemon did not become alive (%s) (stateDir=%s)\n--- portal.log ---\n%s",
		res, stateDir, logBlob)
}

// bootstrapTmuxServer starts the server with an anchor session, then raises
// history-limit globally. set-option -g needs a live server and the limit is
// read at session-creation time, so both must precede any real session; the
// anchor session must stay alive or the server exits with it.
func bootstrapTmuxServer(t *testing.T, sock *tmuxtest.Socket) {
	t.Helper()
	sock.Run(t, "new-session", "-d", "-s", "_anchor", "sh", "-c", "sleep infinity")
	sock.Run(t, "set-option", "-g", "history-limit", strconv.Itoa(historyLimit))
}
