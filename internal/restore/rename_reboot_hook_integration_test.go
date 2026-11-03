//go:build integration

package restore_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestRenameRebootHook_ExternalRename(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRenameRebootFire(t, func(t *testing.T, ts *tmuxtest.Socket, _ *tmux.Client) {
		t.Helper()
		ts.Run(t, "rename-session", "-t", renameOldName, renameNewName)
	})
}

func TestRenameRebootHook_PaneProcessKeptRunning(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRenameRebootFire(t, func(t *testing.T, ts *tmuxtest.Socket, _ *tmux.Client) {
		t.Helper()
		pidBefore := strings.TrimSpace(ts.Run(t, "display-message", "-p",
			"-t", tmux.PaneTarget(renameOldName, 0, 0), "#{pane_pid}"))

		ts.Run(t, "rename-session", "-t", renameOldName, renameNewName)

		pidAfter := strings.TrimSpace(ts.Run(t, "display-message", "-p",
			"-t", tmux.PaneTarget(renameNewName, 0, 0), "#{pane_pid}"))
		if pidBefore == "" || pidAfter == "" {
			t.Fatalf("pane_pid read empty (before=%q after=%q); pane addressing broke", pidBefore, pidAfter)
		}
		if pidBefore != pidAfter {
			t.Fatalf("pane process restarted across rename (pid %q → %q); "+
				"the bug only bites when the process keeps running", pidBefore, pidAfter)
		}
	})
}

func runRenameRebootFire(t *testing.T, rename func(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client)) {
	t.Helper()

	pane, hookFireFile := renameRebootPane(t)
	fx := newRebootFixture(t, "ptl-3-5-", renameOldName, []rebootPane{pane})

	rename(t, fx.ts, fx.client)

	if _, err := fx.ts.TryRun("has-session", "-t", "="+renameNewName); err != nil {
		t.Fatalf("session %q not live after rename: %v", renameNewName, err)
	}
	if liveToken := fx.ts.ReadPaneToken(t, tmux.PaneTargetExact(renameNewName, 0, 0)); liveToken != renamePaneToken {
		t.Fatalf("pane token after rename = %q; want %q (must be rename-immune)", liveToken, renamePaneToken)
	}

	fx.captureAndPersist(t, renameNewName)
	verifyHookKeyed(t, fx.hooksPath, renamePaneToken)

	if err := fx.rebootAndHydrate(t, renameNewName); err != nil {
		t.Fatalf("rebootAndHydrate: %v", err)
	}

	restoretest.AssertMarkerCount(t, hookFireFile, hookFiredMarker, 1)
}
