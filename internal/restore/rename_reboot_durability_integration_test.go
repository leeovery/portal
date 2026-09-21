//go:build integration

package restore_test

import (
	"testing"

	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestRenameRebootHook_DurableAcrossRepeatedReboots(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	pane, hookFireFile := renameRebootPane(t)
	fx := newRebootFixture(t, "ptl-3-6-dur-", renameOldName, []rebootPane{pane})

	fx.ts.Run(t, "rename-session", "-t", renameOldName, renameNewName)
	if _, err := fx.ts.TryRun("has-session", "-t", "="+renameNewName); err != nil {
		t.Fatalf("session %q not live after rename: %v", renameNewName, err)
	}

	fx.captureAndPersist(t, renameNewName)

	if err := fx.rebootAndHydrate(t, renameNewName); err != nil {
		t.Fatalf("cycle 1 rebootAndHydrate: %v", err)
	}
	restoretest.AssertMarkerCount(t, hookFireFile, hookFiredMarker, 1)

	nextIdx, _, err := state.CaptureStructure(fx.client, nil, nil, nil)
	if err != nil {
		t.Fatalf("next CaptureStructure: %v", err)
	}

	t.Run("it re-persists the pane token on the next capture after restore", func(t *testing.T) {
		sess := restoretest.FindCapturedSession(t, nextIdx, renameNewName)
		if got := capturedPaneToken(t, sess); got != renamePaneToken {
			t.Fatalf("next capture pane token = %q; want %q (the token must be RE-PERSISTED by the "+
				"restore re-stamp — an empty one here orphans the hook on the next reboot)",
				got, renamePaneToken)
		}
	})

	verifyHookKeyed(t, fx.hooksPath, renamePaneToken)

	// The second cycle is driven from the post-restore capture, not the original
	// snapshot: that is what makes it a round-trip rather than a replay.
	fx.persist(t, nextIdx, renameNewName)

	if err := fx.rebootAndHydrate(t, renameNewName); err != nil {
		t.Fatalf("cycle 2 rebootAndHydrate: %v", err)
	}

	if liveToken := fx.ts.ReadPaneToken(t, tmux.PaneTargetExact(renameNewName, 0, 0)); liveToken != renamePaneToken {
		t.Errorf("live pane token after the second reboot = %q; want %q (must survive repeated reboots)",
			liveToken, renamePaneToken)
	}

	t.Run("it fires the resume hook again on a second reboot cycle", func(t *testing.T) {
		restoretest.AssertMarkerCount(t, hookFireFile, hookFiredMarker, 2)
	})
}
