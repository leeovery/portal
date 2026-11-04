package cmd

import (
	"context"
	"time"
)

// resumeBackBound bounds Escape's whole ask, every runner on the session or
// else the backend's detach, so a runner or a daemon that never answers
// leaves the panel answering keys again.
const resumeBackBound = 3 * time.Second

// backToPicker sends the terminals showing session to the picker: each runner
// registered on it is asked to, and with none registered every client is
// detached through the backend, a raw zmx client going to its shell.
func backToPicker(session string) error {
	ctx, cancel := context.WithTimeout(context.Background(), resumeBackBound)
	defer cancel()
	return sendClientsToPicker(ctx, stateRunners{}, backendDetach{}, session)
}

// backendDetach is the backend's detach as Escape reaches it: bound only once
// no runner is registered, so a backend that won't bind costs a runner's
// terminal nothing, and its failure marked so the panel words it as a detach.
type backendDetach struct{}

func (backendDetach) Detach(ctx context.Context, name string) error {
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
