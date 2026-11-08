package backend_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

const heldSession = "proj-a1b2"

// aLeader is the tracked environment a backend answers for a client leading a
// session: a variable it tracks that the client's environment lacks.
var aLeader = backend.ClientEnv{{Name: "SSH_CONNECTION", Unset: true}}

// aTerminal is a client attached on a terminal, as AttachClients shows one.
var aTerminal = []backend.ClientProcess{{PID: 4242, Env: []string{"ZMX_DIR=/tmp/zx-held-test"}}}

func TestHeld(t *testing.T) {
	errRefused := errors.New("connection refused")
	cases := []struct {
		name    string
		stage   func(f *backendtest.Fake)
		want    bool
		wantErr bool
	}{
		{"a client leading it holds it", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession, Clients: 1})
			f.SetLeaderEnv(heldSession, aLeader)
		}, true, false},
		{"a terminal attached to it holds it, with no leader answered", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession, Clients: 1})
			f.SetAttachClients(heldSession, aTerminal)
		}, true, false},
		{"a reader alone holds nothing, though the session counts it", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession, Clients: 1})
		}, false, false},
		{"a terminal's attach the session no longer counts holds nothing", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession})
			f.SetAttachClients(heldSession, aTerminal)
		}, false, false},
		{"no client at all holds nothing", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession})
		}, false, false},
		{"a leader is enough when the process table can't be read", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession, Clients: 1})
			f.SetLeaderEnv(heldSession, aLeader)
			f.Fail(backendtest.OpAttachClients, backend.ErrUnreadableClient)
		}, true, false},
		{"a terminal is enough when the leader can't be read", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession, Clients: 1})
			f.SetAttachClients(heldSession, aTerminal)
			f.Fail(backendtest.OpLeaderEnv, errRefused)
		}, true, false},
		{"an unread leader and no terminal can't be told", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession, Clients: 1})
			f.Fail(backendtest.OpLeaderEnv, errRefused)
		}, false, true},
		{"no leader and an unread process table can't be told", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession, Clients: 1})
			f.Fail(backendtest.OpAttachClients, backend.ErrUnreadableClient)
		}, false, true},
		{"a terminal on a session that doesn't answer can't be told", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: heldSession, Clients: 1})
			f.SetAttachClients(heldSession, aTerminal)
			f.StopAnswering(heldSession)
		}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := backendtest.New()
			tc.stage(fake)

			held, err := backend.Held(t.Context(), fake, heldSession)

			if held != tc.want || (err != nil) != tc.wantErr {
				t.Errorf("Held() = %v, %v; want %v with an error %v", held, err, tc.want, tc.wantErr)
			}
		})
	}
}
