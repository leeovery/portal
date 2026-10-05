package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// withOwnTmuxServer gives commit-now the TMUX a session-closed hook's run-shell
// job inherits, naming pid as the server that ran it. The socket stays a dead
// one, so a tmux read the test forgot to inject still fails loudly.
func withOwnTmuxServer(t *testing.T, pid int) {
	t.Helper()
	t.Setenv("TMUX", fmt.Sprintf("/nonexistent/portal-test-own-server,%d,0", pid))
}

// anotherServer answers every capture read as the committer's own server would
// and its confirmation as a server started since on the same socket.
func anotherServer() *daemonFakeCommander {
	fc := workOnlyCommander(nil)
	fc.answeringPID = fakeOwnServerPID + 1
	return fc
}

func TestCommittersStandDownOnAConfirmationFromAnotherServer(t *testing.T) {
	for _, c := range committers {
		t.Run(c.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := anotherServer()

			c.run(t, saved, fc)

			if len(fc.callsContaining("display-message")) == 0 {
				t.Fatal("no confirmation read sent")
			}
			saved.assertUnchanged(t)
		})
	}
}

func TestShutdownFlushReportsAConfirmationFromAnotherServerAsIncomplete(t *testing.T) {
	saved := seedSavedState(t)
	deps := makeDeps(t, saved.dir, anotherServer())
	deps.PrevIndex = &saved.index
	logger, sink := logtest.NewCaptureLogger(t)
	deps.Logger = logger

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	failed := sink.Records().WithMessage("final flush failed").Only(t, "final flush failure")
	if err := failed.ErrorAttr(t, "error"); !errors.Is(err, state.ErrNotOwnServer) {
		t.Errorf("final flush failure error = %v, want one wrapping ErrNotOwnServer", err)
	}
	shutdown := sink.Records().WithMessage("shutdown").AtExactLevel(slog.LevelInfo).Only(t, "shutdown line")
	if got := shutdown.AttrString(t, "flush_completed"); got != "false" {
		t.Errorf("shutdown flush_completed = %q, want false", got)
	}
	saved.assertUnchanged(t)
}

func TestCommitNowFailsOnAConfirmationFromAnotherServer(t *testing.T) {
	saved := seedSavedState(t)
	withOwnTmuxServer(t, fakeOwnServerPID)
	client := tmux.NewClient(anotherServer())
	withCommitNowDeps(t, CommitNowDeps{
		NewClient:   func() state.CaptureCycleClient { return client },
		IsRestoring: func() (bool, error) { return state.IsRestoringSet(client) },
	})

	_, _, err := runRootCmd(t, "state", "commit-now")

	if !errors.Is(err, errCommitNowFailed) {
		t.Errorf("commit-now error = %v, want one wrapping errCommitNowFailed", err)
	}
	saved.assertUnchanged(t)
}

func TestCommitNowStandsDownOutsideAnyTmuxServer(t *testing.T) {
	saved := seedSavedState(t)
	t.Setenv("TMUX", "")
	client := tmux.NewClient(workOnlyCommander(nil))
	withCommitNowDeps(t, CommitNowDeps{
		NewClient:   func() state.CaptureCycleClient { return client },
		IsRestoring: func() (bool, error) { return state.IsRestoringSet(client) },
	})

	_, _, err := runRootCmd(t, "state", "commit-now")

	if !errors.Is(err, errCommitNowFailed) {
		t.Errorf("commit-now error = %v, want one wrapping errCommitNowFailed", err)
	}
	saved.assertUnchanged(t)
}

func TestDaemonTakesItsOwnServerFromTheTMUXItsPaneRunsUnder(t *testing.T) {
	t.Setenv("PORTAL_STATE_DIR", t.TempDir())
	t.Setenv("TMUX", "/nonexistent/portal-test-saver-pane,5151,2")
	holder := withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("runRootCmd(state daemon): %v", err)
	}

	if *holder == nil {
		t.Fatal("daemonRunFunc not invoked")
	}
	if got := (*holder).OwnServer; got != 5151 {
		t.Errorf("daemon's own server = %d, want 5151 from TMUX", got)
	}
}
