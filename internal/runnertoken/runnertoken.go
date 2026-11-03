// Package runnertoken is how Portal tells which attach runner a terminal
// belongs to: the token each runner's zmx client carries in Var, read back
// from the client leading a session, and the answers asking the runner a token
// names can bring. It sits below the runner registry so a caller that only
// reads a leader, as spawn detection does, never links the runner.
package runnertoken

import (
	"context"
	"errors"

	"github.com/leeovery/portal/internal/backend"
)

// Var carries a runner's token in the environment of its zmx client, which
// tracks it, so the backend reports it for the client leading a session.
const Var = "PORTAL_ATTACH"

// ErrNoRunner is a request with no runner to take it: no token, as from a raw
// zmx client or none leading, a token the registry doesn't hold, one naming a
// runner on another session, or a runner proved gone.
var ErrNoRunner = errors.New("no runner to ask")

// ErrRefused is a runner's refusal of a request: one it doesn't understand,
// or can't carry out, as when its attach is already ending.
var ErrRefused = errors.New("refused")

// LeaderReader reads the tracked environment of the client leading a
// session: backend.Backend's LeaderEnv.
type LeaderReader interface {
	LeaderEnv(ctx context.Context, name string) (backend.ClientEnv, error)
}

// Leader is the client leading a session, as the backend reports it.
type Leader struct {
	// Env is its tracked environment, empty when no client leads.
	Env backend.ClientEnv
	// Token is the token of the runner whose client it is, empty for a raw
	// zmx client or none leading.
	Token string
}

// ReadLeader reads the client leading session through b. A backend that
// can't say who leads is an error, never a leader with no token: an unread
// leader is not a raw client. The error is the backend's own, which names the
// session.
func ReadLeader(ctx context.Context, b LeaderReader, session string) (Leader, error) {
	env, err := b.LeaderEnv(ctx, session)
	if err != nil {
		return Leader{}, err
	}
	token, _ := env.Lookup(Var)
	return Leader{Env: env, Token: token}, nil
}
