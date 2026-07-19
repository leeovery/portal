package state_test

import (
	"testing"

	"github.com/leeovery/portal/internal/state"
)

func TestCountPanes_SumsPanesAcrossSessionsAndWindows(t *testing.T) {
	idx := state.Index{
		Sessions: []state.Session{
			{
				Name: "work",
				Windows: []state.Window{
					{Index: 0, Panes: []state.Pane{{Index: 0}, {Index: 1}}},
					{Index: 1, Panes: []state.Pane{{Index: 0}}},
				},
			},
			{
				Name: "play",
				Windows: []state.Window{
					{Index: 0, Panes: []state.Pane{{Index: 0}, {Index: 1}, {Index: 2}}},
				},
			},
		},
	}
	// 2 + 1 + 3 = 6 panes total.
	if got := state.CountPanes(idx); got != 6 {
		t.Errorf("CountPanes = %d; want 6", got)
	}
}

func TestCountPanes_ZeroForEmptyIndex(t *testing.T) {
	if got := state.CountPanes(state.Index{}); got != 0 {
		t.Errorf("CountPanes(empty) = %d; want 0", got)
	}
}

func TestCountPanes_ZeroWhenWindowsHaveNoPanes(t *testing.T) {
	idx := state.Index{
		Sessions: []state.Session{
			{Name: "empty", Windows: []state.Window{{Index: 0}}},
		},
	}
	if got := state.CountPanes(idx); got != 0 {
		t.Errorf("CountPanes(no panes) = %d; want 0", got)
	}
}
