package hooksweep

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
)

// The listing is a read outside the lock, so it is always older than the
// mutation it feeds. Taking the snapshot before it, and the index under the
// lock, is what keeps a registration that lands in that window out of the
// delete set. The world's during seam lands it there: after the snapshot,
// before the deletion.
func TestSweepNeverReapsARegistrationMadeMidSweep(t *testing.T) {
	t.Run("it retains an entry registered under a new key", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmtSeed(hookstest.LiveSeedA, "cmd-live")})
		w.during = func() {
			if err := store.Set(hookstest.ReapableSeedA, "on-resume", hooks.Registration{Command: "cmd-fresh"}, hooks.ViaCLI); err != nil {
				t.Errorf("register a hook during the listing: %v", err)
			}
		}

		if err := runErr(t, w.sources(), store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if _, ok := h[hookstest.ReapableSeedA]; !ok {
			t.Errorf("an entry registered during the listing was reaped; file holds %v (%s)",
				keysOf(h), hookstest.HooksFileBytes(t, path))
		}
	})

	// A name recurs. The stale hook of a session that ended is in the
	// snapshot; the session is made again and `hook set` claims its entry,
	// then registers under the same key, both after the listing. Only the
	// index read under the lock can protect it.
	t.Run("it retains a re-registration under a key the snapshot holds stale", func(t *testing.T) {
		w := newWorld(t)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmtSeed(hookstest.ReapableSeedA, "cmd-gone")})
		w.during = func() {
			w.index(t, hookstest.ReapableSeedA)
			if err := store.Set(hookstest.ReapableSeedA, "on-resume", hooks.Registration{Command: "cmd-fresh"}, hooks.ViaCLI); err != nil {
				t.Errorf("re-register the hook during the listing: %v", err)
			}
		}

		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if len(outcome.Removed) != 0 {
			t.Errorf("Removed = %v, want nothing", outcome.Removed)
		}
		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got := h[hookstest.ReapableSeedA]["on-resume"].Command; got != "cmd-fresh" {
			t.Errorf("on-resume = %q, want the re-registration kept", got)
		}
	})

	t.Run("it holds no lock while listing", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmtSeed(hookstest.LiveSeedA, "cmd-live")})
		probed := false
		w.during = func() {
			probed = true
			hookstest.AssertSidecarFree(t, path)
		}

		if err := runErr(t, w.sources(), store); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !probed {
			t.Fatal("the listing never ran — the probe proves nothing about the lock")
		}
	})

	t.Run("it reports exactly what was deleted", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Seed: hooksBody(hookstest.LiveSeedA, hookstest.ReapableSeedB, hookstest.ReapableSeedC),
		})
		// Both mid-cycle windows at once, so the outcome is measured against
		// what this sweep deleted rather than against a file delta the two
		// happen to agree on: another writer removes C — a key the snapshot
		// holds and the sweep would otherwise have reaped — while a
		// registration lands for D. C leaves the file by someone else's hand
		// and D never enters the snapshot, so neither may be named.
		w.during = func() {
			if _, err := store.Remove(hookstest.ReapableSeedC, "on-resume", hooks.ViaCLI); err != nil {
				t.Errorf("remove a hook during the listing: %v", err)
			}
			if err := store.Set(hookstest.ReapableSeedD, "on-resume", hooks.Registration{Command: "cmd-fresh"}, hooks.ViaCLI); err != nil {
				t.Errorf("register a hook during the listing: %v", err)
			}
		}

		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if want := []string{hookstest.ReapableSeedB}; !slices.Equal(outcome.Removed, want) {
			t.Errorf("Removed = %v, want only %v — the outcome names this sweep's deletions, not a prediction over the file (file holds %s)",
				outcome.Removed, want, hookstest.HooksFileBytes(t, path))
		}
	})
}

// Reaching CleanStale is not free: it creates the config directory and the
// sidecar. An install that has never registered a hook must not pay that
// every cycle.
func TestSweepTakesNoLockWithNothingPersisted(t *testing.T) {
	configRoot := t.TempDir()
	hooksPath := filepath.Join(configRoot, "portal", "hooks.json")

	if err := runErr(t, newWorld(t, hookstest.LiveSeedA).sources(), hooks.NewStore(hooksPath)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if entries := dirListing(t, configRoot); len(entries) != 0 {
		t.Errorf("the sweep created %v under a config root holding no hooks.json", entries)
	}
	if _, err := os.Stat(hooksPath + ".lock"); err == nil {
		t.Error("the sweep created the sidecar with nothing persisted")
	}
}
