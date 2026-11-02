package tmux_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestListWaitingPanesInSession(t *testing.T) {
	t.Run("it returns only the panes whose resume-pending marker is set, with their pids", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("1|4906|%4\n|5000|%5\n1|6100|%6", "list-panes"))

		got, err := tmux.NewClient(mock).ListWaitingPanesInSession("dev")
		if err != nil {
			t.Fatalf("ListWaitingPanesInSession: %v", err)
		}

		want := []tmux.WaitingPane{{ID: "%4", PID: 4906}, {ID: "%6", PID: 6100}}
		if !slices.Equal(got, want) {
			t.Errorf("waiting panes = %+v, want %+v", got, want)
		}
	})

	t.Run("it lists the session's panes across every window under the exact-match target", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "list-panes"))

		if _, err := tmux.NewClient(mock).ListWaitingPanesInSession("dev"); err != nil {
			t.Fatalf("ListWaitingPanesInSession: %v", err)
		}

		calls := mock.Calls()
		if len(calls) != 1 {
			t.Fatalf("tmux calls = %v, want one list-panes", calls)
		}
		argv := calls[0]
		if !slices.Contains(argv, "-s") {
			t.Errorf("argv = %q, want -s so every window of the session is read", argv)
		}
		if i := slices.Index(argv, "-t"); i < 0 || i+1 >= len(argv) || argv[i+1] != string(tmux.CoordTargetExact("dev")) {
			t.Errorf("argv = %q, want -t %s", argv, tmux.CoordTargetExact("dev"))
		}
	})

	t.Run("it answers an empty slice for a session with nothing waiting", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("|5000|%5", "list-panes"))

		got, err := tmux.NewClient(mock).ListWaitingPanesInSession("dev")
		if err != nil {
			t.Fatalf("ListWaitingPanesInSession: %v", err)
		}
		if got == nil || len(got) != 0 {
			t.Errorf("waiting panes = %#v, want an empty, non-nil slice", got)
		}
	})

	t.Run("it returns the error when tmux fails", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Fails(errors.New("can't find session: dev"), "list-panes"))

		got, err := tmux.NewClient(mock).ListWaitingPanesInSession("dev")
		if err == nil {
			t.Fatal("ListWaitingPanesInSession error = nil, want the tmux failure")
		}
		if got != nil {
			t.Errorf("waiting panes = %+v on a failed read, want nil", got)
		}
	})

	t.Run("it refuses a row it cannot read rather than signalling a guessed group", func(t *testing.T) {
		for _, row := range []string{"1|4906", "1|notapid|%4", "1|0|%4", "1|-7|%4"} {
			t.Run(row, func(t *testing.T) {
				mock := commandertest.New(t, commandertest.Returns(row, "list-panes"))
				if _, err := tmux.NewClient(mock).ListWaitingPanesInSession("dev"); err == nil {
					t.Errorf("ListWaitingPanesInSession(%q) error = nil, want a parse error", row)
				}
			})
		}
	})
}

func TestListWaitingPanesInSession_ReadsMarkedPaneFromRealServer(t *testing.T) {
	const (
		waiting = "wps-waiting"
		other   = "wps-other"
	)
	ts, client, _ := seedRealTmuxServer(t, realTmuxFixture{
		socketPrefix: "ptl-waiting-panes-",
		sessions:     []string{waiting, other},
		topology: func(t *testing.T, ts *tmuxtest.Socket, _ *tmux.Client, _ string) {
			ts.Run(t, "split-window", "-t", waiting+":0")
		},
	})
	marked := sessionPaneIDs(t, ts, waiting)[1]
	ts.MarkResumePending(t, tmux.PaneIDTarget(marked))
	ts.MarkResumePending(t, tmux.PaneIDTarget(sessionPaneIDs(t, ts, other)[0]))

	got, err := client.ListWaitingPanesInSession(waiting)
	if err != nil {
		t.Fatalf("ListWaitingPanesInSession: %v", err)
	}

	if len(got) != 1 || got[0].ID != marked {
		t.Fatalf("waiting panes = %+v, want exactly %s: the other session's marked pane is not this session's", got, marked)
	}
	wantPID := strings.TrimSpace(ts.Run(t, "display-message", "-p", "-t", marked, "#{pane_pid}"))
	if strconv.Itoa(got[0].PID) != wantPID {
		t.Errorf("pid = %d, want the pane's own process %s", got[0].PID, wantPID)
	}
}

func TestClientShowingPane(t *testing.T) {
	t.Run("it names the client whose current pane is the pane", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("1720000000|%3|/dev/ttys001\n1720000009|%4|/dev/ttys002", "list-clients"))

		name, ok, err := tmux.NewClient(mock).ClientShowingPane("%4")
		if err != nil {
			t.Fatalf("ClientShowingPane: %v", err)
		}
		if !ok || name != "/dev/ttys002" {
			t.Errorf("ClientShowingPane(%%4) = %q, %v, want /dev/ttys002, true", name, ok)
		}
	})

	t.Run("it picks the most recently active of several clients showing the pane", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns(
			"1720000005|%4|/dev/ttys001\n1720000009|%4|/dev/ttys002\n1720000001|%4|/dev/ttys003\n1720000099|%9|/dev/ttys004",
			"list-clients"))

		name, ok, err := tmux.NewClient(mock).ClientShowingPane("%4")
		if err != nil {
			t.Fatalf("ClientShowingPane: %v", err)
		}
		if !ok || name != "/dev/ttys002" {
			t.Errorf("ClientShowingPane(%%4) = %q, %v, want /dev/ttys002, true: the busier client shows another pane", name, ok)
		}
	})

	t.Run("it reports no client when none is showing the pane", func(t *testing.T) {
		for _, out := range []string{"", "1720000000|%3|/dev/ttys001"} {
			mock := commandertest.New(t, commandertest.Returns(out, "list-clients"))

			name, ok, err := tmux.NewClient(mock).ClientShowingPane("%4")
			if err != nil {
				t.Fatalf("ClientShowingPane: %v", err)
			}
			if ok || name != "" {
				t.Errorf("ClientShowingPane(%%4) over %q = %q, %v, want no client", out, name, ok)
			}
		}
	})

	t.Run("it does not mistake one pane id for another it prefixes", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("1720000000|%41|/dev/ttys001", "list-clients"))

		if _, ok, _ := tmux.NewClient(mock).ClientShowingPane("%4"); ok {
			t.Error("ClientShowingPane(%4) matched a client showing %41")
		}
	})

	t.Run("it returns the error when tmux fails", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Fails(errors.New("no server running"), "list-clients"))

		if _, ok, err := tmux.NewClient(mock).ClientShowingPane("%4"); err == nil || ok {
			t.Errorf("ClientShowingPane() = ok %v, error %v, want the tmux failure", ok, err)
		}
	})

	t.Run("it refuses a malformed row", func(t *testing.T) {
		for _, out := range []string{"1720000000|%4", "soon|%4|/dev/ttys001"} {
			mock := commandertest.New(t, commandertest.Returns(out, "list-clients"))
			if _, _, err := tmux.NewClient(mock).ClientShowingPane("%4"); err == nil {
				t.Errorf("ClientShowingPane() over %q error = nil, want a parse error", out)
			}
		}
	})
}

func TestDetachClientTo(t *testing.T) {
	t.Run("it detaches the named client and hands it the command", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("", "detach-client"))

		err := tmux.NewClient(mock).DetachClientTo(tmux.ClientNameTarget("/dev/ttys002"), "'/opt/portal' open")
		if err != nil {
			t.Fatalf("DetachClientTo: %v", err)
		}

		want := []string{"detach-client", "-t", "/dev/ttys002", "-E", "'/opt/portal' open"}
		if calls := mock.Calls(); len(calls) != 1 || !slices.Equal(calls[0], want) {
			t.Errorf("tmux calls = %q, want one %q", calls, want)
		}
	})

	t.Run("it returns the error when tmux refuses", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Fails(errors.New("can't find client: /dev/ttys002"), "detach-client"))

		if err := tmux.NewClient(mock).DetachClientTo(tmux.ClientNameTarget("/dev/ttys002"), "x"); err == nil {
			t.Error("DetachClientTo error = nil, want the tmux refusal")
		}
	})
}
