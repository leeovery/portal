//go:build integration

package restore_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const exitClosesPaneBudget = 2 * time.Second

func TestExitClosesRestoredPane_NoHook(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	stateDir, ts, sessionName := setupExitClosesPane(t, "")

	driveAndAwaitMarkerClear(t, ts.Client(), stateDir, sessionName)

	ts.SendKeys(t, tmux.PaneTargetExact(sessionName, 0, 0), "exit")

	awaitPaneGone(t, ts, sessionName, 0, 0, exitClosesPaneBudget)
}

func TestExitClosesRestoredPane_WithHook(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	sentinel := filepath.Join(t.TempDir(), "hook-fired")
	hookCmd := fmt.Sprintf("echo hook-fired > %s", sentinel)

	stateDir, ts, sessionName := setupExitClosesPane(t, hookCmd)

	driveAndAwaitMarkerClear(t, ts.Client(), stateDir, sessionName)

	restoretest.WaitForFileExists(t, sentinel, restoretest.PaneReactionBudget, restoretest.PaneReactionTick)

	ts.SendKeys(t, tmux.PaneTargetExact(sessionName, 0, 0), "exit")

	awaitPaneGone(t, ts, sessionName, 0, 0, exitClosesPaneBudget)
}

func TestNoParkedShWrapperPostRestore(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	stateDir, ts, sessionName := setupExitClosesPane(t, "")

	driveAndAwaitMarkerClear(t, ts.Client(), stateDir, sessionName)

	time.Sleep(exitClosesPaneBudget)

	paneSuffix := state.SanitizePaneKey(sessionName, 0, 0)
	pattern := "sh -c.*state hydrate.*" + paneSuffix

	cmd := exec.Command("pgrep", "-fl", pattern)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("AC5 violation: parked sh -c wrapper found around "+
			"portal state hydrate (Fix 3 → Side Effects: orphan sh "+
			"parent must be eliminated). pattern=%q\npgrep output:\n%s",
			pattern, out)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		switch code := exitErr.ExitCode(); code {
		case 1: // pgrep found nothing: the pass condition.
		default:
			t.Fatalf("pgrep -fl %q: unexpected exit code %d "+
				"(want 0=match[fail] or 1=no-match[pass]); err=%v\n"+
				"output:\n%s", pattern, code, err, out)
		}
	} else {
		t.Fatalf("pgrep -fl %q: non-exec.ExitError failure (pgrep "+
			"binary missing or unrunnable?): %v\noutput:\n%s",
			pattern, err, out)
	}
}

func setupExitClosesPane(t *testing.T, hookCmd string) (string, *tmuxtest.Socket, string) {
	t.Helper()

	binDir := restoretest.BuildPortalBinaryDir(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	sessionName := sanitizeSessionForTmux(t.Name())

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	ts := tmuxtest.New(t, "ptl-exit-")
	client := ts.Client()

	ts.Run(t, "new-session", "-d", "-s", sessionName, "sleep", "infinity")
	ts.WaitForSession(t, sessionName, 2*time.Second)

	const paneToken = "exitClosesPaneToken"
	paneTarget := tmux.PaneTargetExact(sessionName, 0, 0)
	ts.StampPaneToken(t, paneTarget, paneToken)

	idx, _, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	if len(idx.Sessions) != 1 || idx.Sessions[0].Name != sessionName {
		t.Fatalf("expected one captured session named %q; got %+v",
			sessionName, idx.Sessions)
	}

	if hookCmd != "" {
		store := hooks.NewStore(hooksPath)
		if err := store.Set(paneToken, "on-resume", hooks.Registration{Command: hookCmd, Resume: resumemode.Eager}, hooks.ViaCLI); err != nil {
			t.Fatalf("hooks.Set: %v", err)
		}
	}

	data, err := state.EncodeIndex(idx)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(stateDir), data, 0o600); err != nil {
		t.Fatalf("write sessions.json: %v", err)
	}

	restoretest.RebootServer(t, ts, client)

	o := restoretest.NewRestoreOrchestrator(t, client, stateDir, binDir)
	if err := restoretest.RestoreWithMarker(t, client, o); err != nil {
		t.Fatalf("restoreWithMarker: %v", err)
	}

	return stateDir, ts, sessionName
}

func driveAndAwaitMarkerClear(t *testing.T, client *tmux.Client, stateDir, sessionName string) {
	t.Helper()
	restoretest.DriveSignalHydrate(t, client, stateDir, []string{sessionName})
	restoretest.WaitForSkeletonMarkersCleared(t, client, restoretest.HydrateBudget, restoretest.HydrateTick)
}

func awaitPaneGone(t *testing.T, ts *tmuxtest.Socket, sessionName string, window, pane int, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	wantPane := fmt.Sprintf("%d:%d", window, pane)
	var lastOut string
	var lastErr error
	for time.Now().Before(deadline) {
		coords, err := restoretest.TryLivePaneCoords(ts, sessionName)
		lastOut, lastErr = strings.Join(coords, " "), err
		if err != nil {
			return
		}
		if !slices.Contains(coords, wantPane) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("AC5 violation: pane %s:%s survived %s after `exit` "+
		"(spec AC5: pane closes on first invocation). last "+
		"list-panes output=%q err=%v",
		sessionName, wantPane, budget, lastOut, lastErr)
}

// tmux rejects ':' outright and '.' at the start of a session name.
func sanitizeSessionForTmux(name string) string {
	replacer := strings.NewReplacer(
		":", "-",
		".", "-",
		"/", "-",
		" ", "-",
		"\t", "-",
	)
	out := replacer.Replace(name)
	if out == "" {
		out = "exit-test"
	}
	return out
}
