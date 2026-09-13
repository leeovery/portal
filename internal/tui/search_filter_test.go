package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/leeovery/portal/internal/tmux"
)

func TestContainmentFilterFallsBackWhenTheSourceIsOutOfStep(t *testing.T) {
	src := &searchItemSource{}
	// The source holds a non-matching item, so a containment lookup against it
	// would answer with nothing while the picker's own rule answers with a row.
	src.set([]list.Item{SessionItem{Session: tmux.Session{Name: "pro-tools"}}})
	targets := []string{"app-port", "pro-tools"}

	got := containmentFilter("port", src)("port", targets)

	want := list.DefaultFilter("port", targets)
	if len(want) == 0 {
		t.Fatalf("test setup invariant: expected the picker's own rule to match a target")
	}
	if len(got) != len(want) {
		t.Fatalf("ranks = %v, want the picker's own rule's %v", got, want)
	}
	for i := range want {
		if got[i].Index != want[i].Index {
			t.Fatalf("ranks = %v, want the picker's own rule's %v", got, want)
		}
	}
}
