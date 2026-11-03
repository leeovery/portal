package tmux_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/shellquote"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// A period-bearing session name is legal (ValidateSessionName accepts it) and
// user-reachable — a rename in the picker is enough to mint one. tmux splits a
// colon-free target on "." into window and pane before it ever tries a session
// lookup, so "=my.app" is read as window "my" of an unnamed session and the
// session lookup is a fallback any live prefix-extension of "my" displaces.
// Measured on tmux 3.7c against isolated -S sockets, with periodSibling live:
// has-session, kill-session, rename-session, switch-client, show-environment,
// set-environment, list-clients and attach-session all fail on "=my.app" and all
// resolve correctly on "=my.app:" — which is why every one of those sites takes
// CoordTargetExact. The pinning survives the move: "=foo:" still misses while
// only "foo-2" is live.
const (
	periodSession = "my.app"
	// periodSibling prefix-extends "my", the pre-period component of
	// periodSession — the live stranger tmux reaches instead.
	periodSibling = "my-cool-app-abc123"
)

var periodFixture = realTmuxFixture{
	socketPrefix: "ptl-periodtgt-",
	sessions:     []string{periodSession, periodSibling},
}

func TestPeriodBearingSessionTargets(t *testing.T) {
	t.Run("it still accepts a period-bearing name as addressable", func(t *testing.T) {
		_, client, _ := seedRealTmuxServer(t, periodFixture)

		if err := tmux.ValidateSessionName(periodSession); err != nil {
			t.Fatalf("ValidateSessionName(%q) = %v; want nil", periodSession, err)
		}
		if !client.HasSession(periodSession) {
			t.Errorf("HasSession(%q) = false; the live session is unreachable through its own target form", periodSession)
		}
		present, err := client.HasSessionProbe(periodSession)
		if !present || err != nil {
			t.Errorf("HasSessionProbe(%q) = (%t, %v); want (true, nil)", periodSession, present, err)
		}
		// The pinning the exact forms exist for: a name no session carries must
		// still miss, rather than reaching a live prefix extension of itself.
		if client.HasSession("my") {
			t.Errorf(`HasSession("my") = true; the target reached a live prefix sibling`)
		}
	})

	t.Run("it kills the period-bearing session and not its prefix-extension sibling", func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, periodFixture)

		if err := client.KillSession(periodSession); err != nil {
			t.Fatalf("KillSession(%q): %v", periodSession, err)
		}

		live := liveSessionNames(t, ts)
		if slices.Contains(live, periodSession) {
			t.Errorf("%q is still live after KillSession: %v", periodSession, live)
		}
		if !slices.Contains(live, periodSibling) {
			t.Errorf("KillSession(%q) destroyed the live %q session: %v", periodSession, periodSibling, live)
		}
	})

	t.Run("it reads a period-bearing session's environment while a longer prefix sibling is live", func(t *testing.T) {
		_, client, _ := seedRealTmuxServer(t, periodFixture)

		if err := client.SetSessionEnvironment(periodSession, "PORTAL_PERIOD", "period-only"); err != nil {
			t.Fatalf("SetSessionEnvironment(%q): %v", periodSession, err)
		}

		got, err := client.ShowEnvironment(periodSession)
		if err != nil {
			t.Fatalf("ShowEnvironment(%q): %v", periodSession, err)
		}
		if !strings.Contains(got, "PORTAL_PERIOD=period-only") {
			t.Errorf("ShowEnvironment(%q) = %q, want it to carry PORTAL_PERIOD=period-only", periodSession, got)
		}

		sibling, err := client.ShowEnvironment(periodSibling)
		if err != nil {
			t.Fatalf("ShowEnvironment(%q): %v", periodSibling, err)
		}
		if strings.Contains(sibling, "PORTAL_PERIOD") {
			t.Errorf("the write landed on the live %q session:\n%s", periodSibling, sibling)
		}
	})

	t.Run("it switches to a period-bearing session with a prefix sibling live", func(t *testing.T) {
		ts, client, _ := seedRealTmuxServer(t, periodFixture)
		attachClient(t, ts, periodSibling)

		if err := client.SwitchClient(periodSession); err != nil {
			t.Fatalf("SwitchClient(%q): %v", periodSession, err)
		}
		if !harnesstest.PollUntil(t, sessionSettleTimeout, 20*time.Millisecond, func() bool {
			return attachedSession(t, ts) == periodSession
		}) {
			t.Errorf("the attached client is on %q, want %q", attachedSession(t, ts), periodSession)
		}

		// list-clients composes the same target kind, so the client the switch
		// moved must be enumerable under the session it reached and absent from
		// the sibling it left.
		on, err := client.ListClients(periodSession)
		if err != nil || len(on) != 1 {
			t.Errorf("ListClients(%q) = %v, %v; want the one attached client", periodSession, on, err)
		}
		off, err := client.ListClients(periodSibling)
		if err != nil || len(off) != 0 {
			t.Errorf("ListClients(%q) = %v, %v; want no clients", periodSibling, off, err)
		}
	})
}

// attachClient gives ts a real tmux client attached to session, by running the
// attach as the pane command of a second isolated server: switch-client and
// list-clients have nothing to resolve without one, and a client is the only
// way to tell a target that resolved from one that could not.
func attachClient(t *testing.T, ts *tmuxtest.Socket, session string) {
	t.Helper()

	host := tmuxtest.New(t, "ptl-periodhost-")
	attach := strings.Join([]string{
		"tmux", "-S", shellquote.Single(ts.SocketPath()), "-f", "/dev/null",
		"attach-session", "-t", shellquote.Single(string(tmux.CoordTargetExact(session))),
	}, " ")
	host.Run(t, "new-session", "-d", "-s", "host", attach)

	if !harnesstest.PollUntil(t, sessionSettleTimeout, 20*time.Millisecond, func() bool {
		return attachedSession(t, ts) == session
	}) {
		t.Fatalf("no client attached to %q within %s", session, sessionSettleTimeout)
	}
}

// attachedSession names the session the first listed client is attached to, or
// "" while no client is attached.
func attachedSession(t *testing.T, ts *tmuxtest.Socket) string {
	t.Helper()
	out, err := ts.TryRun("list-clients", "-F", "#{session_name}")
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	return first
}

func liveSessionNames(t *testing.T, ts *tmuxtest.Socket) []string {
	t.Helper()
	out, err := ts.TryRun("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
