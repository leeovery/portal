package cmd

import (
	"context"
	"fmt"
	"time"
)

// resumeBackBound bounds Escape's whole ask, every runner on the session or
// else the backend's detach, so a runner or a daemon that never answers
// leaves the panel answering keys again.
const resumeBackBound = 3 * time.Second

// backRoute is where Escape sent the session's terminals.
type backRoute int

const (
	// backToRunners is every runner on the session, sent to the picker.
	backToRunners backRoute = iota
	// backToShells is every client, detached through the backend: with no
	// runner to show a picker, a raw zmx client goes to its shell.
	backToShells
)

// backToPicker sends the terminals showing session to the picker: each runner
// registered on it is asked to, and with none registered every client is
// detached through the backend, a raw zmx client going to its shell.
func backToPicker(session string) (backRoute, error) {
	return backToPickerWithin(session, resumeBackBound)
}

// backToPickerWithin is backToPicker bounded by bound. The bound's end is
// named in Portal's words, since an ask cut by it may still land.
func backToPickerWithin(session string, bound time.Duration) (backRoute, error) {
	ctx, cancel := context.WithTimeoutCause(context.Background(), bound, backTimeout{bound: bound})
	defer cancel()
	detach := &backendDetach{}
	err := sendClientsToPicker(ctx, stateRunners{}, detach, session)
	if detach.asked {
		return backToShells, err
	}
	return backToRunners, err
}

// backTimeout is the end of Escape's bound. It wraps nothing, so the panel's
// row carries its words rather than a deadline's.
type backTimeout struct{ bound time.Duration }

func (e backTimeout) Error() string {
	return fmt.Sprintf("no answer in %s; it may still land", e.bound)
}

// backendDetach is the backend's detach as Escape reaches it. The backend is
// bound only once the registry has no runner on the session, so a backend that
// won't bind costs a runner's terminal nothing, and its failure is marked so
// the panel words it as a detach.
type backendDetach struct {
	// asked is set once the backend's detach is reached.
	asked bool
}

func (d *backendDetach) Detach(ctx context.Context, name string) error {
	d.asked = true
	b, err := sessionBackend()
	if err == nil {
		err = b.Detach(ctx, name)
	}
	if err != nil {
		return detachError{err: err}
	}
	return nil
}

// detachError marks a failure of the backend's detach, keeping its own
// message as its text.
type detachError struct{ err error }

func (e detachError) Error() string { return e.err.Error() }
func (e detachError) Unwrap() error { return e.err }
