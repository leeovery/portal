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

// TestSplitTriggerFirst pins the FIRST-trigger split the multi-target open burst
// uses (runOpenBurstWithDeps in cmd/open_burst_run.go): for an ordered surface
// selection the LEADING row is the trigger the invoking terminal self-connects to
// and the trailing N-1 rows are the externally-spawned windows. It is deliberately
// distinct from the trailing-trigger SplitNetN (the legacy spawn CLI / picker).
func TestSplitTriggerFirst(t *testing.T) {
	tests := []struct {
		name         string
		ordered      []Surface
		wantTrigger  Surface
		wantExternal []Surface
	}{
		{
			name:         "two elements",
			ordered:      []Surface{{Kind: SurfaceAttach, Value: "a"}, {Kind: SurfaceMint, Value: "/b"}},
			wantTrigger:  Surface{Kind: SurfaceAttach, Value: "a"},
			wantExternal: []Surface{{Kind: SurfaceMint, Value: "/b"}},
		},
		{
			name:         "three elements",
			ordered:      []Surface{{Kind: SurfaceAttach, Value: "a"}, {Kind: SurfaceAttach, Value: "b"}, {Kind: SurfaceAttach, Value: "c"}},
			wantTrigger:  Surface{Kind: SurfaceAttach, Value: "a"},
			wantExternal: []Surface{{Kind: SurfaceAttach, Value: "b"}, {Kind: SurfaceAttach, Value: "c"}},
		},
		{
			name:         "single element",
			ordered:      []Surface{{Kind: SurfaceMint, Value: "/x"}},
			wantTrigger:  Surface{Kind: SurfaceMint, Value: "/x"},
			wantExternal: []Surface{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTrigger, gotExternal := SplitTriggerFirst(tt.ordered)
			if gotTrigger != tt.wantTrigger {
				t.Errorf("trigger = %+v, want %+v", gotTrigger, tt.wantTrigger)
			}
			if !slices.Equal(gotExternal, tt.wantExternal) {
				t.Errorf("external = %#v, want %#v", gotExternal, tt.wantExternal)
			}
		})
	}
}

// TestSplitTriggerFirst_DistinctFromSplitNetN proves the two splits pick OPPOSITE
// ends of the same ordered slice: SplitTriggerFirst (open burst) takes the FIRST
// element as trigger, while the legacy SplitNetN (spawn CLI / picker) takes the
// LAST. The externals are the mirror-image complements.
func TestSplitTriggerFirst_DistinctFromSplitNetN(t *testing.T) {
	names := []string{"a", "b", "c"}
	surfaces := AttachSurfaces(names)

	trigger, external := SplitTriggerFirst(surfaces)
	netExternal, netTrigger := SplitNetN(names)

	if trigger.Value != "a" {
		t.Errorf("SplitTriggerFirst trigger = %q, want %q (first)", trigger.Value, "a")
	}
	if netTrigger != "c" {
		t.Errorf("SplitNetN trigger = %q, want %q (last)", netTrigger, "c")
	}
	if trigger.Value == netTrigger {
		t.Errorf("the two splits must pick DIFFERENT triggers on the same slice: both got %q", trigger.Value)
	}

	// SplitTriggerFirst externals are the trailing rows; SplitNetN externals are
	// the leading rows — mirror-image complements.
	wantFirstExternal := []Surface{{Kind: SurfaceAttach, Value: "b"}, {Kind: SurfaceAttach, Value: "c"}}
	if !slices.Equal(external, wantFirstExternal) {
		t.Errorf("SplitTriggerFirst external = %#v, want %#v", external, wantFirstExternal)
	}
	if !slices.Equal(netExternal, []string{"a", "b"}) {
		t.Errorf("SplitNetN external = %#v, want %#v", netExternal, []string{"a", "b"})
	}
}
