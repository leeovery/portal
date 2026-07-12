package tmux_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestListClients(t *testing.T) {
	t.Run("it parses client_pid and client_activity lines into ClientInfo", func(t *testing.T) {
		mock := &MockCommander{Output: "501 1720000000\n502 1720000005"}
		client := tmux.NewClient(mock)

		got, err := client.ListClients("dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []tmux.ClientInfo{
			{PID: 501, Activity: 1720000000},
			{PID: 502, Activity: 1720000005},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d clients, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i].PID != want[i].PID {
				t.Errorf("client[%d].PID = %d, want %d", i, got[i].PID, want[i].PID)
			}
			if got[i].Activity != want[i].Activity {
				t.Errorf("client[%d].Activity = %d, want %d", i, got[i].Activity, want[i].Activity)
			}
		}
	})

	t.Run("it parses a single client line", func(t *testing.T) {
		mock := &MockCommander{Output: "777 1699999999"}
		client := tmux.NewClient(mock)

		got, err := client.ListClients("dev")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d clients, want 1", len(got))
		}
		if got[0].PID != 777 || got[0].Activity != 1699999999 {
			t.Errorf("client = %+v, want {PID:777 Activity:1699999999}", got[0])
		}
	})

	t.Run("it tolerates the no-clients case as an empty slice", func(t *testing.T) {
		mock := &MockCommander{Output: "", Err: fmt.Errorf("exit status 1")}
		client := tmux.NewClient(mock)

		got, err := client.ListClients("dev")
		if err != nil {
			t.Fatalf("unexpected error: %v, want nil", err)
		}
		if got == nil {
			t.Fatal("got nil slice, want a non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("got %d clients, want 0", len(got))
		}
	})

	t.Run("it tolerates empty output as an empty slice", func(t *testing.T) {
		mock := &MockCommander{Output: ""}
		client := tmux.NewClient(mock)

		got, err := client.ListClients("dev")
		if err != nil {
			t.Fatalf("unexpected error: %v, want nil", err)
		}
		if got == nil {
			t.Fatal("got nil slice, want a non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("got %d clients, want 0", len(got))
		}
	})

	t.Run("it errors on a malformed client line", func(t *testing.T) {
		cases := []struct {
			name   string
			output string
		}{
			{"non-numeric pid", "notapid 1720000000"},
			{"non-numeric activity", "501 notanactivity"},
			{"missing activity field", "501"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				mock := &MockCommander{Output: tc.output}
				client := tmux.NewClient(mock)

				if _, err := client.ListClients("dev"); err == nil {
					t.Errorf("ListClients(%q) returned nil error, want a parse error", tc.output)
				}
			})
		}
	})

	t.Run("it targets the session exactly and requests pid+activity", func(t *testing.T) {
		mock := &MockCommander{Output: "501 1720000000"}
		client := tmux.NewClient(mock)

		if _, err := client.ListClients("dev"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mock.Calls) == 0 {
			t.Fatal("expected at least one tmux call")
		}
		args := strings.Join(mock.Calls[0], " ")
		if !strings.Contains(args, "list-clients") {
			t.Errorf("args %q do not invoke list-clients", args)
		}
		if !strings.Contains(args, "-t =dev") {
			t.Errorf("args %q do not target the session exactly (=dev)", args)
		}
		if !strings.Contains(args, "#{client_pid} #{client_activity}") {
			t.Errorf("args %q do not request the client_pid+client_activity format", args)
		}
	})
}
