package hooks_test

import (
	"testing"

	"github.com/leeovery/portal/internal/hooks"
)

// TestViaWireValues pins the string each calling surface logs. The values are
// the operator's grep vocabulary — `grep via=hydrate` over portal.log — so a
// changed value is a silently broken search, not a compile error.
func TestViaWireValues(t *testing.T) {
	cases := []struct {
		name string
		via  hooks.Via
		want string
	}{
		{"cli", hooks.ViaCLI, "cli"},
		{"internal", hooks.ViaInternal, "internal"},
		{"hydrate", hooks.ViaHydrate, "hydrate"},
		{"doctor", hooks.ViaDoctor, "doctor"},
		{"panel", hooks.ViaPanel, "panel"},
		// An unset Via must read as absent. Were the vocabulary numbered from
		// zero, every zero-valued Via would impersonate the first surface.
		{"the unset zero value", hooks.Via(0), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.via.String(); got != tc.want {
				t.Errorf("via = %q, want %q", got, tc.want)
			}
		})
	}
}
