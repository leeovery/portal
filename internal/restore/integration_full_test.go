//go:build integration

package restore_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

type fixtureSession struct {
	name        string
	envKey      string
	envValue    string
	cwds        [2][2]string
	zoomedW     int
	zoomedP     int
	activeWin   int
	activePanes [2]int
}

func TestPhase3Integration_FullRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	t.Setenv("PORTAL_HOOKS_FILE", filepath.Join(t.TempDir(), "hooks.json"))

	alpha := fixtureSession{
		name:        "alpha",
		envKey:      "ALPHA_ENV",
		envValue:    "alpha-value",
		cwds:        [2][2]string{{t.TempDir(), t.TempDir()}, {t.TempDir(), t.TempDir()}},
		zoomedW:     0,
		zoomedP:     1,
		activeWin:   1,
		activePanes: [2]int{1, 0},
	}
	beta := fixtureSession{
		name:        "beta",
		envKey:      "BETA_ENV",
		envValue:    "beta-value",
		cwds:        [2][2]string{{t.TempDir(), t.TempDir()}, {t.TempDir(), t.TempDir()}},
		zoomedW:     1,
		zoomedP:     0,
		activeWin:   0,
		activePanes: [2]int{0, 0},
	}

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	ts := tmuxtest.New(t, "ptl-3-13-")
	client := ts.Client()

	createFullTopology(t, ts, alpha)
	createFullTopology(t, ts, beta)

	idx, _, err := state.CaptureStructure(client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	verifyTopologyShape(t, idx, alpha, beta)

	scrollbackFixtures := map[string][]byte{}
	for _, fx := range []fixtureSession{alpha, beta} {
		for w := range 2 {
			for p := range 2 {
				key := state.SanitizePaneKey(fx.name, w, p)
				bytes := ansiFixtureBytes(fx.name, w, p)
				scrollbackFixtures[key] = bytes
				path := state.ScrollbackFile(stateDir, key)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatalf("mkdir scrollback dir: %v", err)
				}
				if err := os.WriteFile(path, bytes, 0o600); err != nil {
					t.Fatalf("write scrollback %s: %v", key, err)
				}
			}
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

	verifyLiveStructure(t, ts, alpha, beta)

	verifyZoomFlags(t, ts, alpha, beta)

	verifyActivePanes(t, ts, alpha, beta)

	verifySessionEnv(t, client, alpha)
	verifySessionEnv(t, client, beta)

	verifyRestoringMarkerCleared(t, ts)

	restoretest.DriveSignalHydrate(t, client, stateDir, []string{alpha.name, beta.name})

	restoretest.WaitForSkeletonMarkersCleared(t, client, restoretest.HydrateBudget, restoretest.HydrateTick)

	for _, fx := range []fixtureSession{alpha, beta} {
		for w := range 2 {
			for p := range 2 {
				verifyANSIInScrollback(t, ts, fx.name, w, p, scrollbackFixtures[state.SanitizePaneKey(fx.name, w, p)])
			}
		}
	}
}

func createFullTopology(t *testing.T, ts *tmuxtest.Socket, fx fixtureSession) {
	t.Helper()
	ts.Run(t, "new-session", "-d", "-s", fx.name, "-c", fx.cwds[0][0], "sleep", "infinity")
	ts.WaitForSession(t, fx.name, 2*time.Second)

	ts.Run(t, "set-environment", "-t", fx.name, fx.envKey, fx.envValue)

	ts.Run(t, "split-window", "-t", fx.name+":0", "-c", fx.cwds[0][1], "sleep", "infinity")

	ts.Run(t, "new-window", "-t", fx.name, "-c", fx.cwds[1][0], "sleep", "infinity")

	ts.Run(t, "split-window", "-t", fx.name+":1", "-c", fx.cwds[1][1], "sleep", "infinity")

	zoomTarget := tmux.PaneTarget(fx.name, fx.zoomedW, fx.zoomedP)
	ts.Run(t, "resize-pane", "-t", zoomTarget, "-Z")

	for w, ap := range fx.activePanes {
		ts.Run(t, "select-pane", "-t", tmux.PaneTarget(fx.name, w, ap))
	}

	ts.Run(t, "select-window", "-t", fmt.Sprintf("%s:%d", fx.name, fx.activeWin))
}

func ansiFixtureBytes(session string, window, pane int) []byte {
	return []byte(fmt.Sprintf(
		"\x1b[31m[fixture %s w%d p%d]\x1b[0m\nbefore-reboot-payload\n",
		session, window, pane,
	))
}

func verifyTopologyShape(t *testing.T, idx state.Index, alpha, beta fixtureSession) {
	t.Helper()
	if got := len(idx.Sessions); got != 2 {
		t.Fatalf("captured %d sessions; want 2 (idx=%+v)", got, idx)
	}
	if idx.Sessions[0].Name != alpha.name || idx.Sessions[1].Name != beta.name {
		t.Fatalf("session names = [%s, %s]; want [%s, %s]",
			idx.Sessions[0].Name, idx.Sessions[1].Name, alpha.name, beta.name)
	}
	for _, fx := range []struct {
		idxOf int
		f     fixtureSession
	}{{0, alpha}, {1, beta}} {
		s := idx.Sessions[fx.idxOf]
		if got := len(s.Windows); got != 2 {
			t.Fatalf("%s windows = %d; want 2", s.Name, got)
		}
		for w := range 2 {
			if got := len(s.Windows[w].Panes); got != 2 {
				t.Fatalf("%s w%d panes = %d; want 2", s.Name, w, got)
			}
		}
		if !s.Windows[fx.f.zoomedW].Zoomed {
			t.Errorf("%s w%d not zoomed in capture; want zoomed=true", s.Name, fx.f.zoomedW)
		}
		if got := s.Environment[fx.f.envKey]; got != fx.f.envValue {
			t.Errorf("%s env[%s] = %q; want %q", s.Name, fx.f.envKey, got, fx.f.envValue)
		}
		for w := range 2 {
			ap := fx.f.activePanes[w]
			if !s.Windows[w].Panes[ap].Active {
				t.Errorf("%s w%d p%d should be active in capture; got Active=false (panes=%+v)",
					s.Name, w, ap, s.Windows[w].Panes)
			}
		}
	}
}

func verifyLiveStructure(t *testing.T, ts *tmuxtest.Socket, sessions ...fixtureSession) {
	t.Helper()
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, fx := range sessions {
		if !strings.Contains(out, fx.name) {
			t.Errorf("session %q missing post-restore; got %q", fx.name, out)
		}
		panes := restoretest.LivePaneCoords(t, ts, fx.name)
		for w := range 2 {
			for p := range 2 {
				want := fmt.Sprintf("%d:%d", w, p)
				if !slices.Contains(panes, want) {
					t.Errorf("%s live pane %q missing; got %q", fx.name, want, panes)
				}
			}
		}
	}
}

func verifyZoomFlags(t *testing.T, ts *tmuxtest.Socket, sessions ...fixtureSession) {
	t.Helper()
	for _, fx := range sessions {
		for w := range 2 {
			got := strings.TrimSpace(ts.Run(t, "display-message", "-p",
				"-t", fmt.Sprintf("%s:%d", fx.name, w),
				"#{window_zoomed_flag}"))
			want := "0"
			if w == fx.zoomedW {
				want = "1"
			}
			if got != want {
				t.Errorf("%s:%d window_zoomed_flag = %q; want %q", fx.name, w, got, want)
			}
		}
	}
}

func verifyActivePanes(t *testing.T, ts *tmuxtest.Socket, sessions ...fixtureSession) {
	t.Helper()
	for _, fx := range sessions {
		for w := range 2 {
			ap := fx.activePanes[w]
			got := strings.TrimSpace(ts.Run(t, "display-message", "-p",
				"-t", tmux.PaneTarget(fx.name, w, ap),
				"#{pane_active}"))
			if got != "1" {
				t.Errorf("%s w%d p%d pane_active = %q; want 1", fx.name, w, ap, got)
			}
		}
	}
}

func verifySessionEnv(t *testing.T, client *tmux.Client, fx fixtureSession) {
	t.Helper()
	out, err := client.ShowEnvironment(fx.name)
	if err != nil {
		t.Fatalf("ShowEnvironment %q: %v", fx.name, err)
	}
	wantLine := fx.envKey + "=" + fx.envValue
	if !strings.Contains(out, wantLine) {
		t.Errorf("session %q env missing %q; got:\n%s", fx.name, wantLine, out)
	}
}

func verifyRestoringMarkerCleared(t *testing.T, ts *tmuxtest.Socket) {
	t.Helper()
	out, err := ts.TryRun("show-options", "-sv", state.RestoringMarkerName)
	if err == nil && strings.TrimSpace(out) != "" {
		t.Errorf("%s should be unset after restoreWithMarker; got %q",
			state.RestoringMarkerName, out)
	}
}

func verifyANSIInScrollback(t *testing.T, ts *tmuxtest.Socket, session string, win, pane int, fixtureBytes []byte) {
	t.Helper()
	target := tmux.PaneTarget(session, win, pane)
	out := ts.Run(t, "capture-pane", "-e", "-p", "-S", "-", "-t", target)

	label := fmt.Sprintf("[fixture %s w%d p%d]", session, win, pane)

	if !strings.Contains(out, "\x1b[31m") {
		t.Errorf("scrollback for %s missing red SGR open (%q); fixture=%q got=%q",
			target, "\x1b[31m", fixtureBytes, out)
	}
	if !strings.Contains(out, label) {
		t.Errorf("scrollback for %s missing per-pane label (%q); fixture=%q got=%q",
			target, label, fixtureBytes, out)
	}
	if !strings.Contains(out, "\x1b[0m") && !strings.Contains(out, "\x1b[39m") {
		t.Errorf("scrollback for %s missing SGR reset (neither %q nor %q); fixture=%q got=%q",
			target, "\x1b[0m", "\x1b[39m", fixtureBytes, out)
	}
	if !strings.Contains(out, "before-reboot-payload") {
		t.Errorf("scrollback for %s missing post-SGR payload (%q); fixture=%q got=%q",
			target, "before-reboot-payload", fixtureBytes, out)
	}
}
