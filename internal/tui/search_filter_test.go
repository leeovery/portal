package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/leeovery/portal/internal/tmux"
)

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

// subsequenceOnlySession is matched by the picker's own fuzzy rule for the term
// "port" (p·o·r·t across "~/Projects/rust-tools") and by containment not at all.
func subsequenceOnlySession(t *testing.T) tmux.Session {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return tmux.Session{Name: "rust-tools", Dir: filepath.Join(home, "Projects", "rust-tools")}
}

func TestContainmentFilterRanksAStaleGenerationsTargetsByContainment(t *testing.T) {
	scattered := SessionItem{Session: subsequenceOnlySession(t)}
	matching := SessionItem{Session: tmux.Session{Name: "portal-a1b2"}}
	src := &searchItemSource{}
	// The source holds one generation; the pass is handed another's targets,
	// in an order that generation never held them in.
	src.set([]list.Item{matching, scattered})
	targets := filterTargets([]list.Item{scattered, matching})

	got := containmentFilter("port", src)("port", targets)

	if want := []int{1}; !slices.Equal(rankIndexes(got), want) {
		t.Fatalf("ranks = %v, want %v at the targets' own indexes", rankIndexes(got), want)
	}
	if fuzzy := list.DefaultFilter("port", targets); len(fuzzy) <= len(got) {
		t.Fatalf("test setup invariant: expected the picker's own rule to rank more than containment does")
	}
}

func TestContainmentFilterRanksEveryRowOfASessionListedUnderMoreThanOneTag(t *testing.T) {
	repeated := SessionItem{Session: tmux.Session{Name: "portal-a1b2"}}
	items := []list.Item{
		HeaderItem{Heading: "Work", Count: 1},
		SessionItem{Session: repeated.Session, GroupKey: "work"},
		HeaderItem{Heading: "Side", Count: 1},
		SessionItem{Session: repeated.Session, GroupKey: "side"},
	}
	src := &searchItemSource{}
	src.set(items)

	got := containmentFilter("port", src)("port", filterTargets(items))

	if want := []int{1, 3}; !slices.Equal(rankIndexes(got), want) {
		t.Fatalf("ranks = %v, want %v — one per row of the repeated session", rankIndexes(got), want)
	}
}

func TestContainmentFilterRanksNothingForATargetTheSourceDoesNotHold(t *testing.T) {
	src := &searchItemSource{}
	src.set([]list.Item{SessionItem{Session: tmux.Session{Name: "portal-a1b2"}}})
	targets := filterTargets([]list.Item{SessionItem{Session: tmux.Session{Name: "app-port"}}})

	got := containmentFilter("port", src)("port", targets)

	if len(got) != 0 {
		t.Fatalf("ranks = %v, want none for a target the source does not hold", rankIndexes(got))
	}
}

func TestContainmentFilterRanksNothingForAHeaderRowsEmptyFilterValue(t *testing.T) {
	items := []list.Item{
		HeaderItem{Heading: "Alpha", Count: 1},
		SessionItem{Session: tmux.Session{Name: "app-port"}},
	}
	src := &searchItemSource{}
	src.set(items)

	got := containmentFilter("port", src)("port", filterTargets(items))

	if want := []int{1}; !slices.Equal(rankIndexes(got), want) {
		t.Fatalf("ranks = %v, want %v — the header ranks nothing", rankIndexes(got), want)
	}
}

func TestContainmentFilterNarrowsToTheTargetsItContains(t *testing.T) {
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

func TestContainmentFilterReturnsToThePickersOwnRuleOnceTheTextIsNoLongerTheTerm(t *testing.T) {
	items := []list.Item{
		SessionItem{Session: tmux.Session{Name: "app-port"}},
		SessionItem{Session: tmux.Session{Name: "pro-tools"}},
	}
	src := &searchItemSource{}
	src.set(items)
	targets := filterTargets(items)

	got := containmentFilter("port", src)("prt", targets)

	want := list.DefaultFilter("prt", targets)
	if len(want) == 0 {
		t.Fatalf("test setup invariant: expected the picker's own rule to match a target")
	}
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

	t.Run("it yields no ranks for a non-empty target list", func(t *testing.T) {
		targets := filterTargets([]list.Item{
			SessionItem{Session: tmux.Session{Name: "app-port"}},
			SessionItem{Session: tmux.Session{Name: "pro-tools"}},
		})

		got := containmentFilter("port", &searchItemSource{})("port", targets)

		if len(got) != 0 {
			t.Fatalf("ranks = %v, want none from a source holding nothing", rankIndexes(got))
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

// collidingSessions are two distinct sessions whose filter values coincide: one
// named for the very text the other's home-abbreviated directory renders as.
func collidingSessions(t *testing.T) (named, recorded tmux.Session) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return tmux.Session{Name: "api ~/Code/api"},
		tmux.Session{Name: "api", Dir: filepath.Join(home, "Code", "api")}
}

func TestContainmentFilterOnTwoSessionsSharingOneFilterValue(t *testing.T) {
	t.Run("it ranks neither row when only one of them contains the term", func(t *testing.T) {
		named, recorded := collidingSessions(t)
		for _, order := range [][]list.Item{
			{SessionItem{Session: named}, SessionItem{Session: recorded}},
			{SessionItem{Session: recorded}, SessionItem{Session: named}},
		} {
			src := &searchItemSource{}
			src.set(order)
			targets := filterTargets(order)
			if targets[0] != targets[1] {
				t.Fatalf("test setup invariant: expected one filter value, got %q and %q", targets[0], targets[1])
			}

			got := containmentFilter("api ~", src)("api ~", targets)

			if len(got) != 0 {
				t.Fatalf("ranks = %v, want none — the rows disagree on the term", rankIndexes(got))
			}
		}
	})

	t.Run("it ranks both rows when both of them contain the term", func(t *testing.T) {
		named, recorded := collidingSessions(t)
		items := []list.Item{SessionItem{Session: named}, SessionItem{Session: recorded}}
		src := &searchItemSource{}
		src.set(items)

		got := containmentFilter("api", src)("api", filterTargets(items))

		if want := []int{0, 1}; !slices.Equal(rankIndexes(got), want) {
			t.Fatalf("ranks = %v, want %v — both rows contain the term", rankIndexes(got), want)
		}
	})
}
