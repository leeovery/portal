//go:build integration

package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	waitingSession    = "waiting"
	waitingTranscript = "transcript-before-shutdown\n"
	waitingHook       = "echo resumed > waiting-fired"

	waitingPanelTitle = "Resume session"

	// waitingPaneBudget bounds a bootstrap's restore reaching the panel and the
	// daemon re-filing the waiting pane's transcript, each a handful of daemon
	// ticks and longer on a loaded host.
	waitingPaneBudget = 20 * time.Second

	commitNowArgs = `args="state commit-now"`
	recoverArgs   = `args="state resume-recover`
	drawArgs      = "resume-draw"
	waitArgs      = "resume-wait"
	markerClear   = "unset resume pending marker failed"
)

var waitingPaneToken = hookstest.SubjectSeedA

func waitingPaneTarget() tmux.Target { return tmux.PaneTargetExact(waitingSession, 0, 0) }

func TestShutdown_WaitingPaneOutlastsSIGTERMAndComesBackStillAsking(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	rt := newWaitingPaneRuntime(t, binary, binDir)
	tokenPath := state.PendingScrollbackFile(waitingPaneToken)
	transcript := rt.awaitWaitingPaneSaved(t, tokenPath)
	tree := rt.restingTree(t)

	t.Run("SIGTERM to the pane and the daemon leaves the pane waiting through the flush and the saver's commit-now", func(t *testing.T) {
		mark := portaltest.ReadPortalLogSafe(rt.stateDir)

		for _, p := range tree {
			rt.signal(t, p.pid, syscall.SIGTERM)
		}
		rt.signal(t, rt.daemonPID, syscall.SIGTERM)

		rt.awaitGone(t, "daemon", rt.daemonPID)
		rt.awaitSaverClosed(t)
		since := rt.awaitCommitNowCommitted(t, mark)

		if !slices.ContainsFunc(since, isCompletedFlush) {
			t.Errorf("the daemon's shutdown flush did not complete after SIGTERM\n%s", rt.waitingDiagnostic())
		}
		for _, p := range tree {
			if err := syscall.Kill(p.pid, 0); err != nil {
				t.Errorf("pid %d (%s) did not outlast SIGTERM: %v", p.pid, p.command, err)
			}
		}
		if screen := rt.paneScreen(t); !strings.Contains(screen, waitingPanelTitle) {
			t.Errorf("the panel is no longer up after the shutdown signal; capture-pane -p:\n%s", screen)
		}
		rt.assertPending(t)
		assertNoRecoveryTail(t, since)
		rt.assertSavedWaiting(t, tokenPath, transcript)
	})

	t.Run("the server's exit ends the parked chain before its recovery tail and keeps the transcript", func(t *testing.T) {
		mark := portaltest.ReadPortalLogSafe(rt.stateDir)

		rt.signal(t, rt.serverPID, syscall.SIGTERM)

		for _, p := range tree {
			rt.awaitGone(t, p.command, p.pid)
		}
		rt.awaitGone(t, "tmux server", rt.serverPID)
		awaitStateDirSettled(t, rt.stateDir)

		assertNoRecoveryTail(t, newLogLines(portaltest.ReadPortalLogSafe(rt.stateDir), mark))
		rt.assertSavedWaiting(t, tokenPath, transcript)
	})

	t.Run("the next restore brings the pane back still asking over its token-named transcript", func(t *testing.T) {
		rt.bootstrap(t)

		rt.awaitPanel(t)
		rt.assertPending(t)
		rt.assertSavedWaiting(t, tokenPath, transcript)
	})
}

// waitingPaneRuntime is a Portal runtime bootstrapped onto saved state holding
// one session whose pane carries a resume hook that falls to the shipped lazy
// default, so the restore leaves that pane waiting on its panel.
type waitingPaneRuntime struct {
	symptomFixture
	daemonPID int
	serverPID int
}

func newWaitingPaneRuntime(t *testing.T, binary, binDir string) *waitingPaneRuntime {
	t.Helper()

	_, stateDir := portaltest.IsolateStateForTest(t)

	// Set before the tmux server starts, so the daemon, the restored pane and
	// every session-closed hook subprocess resolve this fixture's files.
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(stateDir, "hooks.json"))
	t.Setenv("PORTAL_PROJECTS_FILE", filepath.Join(stateDir, "projects.json"))
	t.Setenv("PORTAL_ALIASES_FILE", filepath.Join(stateDir, "aliases"))

	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	seedWaitingPaneState(t, stateDir)

	rt := &waitingPaneRuntime{symptomFixture: symptomFixture{
		sock:     tmuxtest.New(t, "ptl-waitpane-"),
		stateDir: stateDir,
		binary:   binary,
		binDir:   binDir,
	}}
	rt.bootstrap(t)
	rt.awaitPanel(t)
	rt.assertPending(t)
	return rt
}

func seedWaitingPaneState(t *testing.T, stateDir string) {
	t.Helper()
	key := state.SanitizePaneKey(waitingSession, 0, 0)
	restoretest.SeedScrollback(t, stateDir, waitingSession, 0, 0, []byte(waitingTranscript))
	positional, err := filepath.Rel(stateDir, state.ScrollbackFile(stateDir, key))
	if err != nil {
		t.Fatalf("relative scrollback path: %v", err)
	}
	restoretest.WriteIndex(t, stateDir, state.Index{Sessions: []state.Session{{
		Name: waitingSession,
		Windows: []state.Window{{
			Index:  0,
			Layout: "tiled",
			Active: true,
			Panes: []state.Pane{{
				Index:          0,
				Active:         true,
				ScrollbackFile: filepath.ToSlash(positional),
				PortalPaneID:   waitingPaneToken,
			}},
		}},
	}}})
	hookstest.StageStore(t, hookstest.Staging{Dir: stateDir, Entries: map[string]string{waitingPaneToken: waitingHook}})
}

// bootstrap starts a server on the fixture's socket the way EnsureServer does
// and runs Portal's full bootstrap against it: hooks, saver and restore.
func (rt *waitingPaneRuntime) bootstrap(t *testing.T) {
	t.Helper()
	rt.sock.Run(t, "new-session", "-d", "-s", tmux.PortalBootstrapName)
	rt.sock.WaitForSession(t, tmux.PortalBootstrapName, 5*time.Second)
	runPortalList(t, rt.binary, rt.symptomFixture)

	pid, present, err := tmux.SaverPanePIDOrAbsent(rt.sock.Client(), tmux.PortalSaverName)
	if err != nil || !present {
		t.Fatalf("read daemon pid: present=%v err=%v\n%s", present, err, rt.waitingDiagnostic())
	}
	rt.daemonPID = pid
	rt.serverPID = liveServerPID(t, rt.sock)
}

// awaitWaitingPaneSaved waits for a daemon commit filing the waiting pane under
// its token-named transcript alone, its positional name reclaimed, and answers
// that transcript's bytes.
func (rt *waitingPaneRuntime) awaitWaitingPaneSaved(t *testing.T, tokenPath string) []byte {
	t.Helper()
	if err := state.TouchSaveRequested(rt.stateDir); err != nil {
		t.Fatalf("request a save: %v", err)
	}
	positional := state.ScrollbackFile(rt.stateDir, state.SanitizePaneKey(waitingSession, 0, 0))
	saved := func() bool {
		if rt.savedWaitingRecord() != tokenPath {
			return false
		}
		_, err := os.Stat(positional)
		return errors.Is(err, os.ErrNotExist)
	}
	if !harnesstest.PollUntil(t, waitingPaneBudget, daemonTickPollInterval, saved) {
		t.Fatalf("the daemon never filed the waiting pane under %s alone within %s\n%s",
			tokenPath, waitingPaneBudget, rt.waitingDiagnostic())
	}
	data, err := os.ReadFile(filepath.Join(rt.stateDir, tokenPath))
	if err != nil {
		t.Fatalf("read the token-named transcript: %v", err)
	}
	if string(data) != waitingTranscript {
		t.Fatalf("the token-named transcript holds %q; want the seeded %q", data, waitingTranscript)
	}
	return data
}

// savedWaitingRecord answers the scrollback path sessions.json files the
// waiting pane under, empty when it names no such pane.
func (rt *waitingPaneRuntime) savedWaitingRecord() string {
	idx, skip, err := state.ReadIndex(rt.stateDir)
	if err != nil || skip {
		return ""
	}
	for _, s := range idx.Sessions {
		if s.Name != waitingSession || len(s.Windows) != 1 || len(s.Windows[0].Panes) != 1 {
			continue
		}
		return s.Windows[0].Panes[0].ScrollbackFile
	}
	return ""
}

func (rt *waitingPaneRuntime) assertSavedWaiting(t *testing.T, tokenPath string, transcript []byte) {
	t.Helper()
	if got := rt.savedWaitingRecord(); got != tokenPath {
		t.Errorf("sessions.json files session %s's pane under %q; want %q\n%s",
			waitingSession, got, tokenPath, rt.waitingDiagnostic())
	}
	got, err := os.ReadFile(filepath.Join(rt.stateDir, tokenPath))
	if err != nil {
		t.Errorf("token-named transcript %s: %v\n%s", tokenPath, err, rt.waitingDiagnostic())
		return
	}
	if !bytes.Equal(got, transcript) {
		t.Errorf("token-named transcript changed\nbefore: %q\nafter:  %q", transcript, got)
	}
}

func (rt *waitingPaneRuntime) paneScreen(t *testing.T) string {
	t.Helper()
	out, err := rt.sock.TryRun("capture-pane", "-p", "-t", string(waitingPaneTarget()))
	if err != nil {
		return fmt.Sprintf("(capture-pane: %v)\n%s", err, out)
	}
	return out
}

func (rt *waitingPaneRuntime) awaitPanel(t *testing.T) {
	t.Helper()
	shown := func() bool { return strings.Contains(rt.paneScreen(t), waitingPanelTitle) }
	if !harnesstest.PollUntil(t, waitingPaneBudget, daemonTickPollInterval, shown) {
		t.Fatalf("pane %s never showed the panel; capture-pane -p:\n%s\n%s",
			waitingPaneTarget(), rt.paneScreen(t), rt.waitingDiagnostic())
	}
}

func (rt *waitingPaneRuntime) assertPending(t *testing.T) {
	t.Helper()
	set := func() bool {
		value, err := rt.sock.Client().ReadPaneOption(waitingPaneTarget(), state.ResumePendingOption)
		return err == nil && state.ResumePendingSet(value)
	}
	if !harnesstest.PollUntil(t, waitingPaneBudget, daemonTickPollInterval, set) {
		t.Fatalf("%s is not set on pane %s\n%s", state.ResumePendingOption, waitingPaneTarget(), rt.waitingDiagnostic())
	}
}

// waitingProcess is one process of the waiting pane, read by walking down
// from the pid tmux reports for the fixture's own pane, so nothing outside it
// is ever signalled.
type waitingProcess struct {
	pid     int
	command string
}

// restingTree answers the waiting pane's processes once the draw has handed
// off: the parked shell and the waiter it runs.
func (rt *waitingPaneRuntime) restingTree(t *testing.T) []waitingProcess {
	t.Helper()
	raw := strings.TrimSpace(rt.sock.Run(t, "display-message", "-p", "-t", string(waitingPaneTarget()), "#{pane_pid}"))
	root, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("pane_pid of %s = %q: %v", waitingPaneTarget(), raw, err)
	}
	var tree []waitingProcess
	settled := func() bool {
		tree = waitingPaneTree(t, root)
		return len(tree) == 2 &&
			strings.Contains(tree[1].command, waitArgs) && !strings.Contains(tree[1].command, drawArgs)
	}
	if !harnesstest.PollUntil(t, waitingPaneBudget, daemonTickPollInterval, settled) {
		t.Fatalf("waiting pane %d never settled to a parked shell over its waiter: %+v", root, tree)
	}
	return tree
}

func waitingPaneTree(t *testing.T, root int) []waitingProcess {
	t.Helper()
	tree := []waitingProcess{{pid: root, command: processCommand(t, root)}}
	for i := 0; i < len(tree); i++ {
		out, err := exec.Command("pgrep", "-P", strconv.Itoa(tree[i].pid)).Output()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			continue
		}
		if err != nil {
			t.Fatalf("pgrep -P %d: %v", tree[i].pid, err)
		}
		for field := range strings.FieldsSeq(string(out)) {
			pid, err := strconv.Atoi(field)
			if err != nil {
				t.Fatalf("pgrep returned %q: %v", field, err)
			}
			tree = append(tree, waitingProcess{pid: pid, command: processCommand(t, pid)})
		}
	}
	return tree
}

func processCommand(t *testing.T, pid int) string {
	t.Helper()
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// pid must be this fixture's tmux server or a process it hosts.
func (rt *waitingPaneRuntime) signal(t *testing.T, pid int, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(pid, sig); err != nil {
		t.Fatalf("signal %v to %d: %v\n%s", sig, pid, err, rt.waitingDiagnostic())
	}
}

func (rt *waitingPaneRuntime) awaitGone(t *testing.T, what string, pid int) {
	t.Helper()
	gone := func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) }
	if !harnesstest.PollUntil(t, daemonExitBudget, daemonTickPollInterval, gone) {
		t.Fatalf("%s (pid %d) still running %s on\n%s", what, pid, daemonExitBudget, rt.waitingDiagnostic())
	}
}

func (rt *waitingPaneRuntime) awaitSaverClosed(t *testing.T) {
	t.Helper()
	closed := func() bool { return !rawSessionPresent(t, rt.sock, tmux.PortalSaverName) }
	if !harnesstest.PollUntil(t, daemonExitBudget, daemonTickPollInterval, closed) {
		t.Fatalf("%s still open %s after its daemon exited\n%s",
			tmux.PortalSaverName, daemonExitBudget, rt.waitingDiagnostic())
	}
}

// awaitCommitNowCommitted waits for a commit-now started after mark to exit
// zero, which one whose commit cycle stood down does not, and answers the log
// lines written since mark.
func (rt *waitingPaneRuntime) awaitCommitNowCommitted(t *testing.T, mark string) []string {
	t.Helper()
	var since []string
	committed := func() bool {
		since = newLogLines(portaltest.ReadPortalLogSafe(rt.stateDir), mark)
		return commitNowExitedZero(since)
	}
	if !harnesstest.PollUntil(t, daemonExitBudget, daemonTickPollInterval, committed) {
		t.Fatalf("no commit-now committed within %s of %s closing\n%s",
			daemonExitBudget, tmux.PortalSaverName, rt.waitingDiagnostic())
	}
	if _, err := rt.sock.TryRun("has-session", "-t", tmux.PortalBootstrapName); err != nil {
		t.Fatalf("the server stopped answering before the commit-now was observed: %v", err)
	}
	return since
}

func commitNowExitedZero(lines []string) bool {
	var pids []string
	for _, line := range lines {
		if strings.Contains(line, commitNowArgs) {
			pids = append(pids, logPID(line))
		}
	}
	for _, line := range lines {
		parsed, ok := log.ParseLogLine(line)
		if !ok || parsed.Component != "process" || parsed.Message != "exit" {
			continue
		}
		if slices.Contains(pids, logPID(line)) && slices.Contains(strings.Fields(line), "code=0") {
			return true
		}
	}
	return false
}

func logPID(line string) string {
	for field := range strings.FieldsSeq(line) {
		if pid, ok := strings.CutPrefix(field, "pid="); ok {
			return pid
		}
	}
	return ""
}

func isCompletedFlush(line string) bool {
	parsed, ok := log.ParseLogLine(line)
	return ok && parsed.Component == "daemon" && parsed.Message == "shutdown" &&
		slices.Contains(strings.Fields(line), "flush_completed=true")
}

func assertNoRecoveryTail(t *testing.T, lines []string) {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, recoverArgs) {
			t.Errorf("the recovery tail started: %s", line)
		}
		if strings.Contains(line, markerClear) {
			t.Errorf("a pending-marker clear was attempted: %s", line)
		}
	}
}

// newLogLines answers the lines of log written after mark, a read of the same
// log taken earlier.
func newLogLines(full, mark string) []string {
	added := strings.TrimPrefix(full, mark)
	return strings.FieldsFunc(added, func(r rune) bool { return r == '\n' })
}

func (rt *waitingPaneRuntime) waitingDiagnostic() string {
	return fmt.Sprintf("%s\n--- portal.log ---\n%s", rt.diagnostic(), portaltest.ReadPortalLogSafe(rt.stateDir))
}
