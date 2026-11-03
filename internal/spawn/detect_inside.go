package spawn

import (
	"context"
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/runnerreg"
)

// errUncertain is detection inside a session that can't tell which terminal
// leads it, and so opens nothing: spawning on a guess could put windows on
// the wrong machine.
var errUncertain = errors.New("no one terminal leads the session")

func uncertain(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errUncertain}, args...)...)
}

// leaderPID is the process to walk from to the terminal the user types into
// session in, and the route that found it. zmx names the session's leader,
// the client that last sent it keyboard input. A leader carrying a runner's
// token is that runner's terminal, once the runner confirms it shows session
// and has seen no switch. A leader carrying none is a raw zmx client, found in
// the process table when it is the session's only one. Anything less certain
// finds none.
func (d *Detector) leaderPID(ctx context.Context, session string) (int, string, error) {
	env, err := d.sessions.LeaderEnv(ctx, session)
	if err != nil {
		return 0, routeInside, transient("find the session's leader", err)
	}
	if len(env) == 0 {
		return 0, routeInside, uncertain("no client leads session %q", session)
	}
	if token, ok := env.Lookup(runnerreg.TokenVar); ok {
		pid, err := d.runnerPID(ctx, token, session)
		return pid, routeInsideRunner, err
	}
	pid, err := d.rawClientPID(session)
	return pid, routeInsideRaw, err
}

// runnerPID is the pid of the runner registered under token. A runner the
// registry doesn't hold, one on another session, and one that refuses, being
// unsure or between attaches, are uncertain; one that can't be asked is
// transient.
func (d *Detector) runnerPID(ctx context.Context, token, session string) (int, error) {
	pid, err := d.runners.ConfirmSession(ctx, token, session)
	switch {
	case errors.Is(err, runnerreg.ErrNoRunner), errors.Is(err, runnerreg.ErrRefused):
		return 0, uncertain("the leading runner: %w", err)
	case err != nil:
		return 0, transient(fmt.Sprintf("ask the runner leading session %q", session), err)
	}
	return pid, nil
}

// rawClientPID is the pid of session's one zmx attach process.
func (d *Detector) rawClientPID(session string) (int, error) {
	pids, err := d.clients.AttachClients(session)
	if err != nil {
		return 0, transient(fmt.Sprintf("find the clients of session %q", session), err)
	}
	if len(pids) != 1 {
		return 0, uncertain("%d zmx attach processes on session %q, want 1", len(pids), session)
	}
	return pids[0], nil
}
