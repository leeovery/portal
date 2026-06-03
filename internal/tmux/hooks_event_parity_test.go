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

// TestPortalTeardownFingerprintParity is the fingerprint-drift guard, the
// FINGERPRINT analogue of TestPortalManagedEventSetParity (which guards the
// EVENT set). It proves the teardown eviction predicate
// (portalCommandSubstrings) is DERIVED from — and never narrower than — the
// union of every managedEvents entry's fingerprints PLUS the legacy
// migrate-rename substring teardown explicitly retains.
//
// The defect this closes: registration converges `session-closed` onto
// commitNowCommand, whose only fingerprint is `portal state commit-now`, but
// the hand-authored teardown set omitted that substring — so
// UnregisterPortalHooks classified the converged commit-now entry as non-Portal
// and left it installed (AC #5 violation). Deriving the teardown set from the
// managedEvents union closes the seam: any category added to managedEvents
// auto-widens teardown coverage, and this test fails if a future edit
// reintroduces an independent literal that drops a registered fingerprint.
//
// Both intentional asymmetries are asserted:
//   - Every managedEvents fingerprint (including commitNowSubstring) MUST be a
//     member of the teardown set.
//   - The legacy migrate-rename substring MUST be a member of the teardown set
//     even though it appears in NO managedEvents entry (registration never
//     installs/converges it; teardown retains it for old-binary cleanup).
func TestPortalTeardownFingerprintParity(t *testing.T) {
	teardown := tmux.PortalTeardownFingerprints()
	teardownSet := make(map[string]bool, len(teardown))
	for _, fp := range teardown {
		teardownSet[fp] = true
	}

	// Every managedEvents fingerprint must be reapable by teardown.
	for _, fp := range tmux.ManagedEventFingerprintUnion() {
		if !teardownSet[fp] {
			t.Errorf("teardown fingerprint set %v is missing managedEvents fingerprint %q — "+
				"a registered category is unreachable by UnregisterPortalHooks (AC #5 seam)",
				teardown, fp)
		}
	}

	// commit-now specifically — the fingerprint the original literal omitted.
	if !teardownSet[commitNowFingerprint] {
		t.Errorf("teardown fingerprint set %v is missing %q — the converged session-closed "+
			"commit-now hook would survive UnregisterPortalHooks", teardown, commitNowFingerprint)
	}

	// The legacy migrate-rename substring is explicitly retained by teardown
	// even though registration never installs it (asymmetry preserved).
	if !teardownSet[tmux.MigrateRenameSubstring] {
		t.Errorf("teardown fingerprint set %v is missing the explicitly-retained legacy substring %q — "+
			"stale migrate-rename entries from old binaries would survive teardown",
			teardown, tmux.MigrateRenameSubstring)
	}

	// Asymmetry guard: registration must NOT carry migrate-rename in any
	// managedEvents fingerprint set (it is teardown-only).
	for _, fp := range tmux.ManagedEventFingerprintUnion() {
		if fp == tmux.MigrateRenameSubstring {
			t.Errorf("managedEvents fingerprint union %v contains %q — registration must never "+
				"install/converge migrate-rename (it is teardown-retained only)",
				tmux.ManagedEventFingerprintUnion(), tmux.MigrateRenameSubstring)
		}
	}
}
