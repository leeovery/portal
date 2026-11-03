package cmd

import (
	"context"

	"github.com/leeovery/portal/internal/runner"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/state"
)

// runnerRegistry is the registry of attach runners as opening a session uses
// it.
type runnerRegistry interface {
	// Listing keeps one attach loop's runner listed while it is attached,
	// marking each attach's client through t.
	Listing(t runnerreg.Tracker) runnerListing
	// Switch asks the runner registered under token, whose terminal leads
	// session current, to switch to target, returning nil once it has
	// accepted.
	Switch(ctx context.Context, token, current, target string) error
}

// runnerListing is runnerreg.Listing's surface.
type runnerListing interface {
	Attach(cfg *runner.Config)
	Ended()
	Close()
}

// stateRunners is the registry under Portal's state directory.
type stateRunners struct{}

func (stateRunners) Listing(t runnerreg.Tracker) runnerListing {
	dir, err := state.Dir()
	if err != nil {
		attachLogger.Warn("register failed", "error", err)
		return unlisted{}
	}
	return runnerreg.New(dir).Listing(t)
}

func (stateRunners) Switch(ctx context.Context, token, current, target string) error {
	dir, err := state.Dir()
	if err != nil {
		return err
	}
	return runnerreg.New(dir).Switch(ctx, token, current, target)
}

// unlisted lists nothing, for a state directory that won't resolve: the
// runner attaches unlisted, as one the registry can't hold does.
type unlisted struct{}

func (unlisted) Attach(*runner.Config) {}
func (unlisted) Ended()                {}
func (unlisted) Close()                {}
