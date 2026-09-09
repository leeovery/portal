package tmux_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestListAllPaneHookKeys_StampedAndUnstampedInOneRead(t *testing.T) {
	const sessionName = "lapk-mix"
	ts, client, _ := seedRealTmuxServer(t, hookKeyFixture(sessionName,
		func(t *testing.T, ts *tmuxtest.Socket, _ *tmux.Client, _ string) {
			ts.Run(t, "split-window", "-t", sessionName+":0")
		}))

	ts.StampPaneToken(t, sessionName+":0.0", "tokMix")

	rows, err := client.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}

	stamped := findPaneHookRow(t, rows, sessionName+":0.0")
	if stamped.Token != "tokMix" {
		t.Errorf("stamped pane token = %q, want %q", stamped.Token, "tokMix")
	}
	sibling := findPaneHookRow(t, rows, sessionName+":0.1")
	if sibling.Token != "" {
		t.Errorf("unstamped sibling token = %q, want empty (a stamp does not inherit across a split)", sibling.Token)
	}
}

func TestListAllPaneHookKeys_ListPanesFailurePropagates(t *testing.T) {
	ts, client, _ := seedRealTmuxServer(t, hookKeyFixture("lapk-fail", nil))

	// Tear the server down so the subsequent list-panes -a fails with "no
	// server running" — the reliable read-failure path.
	ts.KillServer()

	rows, err := client.ListAllPaneHookKeys()
	if err == nil {
		t.Fatal("expected a wrapped error from a failed list-panes -a read, got nil")
	}
	if rows != nil {
		t.Errorf("rows on read failure = %+v, want nil (MUST NOT treat a tmux failure as an empty live set)", rows)
	}

	if _, ok := errors.AsType[*tmux.CommandError](err); !ok {
		t.Errorf("errors.AsType[*tmux.CommandError](err) = false; want a recoverable *tmux.CommandError; err = %v", err)
	}
}

func findPaneHookRow(t *testing.T, rows []tmux.PaneHookRow, location string) tmux.PaneHookRow {
	t.Helper()
	for _, row := range rows {
		if row.Location == location {
			return row
		}
	}
	t.Fatalf("no row for pane %q in %+v", location, rows)
	return tmux.PaneHookRow{}
}
