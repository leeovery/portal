package tmux_test

import (
	"reflect"
	"testing"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/tmux"
)

// The exactness vocabulary returns tmux.Target rather than a plain string, so a
// hand-composed target cannot be assigned where a pinned one is required. The
// rendering below is the whole of what the type may change: a Target must spell
// out exactly what the string form did.
func TestExactTargetFormsRenderUnchanged(t *testing.T) {
	t.Run("it renders each exact target form to the same string as before the type change", func(t *testing.T) {
		forms := []struct {
			name string
			got  tmux.Target
			want string
		}{
			{"SessionTargetExact", tmux.SessionTargetExact("work"), "=work"},
			{"CoordTargetExact", tmux.CoordTargetExact("work"), "=work:"},
			{"PaneTargetExact", tmux.PaneTargetExact("work", 2, 3), "=work:2.3"},
			{"WindowTargetExact", tmux.WindowTargetExact("work", 2), "=work:2"},
			{"PaneIDTarget", tmux.PaneIDTarget("%7"), "%7"},
		}

		for _, form := range forms {
			if string(form.got) != form.want {
				t.Errorf("%s = %q, want %q", form.name, string(form.got), form.want)
			}
		}
	})

	t.Run("it leaves the unpinned forms returning a plain string", func(t *testing.T) {
		assertPlainString(t, "PaneTarget", tmux.PaneTarget("work", 2, 3), "work:2.3")
	})
}

// assertPlainString takes its subject as a string parameter, so a form that
// started returning Target would not compile here: the declared type is as much
// of the assertion as the value.
func assertPlainString(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

// targetTakingRoute is one client method that is handed an already-composed
// target rather than a session name.
type targetTakingRoute struct {
	name   string
	invoke func(*tmux.Client)
	want   []string
}

const (
	routeTargetSession = "work"
	routeTargetHookKey = "#{@portal-pane-id}"
)

var targetTakingRoutes = []targetTakingRoute{
	{
		name: "SetPaneOption",
		invoke: func(c *tmux.Client) {
			_ = c.SetPaneOption(tmux.PaneTargetExact(routeTargetSession, 2, 3), "@portal-pane-id", "tok")
		},
		want: []string{"set-option", "-p", "-t", "=work:2.3", "@portal-pane-id", "tok"},
	},
	{
		name:   "RespawnPane",
		invoke: func(c *tmux.Client) { _ = c.RespawnPane(tmux.PaneTargetExact(routeTargetSession, 2, 3), "hydrate") },
		want:   []string{"respawn-pane", "-k", "-t", "=work:2.3", "hydrate"},
	},
	{
		name:   "CapturePane",
		invoke: func(c *tmux.Client) { _, _ = c.CapturePane(tmux.PaneTargetExact(routeTargetSession, 2, 3)) },
		want:   []string{"capture-pane", "-e", "-p", "-S", "-", "-t", "=work:2.3"},
	},
	{
		name:   "NewWindow",
		invoke: func(c *tmux.Client) { _ = c.NewWindow(tmux.CoordTargetExact(routeTargetSession), "name", "/tmp", "sh") },
		want:   []string{"new-window", "-t", "=work:", "-n", "name", "-c", "/tmp", "sh"},
	},
	{
		name:   "SplitWindow",
		invoke: func(c *tmux.Client) { _ = c.SplitWindow(tmux.CoordTargetExact(routeTargetSession), "/tmp", "sh") },
		want:   []string{"split-window", "-t", "=work:", "-c", "/tmp", "sh"},
	},
	{
		name:   "SelectLayout",
		invoke: func(c *tmux.Client) { _ = c.SelectLayout(routeTargetSession, 2, "tiled") },
		want:   []string{"select-layout", "-t", "=work:2", "tiled"},
	},
	{
		name:   "SelectWindow",
		invoke: func(c *tmux.Client) { _ = c.SelectWindow(routeTargetSession, 2) },
		want:   []string{"select-window", "-t", "=work:2"},
	},
	{
		name:   "SelectPane",
		invoke: func(c *tmux.Client) { _ = c.SelectPane(routeTargetSession, 2, 3) },
		want:   []string{"select-pane", "-t", "=work:2.3"},
	},
	{
		name:   "ResizePaneZoom",
		invoke: func(c *tmux.Client) { _ = c.ResizePaneZoom(routeTargetSession, 2, 3) },
		want:   []string{"resize-pane", "-Z", "-t", "=work:2.3"},
	},
}

func TestTargetTakingMethodsComposeUnchangedArgv(t *testing.T) {
	t.Run("it composes byte-identical argv for every -t-taking client method", func(t *testing.T) {
		for _, route := range targetTakingRoutes {
			t.Run(route.name, func(t *testing.T) {
				mock := commandertest.New(t, commandertest.Returns("", route.want[0]))

				route.invoke(tmux.NewClient(mock))

				if len(mock.Calls()) != 1 {
					t.Fatalf("composed %d tmux calls, want exactly 1: %q", len(mock.Calls()), mock.Calls())
				}
				if got := mock.Calls()[0]; !reflect.DeepEqual(got, route.want) {
					t.Errorf("argv = %q, want %q", got, route.want)
				}
			})
		}
	})

	t.Run("it composes byte-identical argv for the two reads a hook key resolves through", func(t *testing.T) {
		mock := commandertest.New(t,
			commandertest.Returns("", "show-options"),
			commandertest.Returns("", "display-message"),
		)

		_, _ = tmux.NewClient(mock).ResolveHookKey(tmux.PaneIDTarget("%7"))

		want := [][]string{
			{"show-options", "-p", "-t", "%7"},
			{"display-message", "-p", "-t", "%7", routeTargetHookKey},
		}
		if got := mock.Calls(); !reflect.DeepEqual(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	})
}
