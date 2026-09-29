package tmux_test

import (
	"reflect"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestListWindowsAndPanesInSession_CarriesTokenAndPendingFromRealServer(t *testing.T) {
	const (
		session = "lwp-session"
		token   = "lwpTokenABCDEFGH"
	)
	ts, client, _ := seedRealTmuxServer(t, realTmuxFixture{
		socketPrefix: "ptl-list-windows-panes-",
		sessions:     []string{session},
		topology: func(t *testing.T, ts *tmuxtest.Socket, _ *tmux.Client, _ string) {
			ts.Run(t, "split-window", "-t", session+":0")
		},
	})
	second := tmux.PaneIDTarget(sessionPaneIDs(t, ts, session)[1])
	if err := client.SetPaneOption(second, state.PortalPaneIDOption, token); err != nil {
		t.Fatalf("SetPaneOption token: %v", err)
	}
	ts.MarkResumePending(t, second)

	groups, err := client.ListWindowsAndPanesInSession(session)
	if err != nil {
		t.Fatalf("ListWindowsAndPanesInSession: %v", err)
	}

	want := []tmux.WindowPane{{Index: 0}, {Index: 1, Token: token, Pending: true}}
	if len(groups) != 1 || !reflect.DeepEqual(groups[0].Panes, want) {
		t.Errorf("groups = %+v, want one window holding panes %+v", groups, want)
	}
}
