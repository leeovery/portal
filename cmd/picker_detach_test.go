package cmd

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/runnertoken"
)

// detachHandler is a registered runner's Handler counting the detaches to
// the picker it takes, and refusing every request when refuse is set.
type detachHandler struct {
	mu       sync.Mutex
	refuse   error
	detaches int
}

func (h *detachHandler) Switch(context.Context, string) error         { return errors.ErrUnsupported }
func (h *detachHandler) ConfirmSession(context.Context, string) error { return errors.ErrUnsupported }
func (h *detachHandler) Take(context.Context, string, string) error   { return errors.ErrUnsupported }

func (h *detachHandler) DetachToPicker(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.refuse != nil {
		return h.refuse
	}
	h.detaches++
	return nil
}

func (h *detachHandler) taken() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.detaches
}

// pickerDetachWorld is a state directory of the test's own, the registry in
// it, and a backend holding session with a client attached.
func pickerDetachWorld(t *testing.T, session string) (runnerreg.Registry, *backendtest.Fake) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "pd-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("PORTAL_STATE_DIR", dir)
	return runnerreg.New(dir), backendtest.New(backend.Session{Name: session, Clients: 1})
}

func registerDetachHandler(t *testing.T, r runnerreg.Registry, token, session string, h *detachHandler) {
	t.Helper()
	g, err := r.Register(token, session, h)
	if err != nil {
		t.Fatalf("Register %s on %s: %v", token, session, err)
	}
	t.Cleanup(func() { _ = g.Close() })
}

func TestSendClientsToPicker(t *testing.T) {
	const session = "api-x7Kd9a"

	t.Run("with no runner registered on the session, every client is detached through the backend", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		elsewhere := &detachHandler{}
		registerDetachHandler(t, r, "other1", "blog-c3d4", elsewhere)

		if err := sendClientsToPicker(t.Context(), stateRunners{}, b, session); err != nil {
			t.Fatalf("sendClientsToPicker: %v", err)
		}

		if calls := b.CallsTo(backendtest.OpDetach); len(calls) != 1 || calls[0].Name != session {
			t.Errorf("backend detaches = %+v, want one of %s", calls, session)
		}
		if n := elsewhere.taken(); n != 0 {
			t.Errorf("a runner on another session detached %d times", n)
		}
	})

	t.Run("the backend's failure to detach is returned", func(t *testing.T) {
		_, b := pickerDetachWorld(t, session)
		b.Fail(backendtest.OpDetach, backend.ErrTimeout)

		if err := sendClientsToPicker(t.Context(), stateRunners{}, b, session); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("sendClientsToPicker = %v, want the backend's timeout", err)
		}
	})

	t.Run("a runner accepting is done, with nothing detached through the backend", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		accepting, refusing := &detachHandler{}, &detachHandler{refuse: errors.New("the runner is not attached")}
		registerDetachHandler(t, r, "accept", session, accepting)
		registerDetachHandler(t, r, "refuse", session, refusing)

		if err := sendClientsToPicker(t.Context(), stateRunners{}, b, session); err != nil {
			t.Fatalf("sendClientsToPicker: %v", err)
		}

		if n := accepting.taken(); n != 1 {
			t.Errorf("the accepting runner detached %d times, want once", n)
		}
		if calls := b.CallsTo(backendtest.OpDetach); len(calls) != 0 {
			t.Errorf("backend detaches = %+v, want none beside a runner's acceptance", calls)
		}
	})

	t.Run("runners registered and none accepting is why, with nothing detached through the backend", func(t *testing.T) {
		r, b := pickerDetachWorld(t, session)
		registerDetachHandler(t, r, "first1", session, &detachHandler{refuse: errors.New("the runner is not attached")})
		registerDetachHandler(t, r, "second", session, &detachHandler{refuse: errors.New("the attach is ending")})

		err := sendClientsToPicker(t.Context(), stateRunners{}, b, session)

		if !errors.Is(err, runnertoken.ErrRefused) {
			t.Fatalf("sendClientsToPicker = %v, want the runners' refusals", err)
		}
		for _, why := range []string{"the runner is not attached", "the attach is ending"} {
			if !strings.Contains(err.Error(), why) {
				t.Errorf("sendClientsToPicker = %q, want it to say %q", err, why)
			}
		}
		if calls := b.CallsTo(backendtest.OpDetach); len(calls) != 0 {
			t.Errorf("backend detaches = %+v, want none: it would send the runners' terminals to the shell", calls)
		}
	})
}
