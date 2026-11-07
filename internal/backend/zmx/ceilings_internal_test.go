package zmx

import (
	"testing"
	"time"

	"github.com/leeovery/portal/internal/saverhost"
)

// saver.lock's wait, and every lock's whose bound reads saverhost's
// ceilings, covers another holder's run with the backend's calls at these
// ceilings; a longer one would have a waiter give up on a holder still within
// its bounds.
func TestCeilingsWithinWhatTheLocksAllow(t *testing.T) {
	for call, c := range map[string]struct{ ceiling, allowed time.Duration }{
		"a read":        {defaultCeilings.probe, saverhost.ReadCeiling},
		"a create":      {defaultCeilings.create, saverhost.CreateCeiling},
		"a kill":        {defaultCeilings.kill, saverhost.KillCeiling},
		"a detach":      {defaultCeilings.detach, saverhost.DetachCeiling},
		"a run's grace": {pipeGrace, saverhost.RunGrace},
	} {
		if c.ceiling > c.allowed {
			t.Errorf("%s's ceiling = %s, want within the %s the locks allow it", call, c.ceiling, c.allowed)
		}
	}
}
