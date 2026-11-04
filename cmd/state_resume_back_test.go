package cmd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/runnertoken"
)

// deadlineBackend records the deadline each detach was given.
type deadlineBackend struct {
	*backendtest.Fake
	deadlines []time.Duration
}

func (b *deadlineBackend) Detach(ctx context.Context, name string) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		b.deadlines = append(b.deadlines, 0)
	} else {
		b.deadlines = append(b.deadlines, time.Until(deadline))
	}
	return b.Fake.Detach(ctx, name)
}

func TestBackToPicker(t *testing.T) {
	const session = "proj-a1b2"

	t.Run("a runner accepting is done, with nothing detached through the backend", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		accepting := &detachHandler{}
		registerDetachHandler(t, r, "accept", session, accepting)

		if err := backToPicker(session); err != nil {
			t.Fatalf("backToPicker: %v", err)
		}

		if n := accepting.taken(); n != 1 {
			t.Errorf("the runner detached %d times, want once", n)
		}
		if calls := b.CallsTo(backendtest.OpDetach); len(calls) != 0 {
			t.Errorf("backend detaches = %+v, want none beside a runner's acceptance", calls)
		}
	})

	t.Run("runners registered and none accepting is an error, with nothing detached through the backend", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		registerDetachHandler(t, r, "refuse", session, &detachHandler{refuse: errors.New("the runner is not attached")})

		err := backToPicker(session)

		if !errors.Is(err, runnertoken.ErrRefused) {
			t.Fatalf("backToPicker = %v, want the runner's refusal", err)
		}
		if _, ok := errors.AsType[detachError](err); ok {
			t.Errorf("backToPicker = %v, want a refusal, not a failed detach", err)
		}
		if calls := b.CallsTo(backendtest.OpDetach); len(calls) != 0 {
			t.Errorf("backend detaches = %+v, want none: it would send the runner's terminal to the shell", calls)
		}
	})

	t.Run("with no runner registered, the session is detached through the backend", func(t *testing.T) {
		_, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})

		if err := backToPicker(session); err != nil {
			t.Fatalf("backToPicker: %v", err)
		}

		if calls := b.CallsTo(backendtest.OpDetach); len(calls) != 1 || calls[0].Name != session {
			t.Errorf("backend detaches = %+v, want one of %s", calls, session)
		}
	})

	t.Run("the backend's failure to detach is marked as a detach's", func(t *testing.T) {
		_, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		b.Fail(backendtest.OpDetach, backend.ErrTimeout)

		err := backToPicker(session)

		if _, ok := errors.AsType[detachError](err); !ok || !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("backToPicker = %v, want the backend's timeout marked as a failed detach", err)
		}
	})

	t.Run("the ask is bound to about 3s", func(t *testing.T) {
		_, fake := pickerDetachWorld(t, session)
		b := &deadlineBackend{Fake: fake}
		withBackendDeps(t, BackendDeps{Backend: b})

		if err := backToPicker(session); err != nil {
			t.Fatalf("backToPicker: %v", err)
		}

		if len(b.deadlines) != 1 {
			t.Fatalf("detaches = %d, want one", len(b.deadlines))
		}
		if left := b.deadlines[0]; left < 2*time.Second || left > 3*time.Second {
			t.Errorf("detach given %v, want a deadline about 3s away", left)
		}
	})
}
