package portaltest_test

import (
	"os"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// The system zsh startup file assigns the history path from the live shell's
// own environment, so only a real interactive pane exercises the redirect the
// isolated env installs.
func TestIsolatedHomeAfterAnInteractiveShellPane(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	const shell = "/bin/zsh"
	if _, err := os.Stat(shell); err != nil {
		t.Skipf("%s unavailable: %v", shell, err)
	}

	_, stateDir := portaltest.IsolateStateForTest(t)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	home := os.Getenv("HOME")

	ts := tmuxtest.New(t, "ptl-isolated-home-")
	ts.Run(t, "new-session", "-d", "-s", "shellprobe", shell, "-i")
	ts.WaitForSession(t, "shellprobe", 3*time.Second)

	target := tmux.CoordTargetExact("shellprobe")
	ts.SendKeys(t, target, "echo isolated-home-probe")
	ts.SendKeys(t, target, "exit")
	awaitSessionGone(t, ts, target)

	ts.KillServer()
	portaltest.AwaitTmuxServerGone(t, ts.SocketPath())

	t.Run("it leaves the temp HOME empty after an interactive shell pane exits", func(t *testing.T) {
		entries, err := os.ReadDir(home)
		if err != nil {
			t.Fatalf("read isolated HOME %s: %v", home, err)
		}
		for _, e := range entries {
			t.Errorf("shell wrote %s into the isolated HOME %s, which the framework is about to remove", e.Name(), home)
		}
	})
}

func awaitSessionGone(t *testing.T, ts *tmuxtest.Socket, target tmux.Target) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := ts.TryRun("has-session", "-t", string(target)); err != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("session %s still answering; its shell never exited", target)
}
