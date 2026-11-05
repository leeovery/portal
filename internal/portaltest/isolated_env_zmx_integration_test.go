//go:build integration

package portaltest_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/zmxtest"
)

// shellSettle bounds each wait on the interactive shell: attaching, answering
// its input and exiting.
const shellSettle = 5 * time.Second

// The system zsh startup file assigns the history path from the live shell's
// own environment, so only a real interactive shell in a session exercises the
// redirect the isolated env installs.
func TestIsolatedHomeAfterAnInteractiveShellAgainstZmx(t *testing.T) {
	const shell = "/bin/zsh"
	if _, err := os.Stat(shell); err != nil {
		t.Skipf("%s unavailable: %v", shell, err)
	}

	env, stateDir := portaltest.IsolateStateForTest(t)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)
	home := os.Getenv("HOME")

	b := zmxtest.New(t, zmxtest.WithEnv(env...))
	const name = "shellprobe"
	if _, err := b.Create(t.Context(), name, t.TempDir(), []string{shell, "-i"}); err != nil {
		t.Fatalf("create %s running %s -i: %v", name, shell, err)
	}
	c := zmxtest.Attach(t, b, name)
	if !harnesstest.PollUntil(t, shellSettle, 20*time.Millisecond, func() bool {
		s, err := b.Session(t.Context(), name)
		return err == nil && s.Clients == 1
	}) {
		t.Fatal("the client never attached")
	}

	// The shell computes what it prints, so the typed line alone never
	// answers the wait: the shell must have read the input and run it.
	c.Type(t, "echo isolated-home-$((6*7))\r")
	awaitShown(t, b, name, "isolated-home-42")
	c.Type(t, "exit\r")
	awaitSessionGone(t, b, name)

	t.Run("it leaves the temp HOME empty after an interactive shell exits", func(t *testing.T) {
		entries, err := os.ReadDir(home)
		if err != nil {
			t.Fatalf("read isolated HOME %s: %v", home, err)
		}
		for _, e := range entries {
			t.Errorf("shell wrote %s into the isolated HOME %s, which the framework is about to remove", e.Name(), home)
		}
	})
}

func awaitShown(t *testing.T, b *zmx.Backend, name, want string) {
	t.Helper()
	var shown []byte
	if !harnesstest.PollUntil(t, shellSettle, 50*time.Millisecond, func() bool {
		history, err := b.History(t.Context(), name, 0)
		shown = history
		return err == nil && strings.Contains(string(history), want)
	}) {
		t.Fatalf("session %s never showed %q; its history is %q", name, want, shown)
	}
}

// awaitSessionGone waits for the shell's exit to end the session: zsh writes
// its history file as it exits, so once the session is gone every write the
// shell makes is done.
func awaitSessionGone(t *testing.T, b *zmx.Backend, name string) {
	t.Helper()
	if !harnesstest.PollUntil(t, shellSettle, 50*time.Millisecond, func() bool {
		sessions, err := b.List(t.Context())
		return err == nil && !slices.ContainsFunc(sessions, func(s backend.Session) bool { return s.Name == name })
	}) {
		t.Fatalf("session %s still listed; its shell never exited", name)
	}
}
