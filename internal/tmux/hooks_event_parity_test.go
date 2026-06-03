package tmux_test

import (
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

// TestPortalManagedEventSetParity proves managedEvents is the single source of
// truth for the Portal-managed event-set: the projected `event` field of every
// managedEvents entry must equal, in identical order, the portalEvents
// teardown enumeration.
//
// In the derivation variant (portalEvents built by projecting managedEvents)
// this is a tautology-guard against accidental future re-divergence — if a
// later edit reintroduces an independent teardown list, this fails rather than
// silently leaving a registered-but-never-reaped (or torn-down-but-never-
// converged) event for a user's hook table to discover.
//
// Order is asserted, not just set-membership: hooks_unregister_test.go's
// cross-event removal-order assertion ("save events first, then hydration")
// follows portalEvents order, so the registration→teardown order contract is
// part of the parity.
func TestPortalManagedEventSetParity(t *testing.T) {
	registration := tmux.ManagedEventNames()
	teardown := tmux.PortalTeardownEvents()

	if len(registration) != len(teardown) {
		t.Fatalf("managed-event-set size mismatch: registration has %d events %v, teardown has %d events %v",
			len(registration), registration, len(teardown), teardown)
	}

	for i := range registration {
		if registration[i] != teardown[i] {
			t.Errorf("managed-event-set divergence at index %d: registration=%q teardown=%q\nregistration=%v\nteardown=%v",
				i, registration[i], teardown[i], registration, teardown)
		}
	}
}
