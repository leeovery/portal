package migrate_test

import (
	"context"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/migrate"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// userHooks are hooks of the user's and another tool's, on events Portal's
// share, pane-focus-out among them, which tmux's whole-scope read doesn't
// list.
var userHooks = []tmuxtest.TmuxVersionHook{
	{Event: "session-created", Command: `run-shell "true # the user's"`},
	{Event: "pane-focus-out", Command: `display-message "focus left"`},
	{Event: "client-attached", Command: `run-shell "true # another tool's"`},
}

// olderPortalHooks are Portal's hooks as older tmux versions left them: a
// stacked duplicate, the signal-hydrate body before its -- separator and the
// migrate-rename hook no version registers any more.
var olderPortalHooks = []tmuxtest.TmuxVersionHook{
	{Event: "window-linked", Command: `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`},
	{Event: "client-session-changed", Command: `run-shell "command -v portal >/dev/null 2>&1 && portal state signal-hydrate #{session_name}"`},
	{Event: "session-renamed", Command: `run-shell "command -v portal >/dev/null 2>&1 && portal state migrate-rename"`},
}

// portalTmux is a tmux server holding no portal on its PATH, with the tmux
// version's hooks registered on it, and with them the user's and older
// versions' of Portal's.
func portalTmux(t *testing.T) (*tmuxtest.Socket, migrate.Tmux) {
	t.Helper()
	tmuxtest.SkipIfNoTmux(t)
	ts := tmuxtest.New(t, "ptl-")
	ts.StartPortalFree(t)
	ts.RegisterTmuxVersionHooks(t)
	for _, h := range append(userHooks, olderPortalHooks...) {
		ts.Run(t, "set-hook", "-ga", h.Event, h.Command)
	}
	return ts, migrate.Tmux{Socket: ts.SocketPath()}
}

// hooksOn is every hook's command on event, in index order.
func hooksOn(t *testing.T, ts *tmuxtest.Socket, event string) []string {
	t.Helper()
	var commands []string
	for line := range strings.Lines(ts.Run(t, "show-hooks", "-g", event)) {
		_, command, ok := strings.Cut(strings.TrimSpace(line), "] ")
		if ok {
			commands = append(commands, command)
		}
	}
	return commands
}

func TestTmux_Answering(t *testing.T) {
	t.Run("it reads a running server as answering", func(t *testing.T) {
		_, tmux := portalTmux(t)

		if !tmux.Answering(context.Background()) {
			t.Error("Answering = false for a running server")
		}
	})

	t.Run("it reads a socket no server holds as not answering", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)
		ts := tmuxtest.New(t, "ptl-")

		if (migrate.Tmux{Socket: ts.SocketPath()}).Answering(context.Background()) {
			t.Error("Answering = true with no server")
		}
	})
}

func TestTmux_RemoveHooks(t *testing.T) {
	t.Run("it unsets every Portal hook on each event and no other", func(t *testing.T) {
		ts, tmux := portalTmux(t)

		unset, err := tmux.RemoveHooks(context.Background())
		if err != nil {
			t.Fatalf("RemoveHooks: %v", err)
		}

		if want := len(tmuxtest.TmuxVersionHooks) + len(olderPortalHooks); unset != want {
			t.Errorf("unset %d hooks, want %d", unset, want)
		}
		left := map[string][]string{}
		for _, h := range tmuxtest.TmuxVersionHooks {
			left[h.Event] = hooksOn(t, ts, h.Event)
		}
		for _, h := range tmuxtest.TmuxVersionHooks {
			for _, command := range left[h.Event] {
				if strings.Contains(command, "portal state") {
					t.Errorf("%s still holds Portal's %s", h.Event, command)
				}
			}
		}
		for _, h := range userHooks {
			if got := left[h.Event]; len(got) != 1 || got[0] != h.Command {
				t.Errorf("%s holds %q, want the user's %q alone", h.Event, got, h.Command)
			}
		}
	})

	t.Run("it unsets nothing on a server holding no Portal hook", func(t *testing.T) {
		ts, tmux := portalTmux(t)
		if _, err := tmux.RemoveHooks(context.Background()); err != nil {
			t.Fatalf("RemoveHooks: %v", err)
		}

		unset, err := tmux.RemoveHooks(context.Background())

		if unset != 0 || err != nil {
			t.Errorf("a second RemoveHooks = %d, %v; want 0, nil", unset, err)
		}
		if got := hooksOn(t, ts, "pane-focus-out"); len(got) != 1 {
			t.Errorf("pane-focus-out holds %q, want the user's hook still", got)
		}
	})

	t.Run("it fails, saying what tmux said, when no server answers", func(t *testing.T) {
		tmuxtest.SkipIfNoTmux(t)
		ts := tmuxtest.New(t, "ptl-")

		_, err := (migrate.Tmux{Socket: ts.SocketPath()}).RemoveHooks(context.Background())

		if err == nil || !strings.Contains(err.Error(), "show-hooks -g session-created") {
			t.Errorf("RemoveHooks = %v; want a failure naming the read", err)
		}
	})
}

func TestTmux_KillSaver(t *testing.T) {
	t.Run("it kills _portal-saver by its exact name alone", func(t *testing.T) {
		ts, tmux := portalTmux(t)
		ts.Run(t, "new-session", "-d", "-s", "_portal-saver", "exec sleep 600")
		ts.Run(t, "new-session", "-d", "-s", "_portal-saver-2", "exec sleep 600")

		killed, err := tmux.KillSaver(context.Background())

		if !killed || err != nil {
			t.Fatalf("KillSaver = %v, %v; want true, nil", killed, err)
		}
		if ts.HasSession("_portal-saver") {
			t.Error("_portal-saver is still there")
		}
		for _, kept := range []string{"_portal-saver-2", "_portal-bootstrap"} {
			if !ts.HasSession(kept) {
				t.Errorf("%s was killed too", kept)
			}
		}
	})

	t.Run("it kills nothing, not even a prefix sibling, when there is no saver", func(t *testing.T) {
		ts, tmux := portalTmux(t)
		ts.Run(t, "new-session", "-d", "-s", "_portal-saver-2", "exec sleep 600")

		killed, err := tmux.KillSaver(context.Background())

		if killed || err != nil {
			t.Fatalf("KillSaver = %v, %v; want false, nil", killed, err)
		}
		if !ts.HasSession("_portal-saver-2") {
			t.Error("_portal-saver-2 was killed")
		}
	})
}
