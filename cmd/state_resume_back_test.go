package cmd

import (
	"context"
	"errors"
	"fmt"
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

// wedgedBackend's detach gets no answer from its daemon: it fails as zmx's
// does once the caller's context ends first, with that context's cause.
type wedgedBackend struct{ *backendtest.Fake }

func (wedgedBackend) Detach(ctx context.Context, name string) error {
	<-ctx.Done()
	return fmt.Errorf("detach session %q: %w", name, context.Cause(ctx))
}

// wedgedRunner takes a request and never answers it.
type wedgedRunner struct {
	detachHandler
	release chan struct{}
}

func (h *wedgedRunner) DetachToPicker(context.Context) error {
	<-h.release
	return nil
}

// backRow is the panel's row for an Escape on session that sent no terminal
// anywhere.
func backRow(session string) string {
	_, err := backToPicker(session)
	return resumeBackRefusal(err)
}

func TestBackToPicker(t *testing.T) {
	const session = "proj-a1b2"

	t.Run("a runner accepting is done, with nothing detached through the backend", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		accepting := &detachHandler{}
		registerDetachHandler(t, r, "accept", session, accepting)

		route, err := backToPicker(session)
		if err != nil {
			t.Fatalf("backToPicker: %v", err)
		}

		if route != backToRunners {
			t.Errorf("route = %v, want the runners", route)
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

		_, err := backToPicker(session)

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

		route, err := backToPicker(session)
		if err != nil {
			t.Fatalf("backToPicker: %v", err)
		}

		if route != backToShells {
			t.Errorf("route = %v, want the clients detached to their shells", route)
		}
		if calls := b.CallsTo(backendtest.OpDetach); len(calls) != 1 || calls[0].Name != session {
			t.Errorf("backend detaches = %+v, want one of %s", calls, session)
		}
	})

	t.Run("the backend's failure to detach is marked as a detach's", func(t *testing.T) {
		_, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		b.Fail(backendtest.OpDetach, backend.ErrTimeout)

		_, err := backToPicker(session)

		if _, ok := errors.AsType[detachError](err); !ok || !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("backToPicker = %v, want the backend's timeout marked as a failed detach", err)
		}
	})

	t.Run("the ask is bound to about 3s", func(t *testing.T) {
		_, fake := pickerDetachWorld(t, session)
		b := &deadlineBackend{Fake: fake}
		withBackendDeps(t, BackendDeps{Backend: b})

		if _, err := backToPicker(session); err != nil {
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

func TestBackToPicker_Unanswered(t *testing.T) {
	const session = "proj-a1b2"

	t.Run("a backend detach the bound cuts is named in Portal's words, as one that may still land", func(t *testing.T) {
		_, fake := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: wedgedBackend{Fake: fake}})

		_, err := backToPickerWithin(session, 50*time.Millisecond)

		if got, want := resumeBackRefusal(err), "can't detach: no answer in 50ms; it may still land"; got != want {
			t.Errorf("row = %q, want %q", got, want)
		}
	})

	t.Run("Escape's own bound names itself in the same words", func(t *testing.T) {
		if got, want := (backTimeout{bound: resumeBackBound}).Error(), "no answer in 3s; it may still land"; got != want {
			t.Errorf("bound's end = %q, want %q", got, want)
		}
	})

	t.Run("a runner that never answers is named as one", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		withBackendDeps(t, BackendDeps{Backend: b})
		wedged := &wedgedRunner{release: make(chan struct{})}
		g, err := r.Register("wedged", session, wedged)
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		// Released before the registration closes, which waits for the request in flight.
		t.Cleanup(func() { _ = g.Close() })
		t.Cleanup(func() { close(wedged.release) })

		_, err = backToPickerWithin(session, 700*time.Millisecond)

		if got, want := resumeBackRefusal(err), "can't open the picker: a runner did not answer"; got != want {
			t.Errorf("row = %q, want %q (from %v)", got, want, err)
		}
		if calls := b.CallsTo(backendtest.OpDetach); len(calls) != 0 {
			t.Errorf("backend detaches = %+v, want none beside a registered runner", calls)
		}
	})
}
