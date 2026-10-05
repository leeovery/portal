package cmd

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

var captureReadCommands = []string{"show-options", "list-sessions", "list-panes", "show-environment"}

// workOnlyCommander answers every capture read with "work" alone live, so a
// commit drops the saved "notes" and deletes its transcript.
func workOnlyCommander(confirmErr error) *daemonFakeCommander {
	sessions, panes := oneSession()
	return &daemonFakeCommander{sessionsOut: sessions, panesOut: panes, confirmErr: confirmErr}
}

func refusedConfirmation() error {
	return &tmux.CommandError{
		Args:   []string{"display-message"},
		Stderr: "no server running",
		Err:    errors.New("exit status 1"),
	}
}

// committer runs one committing cycle against fc over the saved state.
type committer struct {
	name string
	run  func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander)
}

var committers = []committer{
	{"the daemon's tick", func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander) {
		deps := makeDeps(t, saved.dir, fc)
		deps.PrevIndex = &saved.index
		tick(t.Context(), deps)
	}},
	{"the daemon's shutdown flush", func(t *testing.T, saved savedStateFixture, fc *daemonFakeCommander) {
		deps := makeDeps(t, saved.dir, fc)
		deps.PrevIndex = &saved.index
		if err := defaultShutdownFlush(deps); err != nil {
			t.Fatalf("defaultShutdownFlush: %v", err)
		}
	}},
	{"commit-now", func(t *testing.T, _ savedStateFixture, fc *daemonFakeCommander) {
		withOwnTmuxServer(t, fakeOwnServerPID)
		client := tmux.NewClient(fc)
		withCommitNowDeps(t, CommitNowDeps{
			NewClient:   func() state.CaptureCycleClient { return client },
			IsRestoring: func() (bool, error) { return state.IsRestoringSet(client) },
		})
		_, _, _ = runRootCmd(t, "state", "commit-now")
	}},
}

func TestCommittersStandDownOnARefusedConfirmation(t *testing.T) {
	for _, c := range committers {
		t.Run(c.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := workOnlyCommander(refusedConfirmation())

			c.run(t, saved, fc)

			if len(fc.callsContaining("display-message")) == 0 {
				t.Fatal("no confirmation read sent")
			}
			saved.assertUnchanged(t)
		})
	}
}

func TestCommitNowFailsOnARefusedConfirmation(t *testing.T) {
	seedSavedState(t)
	withOwnTmuxServer(t, fakeOwnServerPID)
	fc := workOnlyCommander(refusedConfirmation())
	client := tmux.NewClient(fc)
	withCommitNowDeps(t, CommitNowDeps{
		NewClient:   func() state.CaptureCycleClient { return client },
		IsRestoring: func() (bool, error) { return state.IsRestoringSet(client) },
	})

	_, _, err := runRootCmd(t, "state", "commit-now")

	if !errors.Is(err, errCommitNowFailed) {
		t.Errorf("commit-now error = %v, want one wrapping errCommitNowFailed", err)
	}
	if len(fc.callsContaining("display-message")) == 0 {
		t.Fatal("no confirmation read sent")
	}
	if err == nil || !strings.Contains(err.Error(), "confirm tmux answering") {
		t.Errorf("commit-now error = %v, want the refused confirmation to have ended it", err)
	}
}

func TestCommittersConfirmAfterTheLastCaptureRead(t *testing.T) {
	for _, c := range committers {
		t.Run(c.name, func(t *testing.T) {
			saved := seedSavedState(t)
			fc := workOnlyCommander(nil)

			c.run(t, saved, fc)

			committed, skip, err := state.ReadIndex(saved.dir)
			if err != nil || skip {
				t.Fatalf("ReadIndex = (skip %v, err %v), want the committed index", skip, err)
			}
			if len(committed.Sessions) != 1 || committed.Sessions[0].Name != "work" {
				t.Fatalf("committed sessions = %+v, want work alone", committed.Sessions)
			}
			names := make([]string, 0, len(fc.calls))
			for _, call := range fc.calls {
				names = append(names, call[0])
			}
			confirmAt := slices.Index(names, "display-message")
			if confirmAt < 0 {
				t.Fatalf("calls = %v, want a confirmation read", names)
			}
			for i, name := range names[confirmAt+1:] {
				if slices.Contains(captureReadCommands, name) {
					t.Errorf("calls = %v: capture read %q at %d sent after the confirmation", names, name, confirmAt+1+i)
				}
			}
		})
	}
}
