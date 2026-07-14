package spawn

import (
	"slices"
	"testing"
)

// TestSplitNetN pins the net-N (external/trigger) split both callers share: for
// an ordered selection the trailing row is the self-attach trigger and the
// leading N-1 rows are the external windows. A single-element slice yields an
// empty external set and that element as the trigger. These are the exact values
// runSpawn (cmd/spawn.go) and dispatchBurst (internal/tui) now inherit, so a
// single edit here keeps the two paths from drifting.
func TestSplitNetN(t *testing.T) {
	tests := []struct {
		name         string
		ordered      []string
		wantExternal []string
		wantTrigger  string
	}{
		{
			name:         "two elements",
			ordered:      []string{"a", "b"},
			wantExternal: []string{"a"},
			wantTrigger:  "b",
		},
		{
			name:         "three elements",
			ordered:      []string{"a", "b", "c"},
			wantExternal: []string{"a", "b"},
			wantTrigger:  "c",
		},
		{
			name:         "single element",
			ordered:      []string{"a"},
			wantExternal: nil,
			wantTrigger:  "a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExternal, gotTrigger := SplitNetN(tt.ordered)
			if !slices.Equal(gotExternal, tt.wantExternal) {
				t.Errorf("external = %#v, want %#v", gotExternal, tt.wantExternal)
			}
			if gotTrigger != tt.wantTrigger {
				t.Errorf("trigger = %q, want %q", gotTrigger, tt.wantTrigger)
			}
		})
	}
}
