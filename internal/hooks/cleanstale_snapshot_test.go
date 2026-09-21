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

// enumerating answers tokens as the live set whatever the snapshot holds: the
// ordinary case, where nothing lands between the two reads.
func enumerating(tokens ...string) func(hooks.Snapshot) ([]string, error) {
	return func(hooks.Snapshot) ([]string, error) { return tokens, nil }
}

// The snapshot is the file as CleanStale found it before the enumeration ran.
// It exists to keep a key the enumeration never saw out of the delete set,
// because such a key was written after the live set was read — so that read
// never had the chance to protect it.
func TestCleanStaleSnapshotNarrowing(t *testing.T) {
	t.Run("it deletes a key present in the file, in the snapshot and absent from the live set", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"live"},%q:{"on-resume":"gone"}}`, hookstest.LiveSeedA, hookstest.ReapableSeedA)})

		removed, err := store.CleanStale(enumerating(hookstest.LiveSeedA))
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
			t.Error("a key in the file, in the snapshot and absent from the live set survived")
		}
		if _, ok := h[hookstest.LiveSeedA]; !ok {
			t.Error("the live key was reaped")
		}
	})

	t.Run("it retains a key written after the snapshot", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"}}`, hookstest.ReapableSeedA)})

		// The registration that lands while the enumeration runs: token-shaped,
		// absent from the live set, and therefore reapable on shape alone.
		removed, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) {
			if err := store.Set(hookstest.ReapableSeedB, "on-resume", hooks.Registration{Command: "fresh"}, hooks.ViaCLI); err != nil {
				return nil, fmt.Errorf("seed the late registration: %w", err)
			}
			return nil, nil
		})
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

	t.Run("it hands the enumeration the file as it stood before it ran", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"}}`, hookstest.ReapableSeedA)})

		var seen []string
		if _, err := store.CleanStale(func(snapshot hooks.Snapshot) ([]string, error) {
			if err := store.Set(hookstest.ReapableSeedB, "on-resume", hooks.Registration{Command: "fresh"}, hooks.ViaCLI); err != nil {
				return nil, fmt.Errorf("seed the late registration: %w", err)
			}
			seen = keysOf(snapshot)
			return nil, nil
		}); err != nil {
			t.Fatalf("CleanStale: %v", err)
		}

		if !slices.Equal(seen, []string{hookstest.ReapableSeedA}) {
			t.Errorf("the enumeration was handed %v, want [%s] — the snapshot was read after it, not before", seen, hookstest.ReapableSeedA)
		}
	})

	t.Run("it holds no lock while the enumeration runs", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"}}`, hookstest.ReapableSeedA)})
		// A live entry beside the stale one, so the delete set is narrower than
		// the file.
		if err := store.Set(hookstest.LiveSeedA, "on-resume", hooks.Registration{Command: "live"}, hooks.ViaCLI); err != nil {
			t.Fatalf("register the live entry: %v", err)
		}

		probed := false
		if _, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) {
			probed = true
			// The enumeration is the caller's own work: the clean must not be
			// holding the sidecar while it runs.
			hookstest.AssertSidecarFree(t, path)
			return []string{hookstest.LiveSeedA}, nil
		}); err != nil {
			t.Fatalf("CleanStale: %v", err)
		}
		if !probed {
			t.Fatal("the enumeration never ran — the probe proves nothing about the lock")
		}
	})

	t.Run("an enumeration error aborts the clean untouched", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"}}`, hookstest.ReapableSeedA)})
		// A live entry beside the stale one, registered before the sink is
		// installed so its own breadcrumb is not counted against the aborted
		// clean, which must emit nothing at all.
		if err := store.Set(hookstest.LiveSeedA, "on-resume", hooks.Registration{Command: "live"}, hooks.ViaCLI); err != nil {
			t.Fatalf("register the live entry: %v", err)
		}
		before := readFileBytes(t, path)

		sentinel := errors.New("nothing to enumerate")
		sink := logtest.Install(t)

		removed, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) { return nil, sentinel })
		if !errors.Is(err, sentinel) {
			t.Fatalf("CleanStale error = %v, want the enumeration's own error", err)
		}
		if len(removed) != 0 {
			t.Errorf("removed = %v, want none", removed)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "changed on an aborted clean")
		if recs := sink.Records(); len(recs) != 0 {
			t.Errorf("an aborted clean emitted %d records, want 0: %+v", len(recs), recs)
		}
	})

	t.Run("it derives the delete set from the file under the lock, not from the snapshot", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q:{"on-resume":"gone"},%q:{"on-resume":"also-gone"}}`, hookstest.ReapableSeedA, hookstest.ReapableSeedB)})

		// Another writer removes a key the snapshot holds and the clean would
		// otherwise have reaped, so it must not be named as removed.
		removed, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) {
			if _, err := store.Remove(hookstest.ReapableSeedB, "on-resume", hooks.ViaCLI); err != nil {
				return nil, fmt.Errorf("remove during the enumeration: %w", err)
			}
			return nil, nil
		})
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

	t.Run("an unreadable file aborts before the enumeration", func(t *testing.T) {
		// A directory at the hooks.json path is what makes the read fail:
		// malformed JSON decodes to an empty map instead of erroring.
		_, path := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})
		enumerated := false

		removed, err := hooks.NewStore(path).CleanStale(func(hooks.Snapshot) ([]string, error) {
			enumerated = true
			return nil, nil
		})
		if !errors.Is(err, hooks.ErrStoreRead) {
			t.Fatalf("CleanStale error = %v, want errors.Is ErrStoreRead", err)
		}
		if enumerated {
			t.Error("the enumeration ran despite an unreadable file — it was never judgeable")
		}
		if len(removed) != 0 {
			t.Errorf("removed = %v, want none", removed)
		}
	})
}

func keysOf(h hooks.Snapshot) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
