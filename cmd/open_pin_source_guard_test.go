package cmd

// Single-source drift guard for the open domain-pin set. The pin flag names are
// declared ONCE in openDomainPinFlags (cmd/open.go) and consumed by BOTH the
// exclusivity guard (anyOpenDomainPin, cmd/root.go) and the RunE dispatch loop
// (which iterates openDomainPinFlags for precedence order and looks each flag's
// resolver up in pinResolvers). These guards fail loudly if the name list, the
// resolver map, and the live openCmd flag set ever drift out of lockstep — so a
// future pin cannot be added to the dispatch table yet silently omitted from the
// exclusivity guard (the drift these tests exist to prevent).
//
// No package-level state mutation, no cobra Execute, no tmux — but package cmd,
// so per CLAUDE.md it MUST NOT use t.Parallel.

import (
	"slices"
	"testing"
)

// TestPinResolversKeysCoveredByFlagList is the core drift guard: every pinResolvers
// (dispatch) key must be present in the shared openDomainPinFlags name list. A
// resolver keyed by a flag absent from the list would be dead (the dispatch loop
// only iterates the list) AND would escape the exclusivity guard, which iterates
// the same list.
func TestPinResolversKeysCoveredByFlagList(t *testing.T) {
	for flag := range pinResolvers {
		if !slices.Contains(openDomainPinFlags, flag) {
			t.Errorf("pinResolvers has a resolver for %q but openDomainPinFlags omits it — anyOpenDomainPin iterates openDomainPinFlags and would miss this pin; add %q to openDomainPinFlags", flag, flag)
		}
	}
}

// TestFlagListFullyResolved is the reverse coverage: every openDomainPinFlags
// entry must have a resolver in pinResolvers, so the dispatch loop never looks up
// a missing key and calls a nil resolver.
func TestFlagListFullyResolved(t *testing.T) {
	for _, flag := range openDomainPinFlags {
		if _, ok := pinResolvers[flag]; !ok {
			t.Errorf("openDomainPinFlags lists %q but pinResolvers has no resolver for it — dispatching this pin would call a nil resolver; add it to pinResolvers", flag)
		}
	}
}

// TestOpenDomainPinFlagsAreRegistered asserts every canonical pin name is a real
// flag on the live openCmd. A typo in openDomainPinFlags would make
// cmd.Flags().Changed(<typo>) always false, silently disabling the exclusivity
// guard for that pin — this fails loudly instead.
func TestOpenDomainPinFlagsAreRegistered(t *testing.T) {
	for _, flag := range openDomainPinFlags {
		if openCmd.Flags().Lookup(flag) == nil {
			t.Errorf("openDomainPinFlags lists %q but openCmd registers no such flag — cmd.Flags().Changed(%q) is always false, silently disabling the exclusivity guard for this pin", flag, flag)
		}
	}
}

// TestAnyOpenDomainPinCoversEveryPin drives the guard with each declared pin flag
// marked Changed and asserts anyOpenDomainPin fires — proving the guard (which
// iterates openDomainPinFlags) covers every pin the dispatch loop can dispatch.
// openProbeCmdWithFlags (concurrent_bootstrap_gate_test.go) carries the same
// -e/-f/-s/-p/-z/-a surface production registers.
func TestAnyOpenDomainPinCoversEveryPin(t *testing.T) {
	for _, flag := range openDomainPinFlags {
		c := openProbeCmdWithFlags()
		if err := c.Flags().Set(flag, "x"); err != nil {
			t.Fatalf("set --%s: %v", flag, err)
		}
		if !anyOpenDomainPin(c) {
			t.Errorf("anyOpenDomainPin = false with --%s set, want true — the exclusivity guard must cover every declared pin", flag)
		}
	}
}
