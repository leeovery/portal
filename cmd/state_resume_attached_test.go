package cmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

// A question no terminal saw is never answered, so a draw asks one only where
// the session's zmx daemon counts a client, and where it can't tell, asks.
func TestSessionAttached(t *testing.T) {
	session := samplePayload().Session
	cases := []struct {
		name  string
		stage func(f *backendtest.Fake)
		want  bool
	}{
		{"a session a client is attached to", func(f *backendtest.Fake) { f.Add(backend.Session{Name: session, Clients: 1}) }, true},
		{"a session no client is attached to", func(f *backendtest.Fake) { f.Add(backend.Session{Name: session}) }, false},
		{"a session that doesn't answer", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: session})
			f.StopAnswering(session)
		}, true},
		{"a session the backend can't ask", func(f *backendtest.Fake) {
			f.Add(backend.Session{Name: session})
			f.Fail(backendtest.OpSession, errors.New("connection refused"))
		}, true},
		{"no session under the name", func(*backendtest.Fake) {}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := backendtest.New()
			tc.stage(fake)
			withBackendDeps(t, BackendDeps{Backend: fake})

			if got := sessionAttached(session); got != tc.want {
				t.Errorf("sessionAttached() = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("it asks within its bound", func(t *testing.T) {
		asked := &sessionAskBackend{Backend: backendtest.New(backend.Session{Name: session})}
		withBackendDeps(t, BackendDeps{Backend: asked})

		sessionAttached(session)

		if asked.left <= 0 || asked.left > resumeAttachedBound {
			t.Errorf("the ask had %v left of its deadline, want no more than %v", asked.left, resumeAttachedBound)
		}
	})
}

// sessionAskBackend records how long a Session call had left of its context.
type sessionAskBackend struct {
	backend.Backend
	left time.Duration
}

func (b *sessionAskBackend) Session(ctx context.Context, name string) (backend.Session, error) {
	if deadline, ok := ctx.Deadline(); ok {
		b.left = time.Until(deadline)
	}
	return b.Backend.Session(ctx, name)
}
