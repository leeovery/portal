package hooksweep

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
)

// The enumeration is a tmux read outside the lock, so it is always older than
// the mutation it feeds. Taking the snapshot before it is what keeps a
// registration that lands in that window out of the delete set.
func TestHookSweepSnapshotPrecedesEnumeration(t *testing.T) {
	t.Run("it retains an entry written during the pane enumeration", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q: {"on-resume": "cmd-live"}}`, hookstest.LiveSeedA)})

		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), during: func() {
			if err := store.Set(hookstest.ReapableSeedA, "on-resume", hooks.Registration{Command: "cmd-fresh"}, hooks.ViaCLI); err != nil {
				t.Errorf("register a hook during the enumeration: %v", err)
			}
		}}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		if _, ok := postRun[hookstest.ReapableSeedA]; !ok {
			t.Errorf("an entry registered during the enumeration was reaped; file holds %v (%s)",
				keysOf(postRun), hookstest.HooksFileBytes(t, path))
		}
		if _, ok := postRun[hookstest.LiveSeedA]; !ok {
			t.Errorf("the live entry was reaped; file holds %v", keysOf(postRun))
		}
	})

	t.Run("it holds no lock while enumerating", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q: {"on-resume": "cmd-live"}}`, hookstest.LiveSeedA)})

		probed := false
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), during: func() {
			probed = true
			// A tmux read must sit outside the lock.
			hookstest.AssertSidecarFree(t, path)
		}}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !probed {
			t.Fatal("the enumeration never ran — the probe proves nothing about the lock")
		}
	})

	t.Run("it reports exactly what was deleted", func(t *testing.T) {
		seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-live"},
  %q: {"on-resume": "cmd-b"},
  %q: {"on-resume": "cmd-c"}
}`, hookstest.LiveSeedA, hookstest.ReapableSeedB, hookstest.ReapableSeedC)
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		// Both mid-cycle windows at once, so the callback is measured against
		// what this sweep deleted rather than against a file delta the two
		// happen to agree on: another writer removes seed C — a key the
		// snapshot holds and the sweep would otherwise have reaped — while a
		// registration lands for seed D. C leaves the file by someone else's
		// hand and D never enters the snapshot, so neither may be named.
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), during: func() {
			if _, err := store.Remove(hookstest.ReapableSeedC, "on-resume", hooks.ViaCLI); err != nil {
				t.Errorf("remove a hook during the enumeration: %v", err)
			}
			if err := store.Set(hookstest.ReapableSeedD, "on-resume", hooks.Registration{Command: "cmd-fresh"}, hooks.ViaCLI); err != nil {
				t.Errorf("register a hook during the enumeration: %v", err)
			}
		}}

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		reported := slices.Clone(outcome.Removed)
		slices.Sort(reported)

		want := []string{hookstest.ReapableSeedB}
		if !slices.Equal(reported, want) {
			t.Errorf("Removed reported %v, want only %v — the outcome names this sweep's deletions, not a prediction over the file (file holds %s)",
				reported, want, hookstest.HooksFileBytes(t, path))
		}
	})
}

// After the sweep's exclusive hold arrived, reaching CleanStale stopped being
// free: it creates the config directory and the sidecar. An install that has
// never registered a hook must not pay that every cycle.
func TestHookSweepTakesNoLockWithNothingPersisted(t *testing.T) {
	configRoot := t.TempDir()
	configDir := filepath.Join(configRoot, "portal")
	hooksPath := filepath.Join(configDir, "hooks.json")

	lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA)}
	if err := runErr(lister, hooks.NewStore(hooksPath)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if entries := dirListing(t, configRoot); len(entries) != 0 {
		t.Errorf("the sweep created %v under a config root holding no hooks.json", entries)
	}
	if _, err := os.Stat(hooksPath + ".lock"); err == nil {
		t.Error("the sweep created the sidecar with nothing persisted")
	}
}
