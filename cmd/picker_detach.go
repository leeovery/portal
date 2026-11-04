package cmd

import (
	"context"
	"errors"

	"github.com/leeovery/portal/internal/runnertoken"
)

// pickerRunners is the registry of attach runners as sending a session's
// terminals to the picker uses it.
type pickerRunners interface {
	// DetachToPicker asks every runner registered on session to detach to
	// the picker, returning how many accepted: runnertoken.ErrNoRunner with
	// none there to ask, and why with none accepting.
	DetachToPicker(ctx context.Context, session string) (int, error)
}

// sessionDetacher detaches every client of a session, as backend.Backend's
// Detach does.
type sessionDetacher interface {
	Detach(ctx context.Context, name string) error
}

// sendClientsToPicker sends the terminals showing session to the picker. The
// runners registered on it are asked to detach to the picker; with none
// registered, every client is detached through b, a raw zmx client going to
// its shell. Runners registered but none accepting is why, with nothing
// detached: b's detach would send their terminals to the shell. b detaches
// only every client at once, so a raw client attached beside a runner stays
// attached.
func sendClientsToPicker(ctx context.Context, runners pickerRunners, b sessionDetacher, session string) error {
	_, err := runners.DetachToPicker(ctx, session)
	if errors.Is(err, runnertoken.ErrNoRunner) {
		return b.Detach(ctx, session)
	}
	return err
}
