package tmux_test

import (
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

// The exact target form every route below composes, and why it is the
// coordinate one: tmux only honours the "=" exact-match prefix where it parses
// the target as a session. A command whose -t is a window or pane target parses
// a bare "=foo" as a window/pane name and falls through to its own fuzzy lookup,
// prefix-matching anyway — and a colon-free "=my.app" is split on the period
// into window and pane before the session lookup is even tried, so a live
// prefix extension of "my" displaces it. The trailing ":" is what forces the
// leading component to be read as a session name. Measured per command on tmux
// 3.7c, every route Portal runs against one session parses a window-or-pane
// target, set-option (a session-option write reached through a pane target)
// included, so every route here takes the coordinate form.
const coordTargetForm = "=exact:"

// exactTargetSession is the session name every route in perSessionRoutes is
// invoked against; the coordinate form above is its exact rendering.
const exactTargetSession = "exact"

// perSessionRoute is one operation that composes an exact tmux target for a
// single session.
type perSessionRoute struct {
	name    string
	command string
	want    string
	invoke  func(*tmux.Client)
}

// perSessionRoutes is the route set shared by both guards over the composition
// rule: the mock-level target-form table below and the real-tmux prefix-sibling
// routes. A route added here is unguarded until both enumerate it, so the two
// cannot drift apart. It is a maintained set, not the client's whole
// exact-target surface.
var perSessionRoutes = []perSessionRoute{
	{
		name:    "ShowEnvironment",
		command: "show-environment",
		want:    coordTargetForm,
		invoke:  func(c *tmux.Client) { _, _ = c.ShowEnvironment(exactTargetSession) },
	},
	{
		name:    "SetSessionEnvironment",
		command: "set-environment",
		want:    coordTargetForm,
		invoke:  func(c *tmux.Client) { _ = c.SetSessionEnvironment(exactTargetSession, "K", "v") },
	},
	{
		name:    "SetSessionOption",
		command: "set-option",
		want:    coordTargetForm,
		invoke:  func(c *tmux.Client) { _ = c.SetSessionOption(exactTargetSession, "@k", "v") },
	},
	{
		name:    "ActivePaneCurrentPath",
		command: "display-message",
		want:    coordTargetForm,
		invoke:  func(c *tmux.Client) { _, _ = c.ActivePaneCurrentPath(exactTargetSession) },
	},
	{
		name:    "ListPanesInSession",
		command: "list-panes",
		want:    coordTargetForm,
		invoke:  func(c *tmux.Client) { _, _ = c.ListPanesInSession(exactTargetSession) },
	},
	{
		name:    "ListWindowsAndPanesInSession",
		command: "list-panes",
		want:    coordTargetForm,
		invoke:  func(c *tmux.Client) { _, _ = c.ListWindowsAndPanesInSession(exactTargetSession) },
	},
	{
		name:    "SaverPaneID",
		command: "list-panes",
		want:    coordTargetForm,
		invoke:  func(c *tmux.Client) { _, _ = c.SaverPaneID(exactTargetSession) },
	},
	{
		name:    "SaverPanePIDOrAbsent",
		command: "list-panes",
		want:    coordTargetForm,
		invoke:  func(c *tmux.Client) { _, _, _ = tmux.SaverPanePIDOrAbsent(c, exactTargetSession) },
	},
}

func targetArgOf(t *testing.T, call []string) string {
	t.Helper()
	i := slices.Index(call, "-t")
	if i < 0 || i+1 >= len(call) {
		t.Fatalf("call %q composes no -t argument", call)
	}
	return call[i+1]
}

func TestSessionTargetsAreComposedExactly(t *testing.T) {
	for _, route := range perSessionRoutes {
		t.Run(route.name, func(t *testing.T) {
			mock := commandertest.New(t, commandertest.Returns("", route.command))
			route.invoke(tmux.NewClient(mock))

			if len(mock.Calls()) != 1 {
				t.Fatalf("composed %d tmux calls, want exactly 1: %q", len(mock.Calls()), mock.Calls())
			}
			call := mock.Calls()[0]
			if call[0] != route.command {
				t.Errorf("ran tmux %q, want %q", call[0], route.command)
			}
			if got := targetArgOf(t, call); got != route.want {
				t.Errorf("-t argument = %q, want %q", got, route.want)
			}
		})
	}
}
