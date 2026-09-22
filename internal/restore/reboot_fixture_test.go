//go:build integration

package restore_test

import (
	"path/filepath"
	"strconv"
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

// rebootPane is one pane of the arranged session, described by the two things
// the suites arranging it differ in: the durable token it is stamped with, and
// the resume hook registered against that token, pinned eager so the restore
// runs it rather than offering it on a panel. Both are optional — an empty
// token leaves the pane un-stamped, which is the legacy shape a restore must
// still land on a bare shell.
type rebootPane struct {
	token   string
	hookCmd string
}

// rebootFixture is a live single-window session of len(panes) panes, with the
// state, hooks and tmux socket the reboot will run against already isolated.
type rebootFixture struct {
	ts     *tmuxtest.Socket
	client *tmux.Client

	stateDir string
	binDir   string

	hooksPath string
	panes     []rebootPane
}

// newRebootFixture builds the live session the rename-reboot and multipane
// suites arrange against. What they vary — how many panes, and which of those
// carry a token and a hook — arrives as parameters.
func newRebootFixture(t *testing.T, socketPrefix, sessionName string, panes []rebootPane) *rebootFixture {
	t.Helper()

	if len(panes) == 0 {
		t.Fatal("newRebootFixture: no panes; a session has at least the one it is created with")
	}

	fx := &rebootFixture{
		binDir: restoretest.BuildPortalBinaryDir(t),
		panes:  panes,
	}

	_, fx.stateDir = portaltest.IsolateStateForTest(t)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	fx.hooksPath = filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", fx.hooksPath)

	store := hooks.NewStore(fx.hooksPath)
	for i, p := range panes {
		if p.hookCmd == "" {
			continue
		}
		if err := store.Set(p.token, "on-resume", hooks.Registration{Command: p.hookCmd, Resume: resumemode.Eager}, hooks.ViaCLI); err != nil {
			t.Fatalf("hooks.Set pane %d: %v", i, err)
		}
		verifyHookKeyed(t, fx.hooksPath, p.token)
	}

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, fx.stateDir)

	fx.ts = tmuxtest.New(t, socketPrefix)
	fx.client = fx.ts.Client()

	cwd := t.TempDir()
	fx.ts.Run(t, "new-session", "-d", "-s", sessionName, "-c", cwd, "sleep", "infinity")
	fx.ts.WaitForSession(t, sessionName, 2*time.Second)
	for range panes[1:] {
		fx.ts.Run(t, "split-window", "-t", tmux.PaneTarget(sessionName, 0, 0), "-c", cwd, "sleep", "infinity")
	}
	fx.assertLivePanes(t, sessionName)

	for i, p := range panes {
		if p.token == "" {
			continue
		}
		fx.ts.StampPaneToken(t, tmux.PaneTargetExact(sessionName, 0, i), p.token)
	}

	return fx
}

// assertLivePanes pins the pane coordinates the arrange produced, so a fixture
// that silently built the wrong topology fails here rather than as a puzzling
// assertion after the reboot.
func (fx *rebootFixture) assertLivePanes(t *testing.T, sessionName string) {
	t.Helper()
	want := make([]string, 0, len(fx.panes))
	for i := range fx.panes {
		want = append(want, "0:"+strconv.Itoa(i))
	}
	got := restoretest.LivePaneCoords(t, fx.ts, sessionName)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("session %q live panes = %v; want %v", sessionName, got, want)
	}
}

// captureAndPersist saves the live topology as the index the reboot restores
// from, having first checked each pane's token survived into the capture: a
// capture that lost one would restore a hookless pane and prove nothing.
func (fx *rebootFixture) captureAndPersist(t *testing.T, sessionName string) {
	t.Helper()

	idx, _, err := state.CaptureStructure(fx.client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	sess := restoretest.FindCapturedSession(t, idx, sessionName)
	if len(sess.Windows) != 1 || len(sess.Windows[0].Panes) != len(fx.panes) {
		t.Fatalf("captured session %q topology = %d window(s) / %v panes; want 1 window / %d panes",
			sessionName, len(sess.Windows), paneIndices(sess), len(fx.panes))
	}
	for i, p := range fx.panes {
		if got := sess.Windows[0].Panes[i].PortalPaneID; got != p.token {
			t.Fatalf("captured session %q pane %d token = %q; want %q", sessionName, i, got, p.token)
		}
	}

	fx.persist(t, idx, sessionName)
}

// persist writes the index and the scrollback a restore of it will replay.
func (fx *rebootFixture) persist(t *testing.T, idx state.Index, sessionName string) {
	t.Helper()
	for i := range fx.panes {
		restoretest.SeedScrollback(t, fx.stateDir, sessionName, 0, i, []byte(restoretest.ANSIScrollback))
	}
	restoretest.WriteIndex(t, fx.stateDir, idx)
}

// rebootAndHydrate takes the fixture through one whole reboot cycle: the server
// is killed, the saved index restored onto a fresh one, and every restored pane
// driven through its hydrate helper to the point the helper hands over to the
// hook or the shell.
func (fx *rebootFixture) rebootAndHydrate(t *testing.T, sessionName string) error {
	t.Helper()

	restoretest.RebootServer(t, fx.ts, fx.client)
	if err := restoretest.RestoreFromState(t, fx.client, fx.stateDir, fx.binDir); err != nil {
		return err
	}
	fx.assertLivePanes(t, sessionName)

	restoretest.DriveSignalHydrate(t, fx.client, fx.stateDir, []string{sessionName})
	restoretest.WaitForSkeletonMarkersCleared(t, fx.client, restoretest.HydrateBudget, restoretest.HydrateTick)
	return nil
}

func paneIndices(sess state.Session) [][]int {
	var out [][]int
	for _, w := range sess.Windows {
		var panes []int
		for _, p := range w.Panes {
			panes = append(panes, p.Index)
		}
		out = append(out, panes)
	}
	return out
}
