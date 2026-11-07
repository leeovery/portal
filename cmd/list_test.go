package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

// stageList stages fake as the backend and tty as whether stdout is a
// terminal, in a state directory of the test's own, owing no bootstrap.
func stageList(t *testing.T, fake *backendtest.Fake, tty bool) string {
	t.Helper()
	withBootstrapDeps(t, BootstrapDeps{})
	withBackendDeps(t, BackendDeps{Backend: fake})
	withListDeps(t, ListDeps{IsTTY: func() bool { return tty }})
	return isolateStateDir(t)
}

// runList executes list with args and returns its stdout, failing on an error.
func runList(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, err := runRootCmd(t, append([]string{"list"}, args...)...)
	if err != nil {
		t.Fatalf("list %v: %v\nstderr: %s", args, err, stderr)
	}
	return stdout.String()
}

func assertListed(t *testing.T, fake *backendtest.Fake) {
	t.Helper()
	if got := len(fake.CallsTo(backendtest.OpList)); got != 1 {
		t.Errorf("backend List calls = %d, want 1: %+v", got, fake.Calls())
	}
}

func TestListCommand(t *testing.T) {
	twoSessions := func() *backendtest.Fake {
		return backendtest.New(
			backend.Session{Name: "claude-lab", Dir: "/work/lab", Created: joinCreated},
			backend.Session{Name: "flowx-dev", Dir: "/work/flowx", Clients: 2, Created: joinCreated},
		)
	}

	t.Run("it prints names only when piped", func(t *testing.T) {
		fake := twoSessions()
		stageList(t, fake, false)

		if got, want := runList(t), "claude-lab\nflowx-dev\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
		assertListed(t, fake)
	})

	t.Run("it prints label, attached state and dir on a terminal", func(t *testing.T) {
		fake := twoSessions()
		stateDir := stageList(t, fake, true)
		flowx, _ := fake.Held("flowx-dev")
		labelSession(t, stateDir, flowx, "flowx · api work")

		want := "claude-lab    claude-lab    detached    /work/lab\n" +
			"flowx-dev    flowx · api work    attached    /work/flowx\n"
		if got := runList(t); got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it prints names only with --short on a terminal", func(t *testing.T) {
		stageList(t, twoSessions(), true)

		if got, want := runList(t, "--short"), "claude-lab\nflowx-dev\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it prints the long form with --long when piped", func(t *testing.T) {
		stageList(t, twoSessions(), false)

		want := "claude-lab    claude-lab    detached    /work/lab\n" +
			"flowx-dev    flowx-dev    attached    /work/flowx\n"
		if got := runList(t, "--long"); got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it hides Portal's own sessions", func(t *testing.T) {
		fake := backendtest.New(
			backend.Session{Name: "_portal-saver", Dir: "/"},
			backend.Session{Name: "dotfiles-PpaTER", Dir: "/work/dotfiles"},
		)
		stageList(t, fake, false)

		if got, want := runList(t), "dotfiles-PpaTER\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
		assertListed(t, fake)
	})

	t.Run("it lists a session that does not answer by its name, marked not answering in the long form", func(t *testing.T) {
		fake := twoSessions()
		lab, _ := fake.Held("claude-lab")
		fake.StopAnswering("claude-lab")
		stateDir := stageList(t, fake, false)
		labelSession(t, stateDir, lab, "lab")

		if got, want := runList(t, "--short"), "claude-lab\nflowx-dev\n"; got != want {
			t.Errorf("--short output = %q, want %q", got, want)
		}
		want := "claude-lab    not answering\nflowx-dev    flowx-dev    attached    /work/flowx\n"
		if got := runList(t, "--long"); got != want {
			t.Errorf("--long output = %q, want %q", got, want)
		}
	})

	t.Run("it drops only the labels on a terminal when the index cannot be read", func(t *testing.T) {
		stageList(t, twoSessions(), true)
		blockStateDir(t)

		want := "claude-lab    claude-lab    detached    /work/lab\n" +
			"flowx-dev    flowx-dev    attached    /work/flowx\n"
		if got := runList(t); got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it fails --long when the index cannot be read", func(t *testing.T) {
		stageList(t, twoSessions(), true)
		blockStateDir(t)

		if _, _, err := runRootCmd(t, "list", "--long"); err == nil || !strings.Contains(err.Error(), "sessions.json") {
			t.Errorf("error = %v, want the index read failure", err)
		}
	})

	t.Run("it leaves the dir column off a session whose dir is unknown", func(t *testing.T) {
		stageList(t, backendtest.New(backend.Session{Name: "remote-x1"}), false)

		if got, want := runList(t, "--long"), "remote-x1    remote-x1    detached\n"; got != want {
			t.Errorf("output = %q, want %q", got, want)
		}
	})

	t.Run("it surfaces a listing error rather than printing an empty list", func(t *testing.T) {
		fake := twoSessions()
		sentinel := errors.New("zmx dir unreadable")
		fake.Fail(backendtest.OpList, sentinel)
		stageList(t, fake, false)

		stdout, _, err := runRootCmd(t, "list")
		if !errors.Is(err, sentinel) {
			t.Errorf("error = %v, want %v", err, sentinel)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout.String())
		}
	})

	t.Run("it prints nothing and succeeds with no sessions", func(t *testing.T) {
		fake := backendtest.New()
		stageList(t, fake, true)

		if got := runList(t); got != "" {
			t.Errorf("output = %q, want empty", got)
		}
		assertListed(t, fake)
	})

	t.Run("it refuses --short with --long", func(t *testing.T) {
		fake := twoSessions()
		stageList(t, fake, true)

		if _, _, err := runRootCmd(t, "list", "--short", "--long"); err == nil {
			t.Fatal("expected an error for --short with --long, got nil")
		}
		if calls := fake.Calls(); len(calls) != 0 {
			t.Errorf("backend calls = %+v, want none", calls)
		}
	})
}

// The bootstrap's first step needs the binary; once it is owed none, list
// itself reads zmx's sockets alone.
func TestListCommand_DoesNotNeedTheZmxBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("ZMX_DIR", t.TempDir())
	withBootstrapDeps(t, BootstrapDeps{})
	withListDeps(t, ListDeps{IsTTY: func() bool { return false }})

	stdout, _, err := runRootCmd(t, "list")
	if err != nil {
		t.Fatalf("list with no zmx on PATH: %v", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing from the empty zmx dir", stdout.String())
	}
}
