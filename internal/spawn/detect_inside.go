package spawn

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/runnertoken"
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
// token is that runner's terminal, once the runner confirms it shows session.
// A leader carrying none is a raw zmx client, found in the process table when
// it is the only one that can be. Anything less certain finds none.
func (d *Detector) leaderPID(ctx context.Context, session string) (int, string, error) {
	leader, err := runnertoken.ReadLeader(ctx, d.sessions, session)
	if err != nil {
		return 0, routeInside, transient("find the session's leader", err)
	}
	if len(leader.Env) == 0 {
		return 0, routeInside, uncertain("no client leads session %q", session)
	}
	if leader.Token != "" {
		pid, err := d.runnerPID(ctx, leader.Token, session)
		return pid, routeInsideRunner, err
	}
	pid, err := d.rawClientPID(session, leader.Env)
	return pid, routeInsideRaw, err
}

// runnerPID is the pid of the runner registered under token. A runner the
// registry doesn't hold, one on another session, and one that refuses, as
// one between attaches or unsure and unconfirmed does, are uncertain; one that can't be asked is
// transient.
func (d *Detector) runnerPID(ctx context.Context, token, session string) (int, error) {
	pid, err := d.runners.ConfirmSession(ctx, token, session)
	switch {
	case errors.Is(err, runnertoken.ErrNoRunner), errors.Is(err, runnertoken.ErrRefused):
		return 0, uncertain("the leading runner: %w", err)
	case err != nil:
		return 0, transient(fmt.Sprintf("ask the runner leading session %q", session), err)
	}
	return pid, nil
}

// rawClientPID is the pid of the one zmx attach process on session that can
// be its leader: one carrying each variable zmx tracks for the leader as the
// leader has it, so a client over ssh never stands in for a local one, and no
// runner's token, which only a runner's own client carries. The process table
// shows a client zmx switched in-process under the session it attached, so a
// leader that switched here while another raw client still names the session
// is found only when their tracked variables differ. A process that can't be
// read might be the leader, so it finds none.
func (d *Detector) rawClientPID(session string, leader backend.ClientEnv) (int, error) {
	clients, err := d.clients.AttachClients(session)
	switch {
	case errors.Is(err, backend.ErrUnreadableClient):
		return 0, uncertain("%w", err)
	case err != nil:
		return 0, transient(fmt.Sprintf("find the clients of session %q", session), err)
	}
	var pids []int
	for _, c := range clients {
		if !carries(c.Env, runnertoken.Var) && leader.Matches(c.Env) {
			pids = append(pids, c.PID)
		}
	}
	if len(pids) != 1 {
		return 0, uncertain("%d of %d zmx attach processes on session %q can be its leader, want 1", len(pids), len(clients), session)
	}
	return pids[0], nil
}

// carries reports whether environ sets name at all.
func carries(environ []string, name string) bool {
	return slices.ContainsFunc(environ, func(kv string) bool { return strings.HasPrefix(kv, name+"=") })
}
