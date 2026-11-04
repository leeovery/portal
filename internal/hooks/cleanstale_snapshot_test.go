package hooks_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// listing answers names as the backend's listing whatever the snapshot holds:
// the ordinary case, where nothing lands between the reads.
func listing(names ...string) func(hooks.Snapshot) ([]string, error) {
	return func(hooks.Snapshot) ([]string, error) { return names, nil }
}

// The snapshot is the file as CleanStale found it before the listing ran. It
// exists to keep a key the listing never saw out of the delete set, because
// such a key was written after the listing was read — so that read never had
// the chance to protect it.
func TestCleanStaleSnapshotNarrowing(t *testing.T) {
	t.Run("it deletes a key present in the file, in the snapshot and named by nothing", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"live"},%q:{"on-resume":"gone"}}`, hookstest.LiveSeedA, hookstest.ReapableSeedA)})

		removed, err := store.CleanStale(listing(hookstest.LiveSeedA), noIndex)
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !slices.Equal(removed, []string{hookstest.ReapableSeedA}) {
			t.Fatalf("CleanStale removed %v, want [%s]", removed, hookstest.ReapableSeedA)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if _, ok := h[hookstest.ReapableSeedA]; ok {
			t.Error("a key in the file, in the snapshot and named by nothing survived")
		}
		if _, ok := h[hookstest.LiveSeedA]; !ok {
			t.Error("the listed key was reaped")
		}
	})

	t.Run("it retains a key written after the snapshot", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"}}`, hookstest.ReapableSeedA)})

		// The registration that lands while the listing runs, under a name
		// nothing lists or indexes.
		removed, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) {
			if err := store.Set(hookstest.ReapableSeedB, "on-resume", hooks.Registration{Command: "fresh"}, hooks.ViaCLI); err != nil {
				return nil, fmt.Errorf("seed the late registration: %w", err)
			}
			return nil, nil
		}, noIndex)
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !slices.Equal(removed, []string{hookstest.ReapableSeedA}) {
			t.Fatalf("CleanStale removed %v, want [%s]", removed, hookstest.ReapableSeedA)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if _, ok := h[hookstest.ReapableSeedB]; !ok {
			t.Errorf("a key the snapshot did not hold was deleted; file now holds %v (%s)", keysOf(h), string(readFileBytes(t, path)))
		}
	})

	// A name recurs: a session made again under a name whose stale hook the
	// snapshot holds, and `hook set` on it, claim its index entry and then
	// write under that key. The index read under the hold is what sees the
	// claim, the snapshot narrowing being blind to a key it already holds.
	t.Run("it retains a re-registration under a key the snapshot holds, its index entry written first", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"}}`, hookstest.ReapableSeedA)})

		var indexed []string
		removed, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) {
			indexed = append(indexed, hookstest.ReapableSeedA)
			if err := store.Set(hookstest.ReapableSeedA, "on-resume", hooks.Registration{Command: "fresh"}, hooks.ViaCLI); err != nil {
				return nil, fmt.Errorf("seed the re-registration: %w", err)
			}
			return nil, nil
		}, func() ([]string, error) { return indexed, nil })
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if len(removed) != 0 {
			t.Errorf("CleanStale removed %v, want nothing", removed)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got := h[hookstest.ReapableSeedA]["on-resume"].Command; got != "fresh" {
			t.Errorf("on-resume = %q after the clean, want the re-registration kept", got)
		}
	})

	t.Run("it hands the listing the file as it stood before it ran", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"}}`, hookstest.ReapableSeedA)})

		var seen []string
		if _, err := store.CleanStale(func(snapshot hooks.Snapshot) ([]string, error) {
			if err := store.Set(hookstest.ReapableSeedB, "on-resume", hooks.Registration{Command: "fresh"}, hooks.ViaCLI); err != nil {
				return nil, fmt.Errorf("seed the late registration: %w", err)
			}
			seen = keysOf(snapshot)
			return nil, nil
		}, noIndex); err != nil {
			t.Fatalf("CleanStale: %v", err)
		}

		if !slices.Equal(seen, []string{hookstest.ReapableSeedA}) {
			t.Errorf("the listing was handed %v, want [%s] — the snapshot was read after it, not before", seen, hookstest.ReapableSeedA)
		}
	})

	t.Run("it holds no lock while the listing runs", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})

		probed := false
		if _, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) {
			probed = true
			// The listing is the caller's own work: the clean must not be
			// holding the sidecar while it runs.
			hookstest.AssertSidecarFree(t, path)
			return []string{hookstest.LiveSeedA}, nil
		}, noIndex); err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !probed {
			t.Fatal("the listing never ran — the probe proves nothing about the lock")
		}
	})

	t.Run("it reads the index under the exclusive hold", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})

		probed := false
		if _, err := store.CleanStale(listing(hookstest.LiveSeedA), func() ([]string, error) {
			probed = true
			hookstest.AssertSidecarHeld(t, path)
			return nil, nil
		}); err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !probed {
			t.Fatal("the index read never ran — the probe proves nothing about the lock")
		}
	})

	for _, tc := range []struct {
		name      string
		list      func(hooks.Snapshot) ([]string, error)
		readIndex func(error) func() ([]string, error)
	}{
		{
			name:      "a listing error",
			list:      func(hooks.Snapshot) ([]string, error) { return nil, errSentinel },
			readIndex: func(error) func() ([]string, error) { return noIndex },
		},
		{
			name:      "an index error",
			list:      listing(hookstest.LiveSeedA),
			readIndex: func(err error) func() ([]string, error) { return func() ([]string, error) { return nil, err } },
		},
	} {
		t.Run(tc.name+" aborts the clean untouched", func(t *testing.T) {
			store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
			before := readFileBytes(t, path)
			sink := logtest.Install(t)

			removed, err := store.CleanStale(tc.list, tc.readIndex(errSentinel))
			if !errors.Is(err, errSentinel) {
				t.Fatalf("CleanStale error = %v, want the read's own error", err)
			}
			if len(removed) != 0 {
				t.Errorf("removed = %v, want none", removed)
			}
			hookstest.AssertHooksFileUnchanged(t, path, before, "changed on an aborted clean")
			if recs := sink.Records(); len(recs) != 0 {
				t.Errorf("an aborted clean emitted %d records, want 0: %+v", len(recs), recs)
			}
		})
	}

	t.Run("it derives the delete set from the file under the lock, not from the snapshot", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"},%q:{"on-resume":"also-gone"}}`, hookstest.ReapableSeedA, hookstest.ReapableSeedB)})

		// Another writer removes a key the snapshot holds and the clean would
		// otherwise have reaped, so it must not be named as removed.
		removed, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) {
			if _, err := store.Remove(hookstest.ReapableSeedB, "on-resume", hooks.ViaCLI); err != nil {
				return nil, fmt.Errorf("remove during the listing: %w", err)
			}
			return nil, nil
		}, noIndex)
		if err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !slices.Equal(removed, []string{hookstest.ReapableSeedA}) {
			t.Fatalf("CleanStale removed %v, want only [%s] — a key no longer in the file was reported as removed", removed, hookstest.ReapableSeedA)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if len(h) != 0 {
			t.Errorf("file holds %v, want nothing (%s)", keysOf(h), string(readFileBytes(t, path)))
		}
	})

	// A file read as empty would pass for one with nothing to sweep, so a
	// file that doesn't parse aborts the clean as one that can't be read does.
	for _, tc := range []struct {
		name    string
		staging hookstest.Staging
		want    error
	}{
		{name: "an unreadable file", staging: hookstest.Staging{Unreadable: true}, want: hooks.ErrStoreRead},
		{name: "a file that doesn't parse", staging: hookstest.Staging{Seed: "{invalid json!!!"}, want: hooks.ErrMalformed},
	} {
		t.Run(tc.name+" aborts before the listing", func(t *testing.T) {
			_, path := hookstest.StageStore(t, tc.staging)
			read := false

			removed, err := hooks.NewStore(path).CleanStale(func(hooks.Snapshot) ([]string, error) {
				read = true
				return nil, nil
			}, func() ([]string, error) {
				read = true
				return nil, nil
			})
			if !errors.Is(err, hooks.ErrStoreRead) || !errors.Is(err, tc.want) {
				t.Fatalf("CleanStale error = %v, want errors.Is ErrStoreRead and %v", err, tc.want)
			}
			if read {
				t.Error("the listing or the index was read despite a file it could not judge")
			}
			if len(removed) != 0 {
				t.Errorf("removed = %v, want none", removed)
			}
		})
	}
}

var errSentinel = errors.New("nothing to read")

func keysOf(h hooks.Snapshot) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
