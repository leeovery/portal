package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"syscall"

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

var errZmxMissing = errors.New("zmx not found on PATH; Portal needs a zmx build newer than 0.8.1")

// sessionBackend is the backend for everything but creating and attaching:
// listing, kill, detach, history and the current session reach zmx over its
// sockets, so they work with no zmx binary on PATH.
func sessionBackend() (backend.Backend, error) {
	if backendDeps != nil {
		return backendDeps.Backend, nil
	}
	return bindZmx(zmxProgram)
}

// currentName is the session this process runs in, empty outside one. It
// binds nothing: zmx names the session in the environment.
func currentName() string {
	if backendDeps != nil {
		return backendDeps.Backend.Current()
	}
	return zmx.CurrentFor(os.Environ())
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

// bindZmx binds the zmx directory the environment names.
func bindZmx(binary string) (backend.Backend, error) {
	b, err := bindZmxAt(binary, zmx.DirFor(os.Environ()))
	if err != nil {
		return nil, err
	}
	return b, nil
}

// bindZmxAt does no I/O, so binding per call costs nothing. zmx.New refuses a
// relative or empty directory, which no command can work around.
func bindZmxAt(binary, dir string) (*zmx.Backend, error) {
	return zmx.New(zmxConfig(binary, dir))
}

// zmxConfig binds binary to dir, its Create ending a session an older zmx
// made through signalProcess, should that zmx's kill fail.
func zmxConfig(binary, dir string) zmx.Config {
	return zmx.Config{Binary: binary, Dir: dir, Signal: func(pid int, sig syscall.Signal) error { return signalProcess(pid, sig) }}
}

// readZmxAt is bindZmxAt signalling nothing, for doctor, which only reads.
func readZmxAt(binary, dir string) (*zmx.Backend, error) {
	cfg := zmxConfig(binary, dir)
	cfg.Signal = nil
	return zmx.New(cfg)
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

// userSessionNames is the names of b's user sessions: what a target resolves
// against and completion offers, names and never labels.
func userSessionNames(ctx context.Context, b backend.Backend) ([]string, error) {
	sessions, err := userSessions(ctx, b)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.Name)
	}
	return names, nil
}
