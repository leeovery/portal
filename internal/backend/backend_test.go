package backend_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/portal/internal/backend"
)

func TestSession(t *testing.T) {
	t.Run("it is attached only while a client is attached", func(t *testing.T) {
		for clients, want := range map[int]bool{0: false, 1: true, 3: true} {
			if got := (backend.Session{Clients: clients}).Attached(); got != want {
				t.Errorf("Attached() with %d clients = %v, want %v", clients, got, want)
			}
		}
	})

	t.Run("it claims no answer from the backend at its zero value", func(t *testing.T) {
		if (backend.Session{Name: "a"}).Answered {
			t.Error("a Session built with only a Name reads as answered")
		}
	})

	t.Run("it is Portal's own when its name starts with an underscore", func(t *testing.T) {
		for name, want := range map[string]bool{
			"_portal-saver":   true,
			"_":               true,
			"dotfiles-PpaTER": false,
			"a_b":             false,
			"":                false,
		} {
			if got := (backend.Session{Name: name}).PortalOwned(); got != want {
				t.Errorf("PortalOwned() for %q = %v, want %v", name, got, want)
			}
		}
	})
}

func TestSessionExistsError(t *testing.T) {
	t.Run("it matches ErrSessionExists and carries the name as held", func(t *testing.T) {
		var err error = &backend.SessionExistsError{Name: "FOO", Held: "foo"}

		if !errors.Is(err, backend.ErrSessionExists) {
			t.Errorf("errors.Is(%v, ErrSessionExists) = false, want true", err)
		}
		got, ok := errors.AsType[*backend.SessionExistsError](fmt.Errorf("open: %w", err))
		if !ok || got.Held != "foo" {
			t.Errorf("AsType of the wrapped refusal = %+v, %v; want Held foo", got, ok)
		}
	})
}

func TestClientEnvMatches(t *testing.T) {
	leader := backend.ClientEnv{
		{Name: "SSH_CONNECTION", Unset: true},
		{Name: "DISPLAY", Value: ":0"},
	}
	for _, tt := range []struct {
		name    string
		environ []string
		want    bool
	}{
		{"every variable as the leader has it", []string{"HOME=/h", "DISPLAY=:0"}, true},
		{"the last of a variable's entries", []string{"DISPLAY=:1", "DISPLAY=:0"}, true},
		{"a variable the leader lacks, set", []string{"DISPLAY=:0", "SSH_CONNECTION=10.0.0.2 52000 10.0.0.1 22"}, false},
		{"a variable the leader lacks, set empty", []string{"DISPLAY=:0", "SSH_CONNECTION="}, false},
		{"a variable the leader has, with another value", []string{"DISPLAY=:1"}, false},
		{"a variable the leader has, absent", []string{"HOME=/h"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := leader.Matches(tt.environ); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.environ, got, tt.want)
			}
		})
	}

	t.Run("a variable the leader has set empty must be set in the client", func(t *testing.T) {
		leader := backend.ClientEnv{{Name: "DISPLAY", Value: ""}}
		if leader.Matches([]string{"HOME=/h"}) {
			t.Error("a leader with DISPLAY set empty matched a client lacking DISPLAY")
		}
		if !leader.Matches([]string{"DISPLAY="}) {
			t.Error("a leader with DISPLAY set empty did not match a client with it set empty")
		}
	})

	t.Run("a remote leader never matches a local client", func(t *testing.T) {
		remote := backend.ClientEnv{{Name: "SSH_CONNECTION", Value: "10.0.0.2 52000 10.0.0.1 22"}}
		if remote.Matches([]string{"HOME=/h"}) {
			t.Error("a leader over ssh matched a client with no SSH_CONNECTION")
		}
	})
}
