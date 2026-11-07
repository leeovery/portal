package hooks_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// noIndex is a readIndex whose sessions.json holds no entry.
func noIndex() ([]string, error) { return nil, nil }

// indexing answers names as the names sessions.json holds.
func indexing(names ...string) func() ([]string, error) {
	return func() ([]string, error) { return names, nil }
}

func TestCleanStaleJudgesEveryKey(t *testing.T) {
	t.Run("it keeps a key a listed session names", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})

		removed, err := store.CleanStale(listing(hookstest.LiveSeedA), noIndex)
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !slices.Equal(removed, []string{hookstest.ReapableSeedA}) {
			t.Errorf("CleanStale removed %v, want [%s]", removed, hookstest.ReapableSeedA)
		}
	})

	t.Run("it keeps a key only an index entry names", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})

		removed, err := store.CleanStale(listing(), indexing(hookstest.LiveSeedA))
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !slices.Equal(removed, []string{hookstest.ReapableSeedA}) {
			t.Errorf("CleanStale removed %v, want [%s]", removed, hookstest.ReapableSeedA)
		}
	})

	t.Run("it reaps the keys the tmux version wrote", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"token"},%q:{"on-resume":"coordinates"},%q:{"on-resume":"live"}}`,
			hookstest.TmuxEraSeedA, hookstest.TmuxEraSeedB, hookstest.LiveSeedA)})

		removed, err := store.CleanStale(listing(hookstest.LiveSeedA), noIndex)
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		want := []string{hookstest.TmuxEraSeedA, hookstest.TmuxEraSeedB}
		slices.Sort(want)
		if !slices.Equal(removed, want) {
			t.Fatalf("CleanStale removed %v, want %v", removed, want)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if !slices.Equal(keysOf(h), []string{hookstest.LiveSeedA}) {
			t.Errorf("file holds %v, want only [%s]", keysOf(h), hookstest.LiveSeedA)
		}
	})

	t.Run("it deletes an empty key", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{"":{"on-resume":"malformed"},%q:{"on-resume":"live"}}`, hookstest.LiveSeedA)})

		removed, err := store.CleanStale(listing(hookstest.LiveSeedA), noIndex)
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !slices.Equal(removed, []string{""}) {
			t.Fatalf("CleanStale removed %v, want the empty key", removed)
		}
	})

	t.Run("it writes no file and emits no summary when every key is named", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"a"},%q:{"on-resume":"b"}}`, hookstest.LiveSeedA, hookstest.LiveSeedB)})
		before := readFileBytes(t, path)

		sink := logtest.Install(t)
		removed, err := store.CleanStale(listing(hookstest.LiveSeedA), indexing(hookstest.LiveSeedB))
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if len(removed) != 0 {
			t.Fatalf("CleanStale removed %v, want nothing", removed)
		}
		for _, r := range sink.Records() {
			if r.Msg == "clean-stale" {
				t.Errorf("unexpected clean-stale record: %+v", r)
			}
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed when every key was named")
	})
}
