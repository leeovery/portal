package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
)

var backendDeps *BackendDeps

// BackendDeps overrides the session backend; production leaves backendDeps
// nil and binds zmx afresh on every call.
type BackendDeps struct {
	Backend backend.Backend
}

const zmxProgram = "zmx"

var errZmxMissing = errors.New("zmx not found on PATH; install it with: brew install neurosnap/tap/zmx")

// sessionBackend is the backend for everything but creating and attaching:
// listing, kill, detach, history and the current session reach zmx over its
// sockets, so they work with no zmx binary on PATH.
func sessionBackend() (backend.Backend, error) {
	if backendDeps != nil {
		return backendDeps.Backend, nil
	}
	return bindZmx(zmxProgram)
}

// attachingBackend is the backend for creating and attaching a session, which
// run the zmx binary: one missing from PATH is fatal here and only here.
func attachingBackend() (backend.Backend, error) {
	if backendDeps != nil {
		return backendDeps.Backend, nil
	}
	binary, err := exec.LookPath(zmxProgram)
	if err != nil {
		return nil, errZmxMissing
	}
	return bindZmx(binary)
}

// bindZmx does no I/O, so binding per call costs nothing. zmx.New refuses a
// relative or empty directory, which no command can work around.
func bindZmx(binary string) (backend.Backend, error) {
	b, err := zmx.New(zmx.Config{Binary: binary, Dir: zmx.DirFor(os.Environ())})
	if err != nil {
		return nil, err
	}
	return b, nil
}

// userSessions is every session b lists but Portal's own, the one listing
// every user-facing surface reads.
func userSessions(ctx context.Context, b backend.Backend) ([]backend.Session, error) {
	sessions, err := b.List(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(sessions, backend.Session.PortalOwned), nil
}
