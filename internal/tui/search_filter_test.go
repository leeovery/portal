package tui

import (
	"fmt"
	"slices"
	"sync"
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

func filterTargets(items []list.Item) []string {
	targets := make([]string, len(items))
	for i, item := range items {
		targets[i] = item.FilterValue()
	}
	return targets
}

func rankIndexes(ranks []list.Rank) []int {
	indexes := make([]int, len(ranks))
	for i, rank := range ranks {
		indexes[i] = rank.Index
	}
	return indexes
}

func TestContainmentFilterFallsBackWhenTheSourceWasReplacedAtTheSameLength(t *testing.T) {
	src := &searchItemSource{}
	// A same-length replacement: the recorded values no longer describe the
	// targets, and a containment lookup would rank "app-port" at the index the
	// replaced source holds it at rather than the one the targets do.
	src.set([]list.Item{
		SessionItem{Session: tmux.Session{Name: "app-port"}},
		SessionItem{Session: tmux.Session{Name: "pro-tools"}},
	})
	targets := filterTargets([]list.Item{
		SessionItem{Session: tmux.Session{Name: "pro-tools"}},
		SessionItem{Session: tmux.Session{Name: "app-port"}},
	})

	got := containmentFilter("port", src)("port", targets)

	want := list.DefaultFilter("port", targets)
	if !slices.Equal(rankIndexes(got), rankIndexes(want)) {
		t.Fatalf("ranks = %v, want the picker's own rule's %v", rankIndexes(got), rankIndexes(want))
	}
}

func TestContainmentFilterNarrowsWhenTheRecordedValuesMatchTheTargets(t *testing.T) {
	items := []list.Item{
		SessionItem{Session: tmux.Session{Name: "app-port"}},
		HeaderItem{Heading: "Alpha", Count: 2},
		SessionItem{Session: tmux.Session{Name: "pro-tools"}},
		SessionItem{Session: tmux.Session{Name: "portal-a1b2"}},
	}
	src := &searchItemSource{}
	src.set(items)

	got := containmentFilter("port", src)("port", filterTargets(items))

	if want := []int{0, 3}; !slices.Equal(rankIndexes(got), want) {
		t.Fatalf("ranks = %v, want %v in the list's own order", rankIndexes(got), want)
	}
	for _, rank := range got {
		if rank.MatchedIndexes != nil {
			t.Errorf("rank %d carries MatchedIndexes %v, want none", rank.Index, rank.MatchedIndexes)
		}
	}
}

func TestContainmentFilterFallsBackWhenTheFilterTextIsNoLongerTheTerm(t *testing.T) {
	items := []list.Item{
		SessionItem{Session: tmux.Session{Name: "app-port"}},
		SessionItem{Session: tmux.Session{Name: "pro-tools"}},
	}
	src := &searchItemSource{}
	src.set(items)
	targets := filterTargets(items)

	got := containmentFilter("port", src)("prt", targets)

	want := list.DefaultFilter("prt", targets)
	if !slices.Equal(rankIndexes(got), rankIndexes(want)) {
		t.Fatalf("ranks = %v, want the picker's own rule's %v", rankIndexes(got), rankIndexes(want))
	}
}

func TestContainmentFilterOnASourceThatWasNeverSet(t *testing.T) {
	t.Run("it yields no ranks for an empty target list", func(t *testing.T) {
		got := containmentFilter("port", &searchItemSource{})("port", nil)

		if len(got) != 0 {
			t.Fatalf("ranks = %v, want none", rankIndexes(got))
		}
	})

	t.Run("it falls back to the picker's own rule for a non-empty target list", func(t *testing.T) {
		targets := filterTargets([]list.Item{
			SessionItem{Session: tmux.Session{Name: "app-port"}},
			SessionItem{Session: tmux.Session{Name: "pro-tools"}},
		})

		got := containmentFilter("port", &searchItemSource{})("port", targets)

		want := list.DefaultFilter("port", targets)
		if len(want) == 0 {
			t.Fatalf("test setup invariant: expected the picker's own rule to match a target")
		}
		if !slices.Equal(rankIndexes(got), rankIndexes(want)) {
			t.Fatalf("ranks = %v, want the picker's own rule's %v", rankIndexes(got), rankIndexes(want))
		}
	})
}

func TestContainmentFilterIsRaceFreeAgainstAConcurrentSet(t *testing.T) {
	items := []list.Item{
		SessionItem{Session: tmux.Session{Name: "app-port"}},
		SessionItem{Session: tmux.Session{Name: "pro-tools"}},
	}
	src := &searchItemSource{}
	src.set(items)
	filter := containmentFilter("port", src)
	targets := filterTargets(items)

	var writers sync.WaitGroup
	writers.Go(func() {
		for i := range 500 {
			src.set([]list.Item{SessionItem{Session: tmux.Session{Name: fmt.Sprintf("app-port-%d", i)}}})
		}
	})
	for range 500 {
		filter("port", targets)
	}
	writers.Wait()
}
