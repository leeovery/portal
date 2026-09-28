package tmux_test

import (
	"testing"

	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestListPendingResumePanes_ReadsMarkedPaneFromRealServer(t *testing.T) {
	const (
		waiting = "rpr-waiting"
		idle    = "rpr-idle"
	)
	ts, client, _ := seedRealTmuxServer(t, realTmuxFixture{
		socketPrefix: "ptl-resume-pending-",
		sessions:     []string{waiting, idle},
		topology: func(t *testing.T, ts *tmuxtest.Socket, _ *tmux.Client, _ string) {
			ts.Run(t, "split-window", "-t", waiting+":0")
			ts.Run(t, "split-window", "-t", idle+":0")
		},
	})
	ts.MarkResumePending(t, tmux.PaneIDTarget(sessionPaneIDs(t, ts, waiting)[1]))

	view, err := client.ListPendingResumePanes()
	if err != nil {
		t.Fatalf("ListPendingResumePanes: %v", err)
	}

	if view.Panes != 1 {
		t.Errorf("Panes = %d, want 1", view.Panes)
	}
	if _, ok := view.Sessions[waiting]; !ok || len(view.Sessions) != 1 {
		t.Errorf("Sessions = %v, want exactly {%q}", view.Sessions, waiting)
	}
	unmarked := map[string]int{}
	for _, row := range view.Rows {
		if !row.Pending {
			unmarked[row.Session]++
		}
	}
	if unmarked[waiting] != 1 || unmarked[idle] != 2 {
		t.Errorf("unmarked panes per session = %v, want %s:1 and %s:2 (rows %+v)", unmarked, waiting, idle, view.Rows)
	}
}
