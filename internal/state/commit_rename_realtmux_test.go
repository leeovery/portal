package state_test

import (
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestCommitRealTmuxTellsARenameFromADropBesideANewSession(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-renamedrop-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}
	paneDir := t.TempDir()
	for _, name := range []string{"alpha", "bravo"} {
		ts.Run(t, "new-session", "-d", "-s", name, "-c", paneDir)
	}
	waitForListedSessions(t, ts, tmux.PortalBootstrapName, "alpha", "bravo")
	stateDir := t.TempDir()
	captureAndCommit(t, client, stateDir, nil)

	movePaneOn(t, ts, "bravo", t.TempDir())
	ts.Run(t, "rename-session", "-t", "=bravo:", "zulu")
	waitForListedSessions(t, ts, tmux.PortalBootstrapName, "alpha", "zulu")
	logger, sink := logtest.NewCaptureLogger(t)
	captureAndCommit(t, client, stateDir, logger)

	committed := sessionNames(readIndexFile(t, stateDir))
	slices.Sort(committed)
	if !slices.Equal(committed, []string{"alpha", "zulu"}) {
		t.Fatalf("committed sessions = %v, want [alpha zulu]", committed)
	}
	if got := droppedSessionNames(t, sink); len(got) != 0 {
		t.Errorf("a rename logged dropped sessions %v, want none", got)
	}

	ts.Run(t, "kill-session", "-t", "=alpha:")
	ts.Run(t, "new-session", "-d", "-s", "yankee", "-c", paneDir)
	waitForListedSessions(t, ts, tmux.PortalBootstrapName, "zulu", "yankee")
	logger, sink = logtest.NewCaptureLogger(t)
	captureAndCommit(t, client, stateDir, logger)

	if got, want := droppedSessionNames(t, sink), []string{"alpha"}; !slices.Equal(got, want) {
		t.Errorf("dropped sessions logged = %v, want %v", got, want)
	}
}

// movePaneOn changes the session's pane folder and starts a program in it, so
// the saved record lags the live pane as it does between captures.
func movePaneOn(t *testing.T, ts *tmuxtest.Socket, session, dir string) {
	t.Helper()
	target := string(tmux.PaneTargetExact(session, 0, 0))
	ts.Run(t, "send-keys", "-t", target, "cd "+dir+" && exec sleep 30", "Enter")
	movedOn := func() bool {
		out, err := ts.TryRun("display-message", "-p", "-t", target, "#{pane_current_command}")
		return err == nil && strings.Contains(out, "sleep")
	}
	if !harnesstest.PollUntil(t, 2*time.Second, 20*time.Millisecond, movedOn) {
		t.Fatalf("%q's pane did not start sleep within 2s", session)
	}
}

func captureAndCommit(t *testing.T, client *tmux.Client, stateDir string, logger *slog.Logger) {
	t.Helper()
	idx, _, err := state.CaptureStructure(client, nil, nil, logger)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	if err := state.Commit(stateDir, idx, false, logger); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}
