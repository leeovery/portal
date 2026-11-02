package zmx

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/backend"
)

func TestNotCreated(t *testing.T) {
	boom := errors.New("boom")

	t.Run("it reports a session joined under the name, as held", func(t *testing.T) {
		for _, runErr := range []error{nil, boom} {
			err := notCreated("FOO", runErr, "foo")

			if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != "foo" || exists.Name != "FOO" {
				t.Errorf("notCreated(FOO, %v, foo) = %v, want a SessionExistsError held as foo", runErr, err)
			}
		}
	})

	t.Run("it reports a clean run that made nothing as a join of a session gone since", func(t *testing.T) {
		err := notCreated("x", nil, "")

		if exists, ok := errors.AsType[*backend.SessionExistsError](err); !ok || exists.Held != "" {
			t.Errorf("notCreated(x, nil, \"\") = %v, want a SessionExistsError with no held name", err)
		}
	})

	t.Run("it reports a failed run that left nothing live as the failure", func(t *testing.T) {
		err := notCreated("x", boom, "")

		if !errors.Is(err, boom) || errors.Is(err, backend.ErrSessionExists) {
			t.Errorf("notCreated(x, boom, \"\") = %v, want boom and not ErrSessionExists", err)
		}
	})
}
