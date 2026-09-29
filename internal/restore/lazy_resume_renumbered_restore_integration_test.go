//go:build integration

package restore_test

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	renumberedSession = "lazyrenum"
	// The width and charset the pane-token mint produces.
	renumberedToken         = "lzrenu"
	renumberedCommand       = "echo renumbered-ok"
	renumberedPreRebootLine = "renumbered-before-reboot"

	// Saved at the window the killed one leaves behind it; restore closes the
	// gap, so the pane comes back one window lower.
	renumberedSavedWindow    = 2
	renumberedRestoredWindow = 1
)

// A pane that waited across a reboot is restored away from its saved address,
// and two captures reach it before its helper has marked it pending: the
// commit a session close fires, and the daemon's first tick after the restore.
func TestLazyResumePanel_RenumberedRestoreKeepsTranscriptThroughEarlyCaptures(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	fx := setupRenumberedWaitingPane(t)
	tokenPath := state.PendingScrollbackFile(renumberedToken)
	restored := tmux.PaneTargetExact(renumberedSession, renumberedRestoredWindow, 0)

	waited := fx.captureRound(t)
	if got := renumberedRecord(t, waited.idx, renumberedSavedWindow).ScrollbackFile; got != tokenPath {
		t.Fatalf("waiting pane saved naming %q; want %q", got, tokenPath)
	}
	transcript := fx.readScrollbackAt(t, tokenPath)
	if !strings.Contains(string(transcript), renumberedPreRebootLine) {
		t.Fatalf("the saved transcript never held %q; the fixture proves nothing:\n%s",
			renumberedPreRebootLine, transcript)
	}

	restoretest.RebootServer(t, fx.ts, fx.client)
	if err := restoretest.RestoreFromState(t, fx.client, fx.stateDir, fx.binDir); err != nil {
		t.Fatalf("RestoreFromState: %v", err)
	}
	fx.assertRestoredRenumbered(t)

	t.Run("it keeps the transcript through a commit-now taken before the pending mark", func(t *testing.T) {
		idx := fx.commitNowRound(t)
		fx.assertTranscriptKept(t, idx, tokenPath, transcript)
	})

	t.Run("it keeps the transcript through the daemon's first tick before the pending mark", func(t *testing.T) {
		round := fx.captureRound(t)
		fx.assertTranscriptKept(t, round.idx, tokenPath, transcript)
		if _, captured := round.written[renumberedKey(renumberedRestoredWindow)]; captured {
			t.Fatalf("the skeleton-marked pane was capture-paned")
		}
	})

	restoretest.DriveSignalHydrate(t, fx.client, fx.stateDir, []string{renumberedSession})
	restoretest.WaitForSkeletonMarkersCleared(t, fx.client, restoretest.HydrateBudget, restoretest.HydrateTick)
	fx.awaitScreenContains(t, restored, panelTitle)
	fx.assertPending(t, restored)
	fx.assertTranscriptKept(t, fx.captureRound(t).idx, tokenPath, transcript)

	t.Run("it restores the original transcript above its panel at the following reboot", func(t *testing.T) {
		fx.rebootRestoreHydrateSessions(t, []string{renumberedSession})

		fx.awaitScreenContains(t, restored, panelTitle)
		history := fx.paneHistory(t, restored)
		if !strings.Contains(history, renumberedPreRebootLine) {
			t.Fatalf("restored pane history lost %q; capture-pane -a -p:\n%s",
				renumberedPreRebootLine, history)
		}
	})
}

// setupRenumberedWaitingPane leaves a tokened pane under a lazy registration at
// window 2 of a session whose window 1 is gone, captured once and then marked
// pending, so the next capture files it as a pane waiting at shutdown.
func setupRenumberedWaitingPane(t *testing.T) *lazyPanelFixture {
	t.Helper()

	fx := &lazyPanelFixture{
		binDir: restoretest.BuildPortalBinaryDir(t),
		hashes: state.HashMap{},
	}

	_, fx.stateDir = portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", fx.stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	configDir := t.TempDir()
	hooksPath := filepath.Join(configDir, "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)
	t.Setenv("PORTAL_PREFS_FILE", filepath.Join(configDir, "prefs.json"))
	if err := hooks.NewStore(hooksPath).Set(renumberedToken, "on-resume",
		hooks.Registration{Command: renumberedCommand}, hooks.ViaCLI); err != nil {
		t.Fatalf("hooks.Set: %v", err)
	}

	portaltest.RegisterStateDirTeardownGuard(t, fx.stateDir)

	fx.ts = tmuxtest.New(t, "ptl-lzrn-")
	fx.client = fx.ts.Client()
	fx.workDir = t.TempDir()

	fx.ts.Run(t, "new-session", "-d", "-s", renumberedSession, "-c", fx.workDir)
	fx.ts.WaitForSession(t, renumberedSession, 2*time.Second)
	fx.ts.Run(t, "new-window", "-t", renumberedSession+":1", "-c", fx.workDir)
	fx.ts.Run(t, "new-window", "-t", renumberedSession+":2", "-c", fx.workDir)
	fx.ts.Run(t, "kill-window", "-t", renumberedSession+":1")

	saved := tmux.PaneTargetExact(renumberedSession, renumberedSavedWindow, 0)
	fx.ts.StampPaneToken(t, saved, renumberedToken)
	fx.printLine(t, saved, renumberedPreRebootLine)

	fx.captureRound(t)
	fx.ts.Run(t, "set-option", "-p", "-t", string(saved), state.ResumePendingOption, "1")
	return fx
}

func (fx *lazyPanelFixture) assertRestoredRenumbered(t *testing.T) {
	t.Helper()
	out := fx.ts.Run(t, "list-windows", "-t", string(tmux.CoordTargetExact(renumberedSession)), "-F", "#{window_index}")
	var windows []int
	for field := range strings.FieldsSeq(out) {
		n, err := strconv.Atoi(field)
		if err != nil {
			t.Fatalf("window index %q: %v", field, err)
		}
		windows = append(windows, n)
	}
	if !slices.Equal(windows, []int{0, renumberedRestoredWindow}) {
		t.Fatalf("restored windows = %v; want [0 %d], a restore that renumbered the waiting pane",
			windows, renumberedRestoredWindow)
	}
	markers, err := state.ListSkeletonMarkers(fx.client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}
	if _, armed := markers[renumberedKey(renumberedRestoredWindow)]; !armed {
		t.Fatalf("skeleton markers = %v; want the restored pane still armed",
			restoretest.SortedKeySet(markers))
	}
}

// commitNowRound takes the capture `portal state commit-now` takes: the
// previous index read from disk, the composite, and a commit that writes no
// scrollback.
func (fx *lazyPanelFixture) commitNowRound(t *testing.T) state.Index {
	t.Helper()
	prev, _, err := state.ReadIndex(fx.stateDir)
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	capture, err := state.CaptureAndRefile(fx.client, fx.stateDir, &prev, nil, nil)
	if err != nil {
		t.Fatalf("CaptureAndRefile: %v", err)
	}
	if err := state.Commit(fx.stateDir, capture.Index, false, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	fx.prev = capture.Index
	return capture.Index
}

func (fx *lazyPanelFixture) assertTranscriptKept(t *testing.T, idx state.Index, tokenPath string, transcript []byte) {
	t.Helper()
	record := renumberedRecord(t, idx, renumberedRestoredWindow)
	if record.ScrollbackFile != tokenPath {
		t.Fatalf("restored pane's record names %q; want %q", record.ScrollbackFile, tokenPath)
	}
	if got := fx.readScrollbackAt(t, tokenPath); string(got) != string(transcript) {
		t.Fatalf("token-named scrollback changed\nbefore:\n%s\nafter:\n%s", transcript, got)
	}
}

func renumberedKey(window int) string {
	return state.SanitizePaneKey(renumberedSession, window, 0)
}

func renumberedRecord(t *testing.T, idx state.Index, window int) state.Pane {
	t.Helper()
	sess := restoretest.FindCapturedSession(t, idx, renumberedSession)
	for _, w := range sess.Windows {
		if w.Index == window && len(w.Panes) == 1 {
			return w.Panes[0]
		}
	}
	t.Fatalf("captured session %q holds no single-pane window %d: %+v", renumberedSession, window, sess.Windows)
	return state.Pane{}
}
