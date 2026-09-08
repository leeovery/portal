package tmux_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

func TestListClients(t *testing.T) {
	t.Run("it parses client_pid and client_activity lines into ClientInfo", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("501 1720000000\n502 1720000005", "list-clients"))
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
		mock := commandertest.New(t, commandertest.Returns("777 1699999999", "list-clients"))
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
		mock := commandertest.New(t, commandertest.Fails(fmt.Errorf("exit status 1"), "list-clients"))
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
		mock := commandertest.New(t, commandertest.Returns("", "list-clients"))
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
				mock := commandertest.New(t, commandertest.Returns(tc.output, "list-clients"))
				client := tmux.NewClient(mock)

				if _, err := client.ListClients("dev"); err == nil {
					t.Errorf("ListClients(%q) returned nil error, want a parse error", tc.output)
				}
			})
		}
	})

	t.Run("it targets the session exactly and requests pid+activity", func(t *testing.T) {
		mock := commandertest.New(t, commandertest.Returns("501 1720000000", "list-clients"))
		client := tmux.NewClient(mock)

		if _, err := client.ListClients("dev"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(mock.Calls()) == 0 {
			t.Fatal("expected at least one tmux call")
		}
		// The trailing ":" is the exactness, not the "=" alone: list-clients
		// parses its -t as a window or pane target, so a separator-free "=dev"
		// is read as a window name and falls through to the fuzzy lookup that
		// reaches a live prefix sibling.
		want := []string{"list-clients", "-t", "=dev:", "-F", "#{client_pid} #{client_activity}"}
		if got := mock.Calls()[0]; !slices.Equal(got, want) {
			t.Errorf("composed %v, want %v", got, want)
		}
	})
}
