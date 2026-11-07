package zmx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/backend"
)

func TestFailure(t *testing.T) {
	cause := errors.New("caller gave up")
	ended, cancel := context.WithCancelCause(t.Context())
	cancel(cause)

	t.Run("it reports the context's cause for I/O its end failed", func(t *testing.T) {
		for _, err := range []error{
			&net.OpError{Op: "read", Net: "unix", Err: os.ErrDeadlineExceeded},
			&net.OpError{Op: "dial", Net: "unix", Err: context.Canceled},
			&net.OpError{Op: "dial", Net: "unix", Err: context.DeadlineExceeded},
		} {
			if got := failure(ended, err); !errors.Is(got, cause) {
				t.Errorf("failure(%v) = %v, want the cause", err, got)
			}
		}
	})

	t.Run("it keeps the error of I/O that failed on its own, though the context has ended since", func(t *testing.T) {
		for _, err := range []error{io.EOF, io.ErrUnexpectedEOF, errUnsupported, errInfoSize} {
			if got := failure(ended, err); !errors.Is(got, err) || errors.Is(got, cause) {
				t.Errorf("failure(%v) = %v, want it kept", err, got)
			}
		}
	})
}

func TestUnanswered(t *testing.T) {
	t.Run("it counts a probe cut short only when it failed with the caller's cause", func(t *testing.T) {
		caller, cancel := context.WithCancelCause(t.Context())
		cancel(errors.New("caller gave up"))

		if !unanswered(caller, "s", context.Cause(caller)).cut {
			t.Error("a probe failing with the caller's cause is not cut short")
		}
		for _, err := range []error{io.EOF, fmt.Errorf("no answer in 1s: %w", backend.ErrTimeout)} {
			if p := unanswered(caller, "s", err); p.cut || p.session != (backend.Session{Name: "s"}) {
				t.Errorf("unanswered(%v) = %+v, want s with only its name, not cut short", err, p)
			}
		}
	})
}

func TestCallerEnded(t *testing.T) {
	t.Run("it tells the caller's own cause from a ceiling's that wraps it", func(t *testing.T) {
		caller, cancel := context.WithCancelCause(t.Context())
		cancel(backend.ErrTimeout)
		ceiling := fmt.Errorf("no answer in 1s: %w", backend.ErrTimeout)

		if !callerEnded(caller, context.Cause(caller)) {
			t.Error("callerEnded of the caller's cause = false, want true")
		}
		if callerEnded(caller, ceiling) {
			t.Error("callerEnded of a ceiling's cause wrapping the caller's = true, want false")
		}
	})

	t.Run("it is false while the caller's context runs", func(t *testing.T) {
		if callerEnded(t.Context(), errors.New("boom")) {
			t.Error("callerEnded = true with the caller's context running")
		}
	})
}
