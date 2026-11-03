//go:build integration

package cmd

import (
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/hooksweep"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	renameRestoreName    = "renamedst"
	renameRestoreNewName = "renamedst2"
)

func TestRenameRestoreCleanupSurvival_KeepsRestoredTokenKeyedHook(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	ts := tmuxtest.New(t, "ptl-3-6-clean-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	ts.Run(t, "new-session", "-d", "-s", renameRestoreName, "sleep", "infinity")
	ts.WaitForSession(t, renameRestoreName, 2*time.Second)

	// Restore's re-stamp: the recreated pane carries the token its saved state
	// held, and it is read back after the rename — the key a restored hook was
	// registered under must still name a live pane, or the sweep reaps it the
	// way the mutable session name used to let it. Omitting the stamp leaves
	// the pane unstamped and the entry unprotected.
	ts.StampPaneToken(t, renameRestoreName+":0.0", hookstest.LiveSeedA)

	ts.Run(t, "rename-session", "-t", renameRestoreName, renameRestoreNewName)
	ts.WaitForSession(t, renameRestoreNewName, 2*time.Second)

	assertLiveTokenPresent(t, client, hookstest.LiveSeedA)

	// The truly-stale entry below (ReapableSeedA) has no matching live pane and
	// is token-shaped, so the reaper can judge it: it must still be swept.
	seed := `{
  "` + hookstest.LiveSeedA + `": {"on-resume": "echo restored"},
  "` + hookstest.ReapableSeedA + `": {"on-resume": "echo gone"}
}`
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

	preRun, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("pre-cleanup store.Load: %v", err)
	}
	if _, ok := preRun[hookstest.LiveSeedA]; !ok {
		t.Fatalf("pre-cleanup seed missing token key %q; keys=%v", hookstest.LiveSeedA, keysOf(preRun))
	}
	if _, ok := preRun[hookstest.ReapableSeedA]; !ok {
		t.Fatalf("pre-cleanup seed missing stale key %q; keys=%v", hookstest.ReapableSeedA, keysOf(preRun))
	}

	if err := sweepErr(client, store); err != nil {
		t.Fatalf("hooksweep.Run: %v", err)
	}

	postRun, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("post-cleanup store.Load: %v", err)
	}
	if _, ok := postRun[hookstest.LiveSeedA]; !ok {
		t.Errorf("token-keyed hook %q was swept after a rename; want preserved "+
			"(the token is stamped on the pane and a rename cannot move it). "+
			"post-cleanup keys=%v (path=%s)", hookstest.LiveSeedA, keysOf(postRun), path)
	}
	if _, ok := postRun[hookstest.ReapableSeedA]; ok {
		t.Errorf("truly-stale hook %q survived; want swept "+
			"(no matching live pane — cleanup correctness must not be weakened). "+
			"post-cleanup keys=%v (path=%s)", hookstest.ReapableSeedA, keysOf(postRun), path)
	}
}

func assertLiveTokenPresent(t *testing.T, lister hooksweep.Reader, want string) {
	t.Helper()
	rows, err := lister.ListAllPaneHookKeys()
	if err != nil {
		t.Fatalf("ListAllPaneHookKeys: %v", err)
	}
	for _, row := range rows {
		if row.Token == want {
			return
		}
	}
	t.Fatalf("token %q not enumerated; got %+v (a stamped pane must report its token)", want, rows)
}
