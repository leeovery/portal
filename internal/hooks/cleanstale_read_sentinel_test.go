package hooks_test

import (
	"errors"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
)

// makeUnreadable replaces the staged hooks.json with a directory, so every
// later read of it fails where an earlier one succeeded.
func makeUnreadable(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove the staged hooks.json: %v", err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("stage a directory at the hooks.json path: %v", err)
	}
}

// A clean reads hooks.json twice — once for its snapshot, once under the hold it
// deletes from — and either read failing costs the cycle the same thing. Both
// therefore carry the same sentinel, so a caller classifying the failure need
// not know which of the two reads it was; a failed write carries neither.
func TestCleanStaleReadFailuresShareASentinel(t *testing.T) {
	t.Run("a failed pre-read wraps the read sentinel", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		_, err := store.CleanStale(listing(hookstest.LiveSeedA), noIndex)
		if !errors.Is(err, hooks.ErrStoreRead) {
			t.Fatalf("CleanStale error = %v, want errors.Is the read sentinel", err)
		}
	})

	t.Run("a failed delete-phase load wraps the read sentinel", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})

		// Readable when the snapshot was taken, unreadable under the exclusive
		// hold: the second read is the only one that fails here.
		_, err := store.CleanStale(func(hooks.Snapshot) ([]string, error) {
			makeUnreadable(t, path)
			return []string{hookstest.LiveSeedA}, nil
		}, noIndex)
		if !errors.Is(err, hooks.ErrStoreRead) {
			t.Fatalf("CleanStale error = %v, want errors.Is the read sentinel", err)
		}
	})

	t.Run("a failed save wraps no read sentinel", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed, WritesDenied: true})

		_, err := store.CleanStale(listing(hookstest.LiveSeedA), noIndex)
		if err == nil {
			t.Fatal("CleanStale succeeded with the directory unwritable, so nothing was reported")
		}
		if errors.Is(err, hooks.ErrStoreRead) {
			t.Fatalf("CleanStale error = %v, want a write failure carrying no read sentinel", err)
		}
	})
}
