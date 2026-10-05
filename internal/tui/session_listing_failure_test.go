package tui

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

func TestFetchSessions_FailedListingListsNoSessionsWithoutError(t *testing.T) {
	mock := commandertest.New(t, commandertest.Fails(&tmux.CommandError{
		Args:   []string{"list-sessions"},
		Stderr: "no server running on /tmp/tmux-501/default",
		Err:    errors.New("exit status 1"),
	}, "list-sessions"))
	m := Build(Deps{Lister: tmux.NewClient(mock)})

	msg, ok := m.fetchSessionsCmd()().(SessionsMsg)

	if !ok {
		t.Fatalf("fetchSessionsCmd produced %T, want SessionsMsg", msg)
	}
	if msg.Err != nil {
		t.Errorf("Err = %v, want nil", msg.Err)
	}
	if len(msg.Sessions) != 0 {
		t.Errorf("Sessions = %v, want none", msg.Sessions)
	}
}
