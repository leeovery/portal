//go:build integration

package restore_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	// The control's hook program records its own pid in the pane's working
	// directory and runs until the pane ends.
	runningHookPIDFile = "running-hook.pid"
	runningHookCommand = "sh -c 'echo $$ > " + runningHookPIDFile + "; exec sleep 60'"

	// paneEndBudget bounds how long a pane's processes take to end once tmux has
	// closed its terminal: a SIGHUP ends them at once.
	paneEndBudget = 3 * time.Second

	chainStartArgs     = "args=\"state "
	recoverStartArgs   = chainStartArgs + resumeRecoverArgv
	markerClearFailure = "unset resume pending marker failed"
)

func TestResumePanes_EndWhenTheirTerminalCloses(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	fx, hookPID := setupHangupPanes(t)

	t.Run("a killed eager pane whose hook program still runs ends at once and leaves the saved state", func(t *testing.T) {
		shell := fx.panePID(t, controlPaneTarget())
		files := savedScrollbackOf(t, fx.stateDir, eagerControlSession)

		fx.ts.Run(t, "kill-session", "-t", string(tmux.CoordTargetExact(eagerControlSession)))

		awaitProcessesEnded(t, map[string]int{"the eager resume shell": shell, "the hook program": hookPID})
		assertKillCommitted(t, fx, eagerControlSession, files)
	})

	t.Run("tmux's exit ends a waiting pane without starting its recovery tail", func(t *testing.T) {
		tree := fx.restingTree(t)
		assertRestingTreeShape(t, tree)
		idx, _, err := state.ReadIndex(fx.stateDir)
		if err != nil {
			t.Fatalf("ReadIndex: %v", err)
		}
		tokenPath := state.PendingScrollbackFile(lazySubjectToken)
		if got := fx.subjectRecord(t, idx).ScrollbackFile; got != tokenPath {
			t.Fatalf("the waiting pane was saved naming %q; want %q", got, tokenPath)
		}
		transcript := fx.readScrollbackAt(t, tokenPath)
		logBefore := portaltest.ReadPortalLogSafe(fx.stateDir)
		if !strings.Contains(logBefore, chainStartArgs+resumeWaitArgv) {
			t.Fatalf("portal.log carries no start line from the waiting pane's chain, so it cannot show the recovery tail's absence:\n%s", logBefore)
		}

		restoretest.OpenRebootGap(t, fx.ts)

		awaitProcessesEnded(t, map[string]int{"the parked shell": tree[0].pid, "the waiter": tree[1].pid})
		logged := strings.TrimPrefix(portaltest.ReadPortalLogSafe(fx.stateDir), logBefore)
		for line := range strings.SplitSeq(logged, "\n") {
			if strings.Contains(line, recoverStartArgs) {
				t.Errorf("the recovery tail started after tmux's exit: %s", line)
			}
			if strings.Contains(line, markerClearFailure) {
				t.Errorf("a pending-marker clear was attempted after tmux's exit: %s", line)
			}
		}

		saved, _, err := state.ReadIndex(fx.stateDir)
		if err != nil {
			t.Fatalf("ReadIndex after tmux's exit: %v", err)
		}
		if got := fx.subjectRecord(t, saved).ScrollbackFile; got != tokenPath {
			t.Errorf("after tmux's exit the waiting pane's record names %q; want %q", got, tokenPath)
		}
		if got := fx.readScrollbackAt(t, tokenPath); string(got) != string(transcript) {
			t.Errorf("the token-named transcript changed through tmux's exit\nbefore:\n%s\nafter:\n%s", transcript, got)
		}

		fx.rebootRestoreHydrateSessions(t, []string{lazySubjectSession})
		fx.awaitScreenContains(t, subjectPaneTarget(), panelTitle)
		fx.assertPending(t, subjectPaneTarget())
	})

	t.Run("a killed waiting pane ends at once and leaves the saved state", func(t *testing.T) {
		tree := fx.restingTree(t)
		assertRestingTreeShape(t, tree)
		files := savedScrollbackOf(t, fx.stateDir, lazySubjectSession)

		fx.ts.Run(t, "kill-session", "-t", string(tmux.CoordTargetExact(lazySubjectSession)))

		awaitProcessesEnded(t, map[string]int{"the parked shell": tree[0].pid, "the waiter": tree[1].pid})
		assertKillCommitted(t, fx, lazySubjectSession, files)
	})
}

// setupHangupPanes leaves the lazy subject waiting on its panel beside an eager
// control whose hook program is still running, both saved, and answers the
// hook program's pid.
func setupHangupPanes(t *testing.T) (*lazyPanelFixture, int) {
	t.Helper()
	fx := setupLazyResumePanel(t)
	if err := hooks.NewStore(os.Getenv("PORTAL_HOOKS_FILE")).Set(eagerControlToken, "on-resume",
		hooks.Registration{Command: runningHookCommand, Resume: resumemode.Eager}, hooks.ViaCLI); err != nil {
		t.Fatalf("hooks.Set control: %v", err)
	}
	fx.captureRound(t)
	fx.rebootRestoreHydrate(t)

	pidPath := filepath.Join(fx.workDir, runningHookPIDFile)
	restoretest.WaitForFileExists(t, pidPath, restoretest.HydrateBudget, restoretest.HydrateTick)
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read the hook program's pid: %v", err)
	}
	hookPID, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("the hook program recorded pid %q: %v", raw, err)
	}

	fx.awaitScreenContains(t, subjectPaneTarget(), panelTitle)
	fx.assertPending(t, subjectPaneTarget())
	fx.captureRound(t)
	return fx, hookPID
}

// savedScrollbackOf answers the scrollback files the saved state names for one
// session, each one present on disk.
func savedScrollbackOf(t *testing.T, stateDir, session string) []string {
	t.Helper()
	idx, _, err := state.ReadIndex(stateDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	var files []string
	for _, w := range restoretest.FindCapturedSession(t, idx, session).Windows {
		for _, p := range w.Panes {
			if _, err := os.Stat(filepath.Join(stateDir, p.ScrollbackFile)); err != nil {
				t.Fatalf("saved scrollback %s of %s: %v", p.ScrollbackFile, session, err)
			}
			files = append(files, p.ScrollbackFile)
		}
	}
	if len(files) == 0 {
		t.Fatalf("the saved state names no scrollback for %s; the fixture proves nothing", session)
	}
	return files
}

// awaitProcessesEnded reads only pids the fixture's own panes reported.
func awaitProcessesEnded(t *testing.T, pids map[string]int) {
	t.Helper()
	for what, pid := range pids {
		ended := harnesstest.PollUntil(t, paneEndBudget, 10*time.Millisecond, func() bool {
			return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
		})
		if !ended {
			t.Errorf("%s (pid %d) outlived its pane's terminal by %s", what, pid, paneEndBudget)
		}
	}
}

// assertKillCommitted takes the commit the kill's session-closed hook runs and
// checks it removed the session and every scrollback file it saved.
func assertKillCommitted(t *testing.T, fx *lazyPanelFixture, session string, files []string) {
	t.Helper()
	idx := fx.commitNowRound(t)
	for _, s := range idx.Sessions {
		if s.Name == session {
			t.Errorf("the saved state still holds killed session %s", session)
		}
	}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(fx.stateDir, f)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("killed session %s's scrollback %s is still on disk (stat: %v)", session, f, err)
		}
	}
}
