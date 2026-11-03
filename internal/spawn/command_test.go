package spawn

import (
	"slices"
	"strings"
	"testing"
)

// testWindowEnv stands for an opener's window environment: a variable it
// lacks, then its PATH and its resolved zmx and state directories.
var testWindowEnv = WindowEnvironment{
	Unset: []string{"PORTAL_HOOKS_FILE"},
	Set: []string{
		"PATH=/opt/homebrew/bin:/usr/bin",
		"ZMX_DIR=/Users/me/.local/state/zmx",
		"PORTAL_STATE_DIR=/Users/me/portal-state",
	},
}

func fixedExe(path string) ExecutableResolver {
	return func() (string, error) { return path, nil }
}

func TestComposeOpenArgv(t *testing.T) {
	t.Run("an attach surface composes env -u ZMX_SESSION -u TMUX -u TMUX_PANE <window env> <exe> open --session <name> --ack <batch>:<token>", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceAttach, Value: "proj-abc123"}, "b1", "t1", nil)

		want := []string{
			"/usr/bin/env", "-u", "ZMX_SESSION", "-u", "TMUX", "-u", "TMUX_PANE", "-u", "PORTAL_HOOKS_FILE",
			"PATH=/opt/homebrew/bin:/usr/bin", "ZMX_DIR=/Users/me/.local/state/zmx", "PORTAL_STATE_DIR=/Users/me/portal-state",
			"/abs/portal", "open", "--session", "proj-abc123",
			"--ack", "b1:t1",
		}
		if !slices.Equal(got, want) {
			t.Errorf("composeOpenArgv attach argv = %#v, want %#v", got, want)
		}
	})

	t.Run("a mint surface composes env -u ZMX_SESSION -u TMUX -u TMUX_PANE <window env> <exe> open --path <literal-dir> --ack <batch>:<token>", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceMint, Value: "/Users/me/projects/foo"}, "b1", "t1", nil)

		want := []string{
			"/usr/bin/env", "-u", "ZMX_SESSION", "-u", "TMUX", "-u", "TMUX_PANE", "-u", "PORTAL_HOOKS_FILE",
			"PATH=/opt/homebrew/bin:/usr/bin", "ZMX_DIR=/Users/me/.local/state/zmx", "PORTAL_STATE_DIR=/Users/me/portal-state",
			"/abs/portal", "open", "--path", "/Users/me/projects/foo",
			"--ack", "b1:t1",
		}
		if !slices.Equal(got, want) {
			t.Errorf("composeOpenArgv mint argv = %#v, want %#v", got, want)
		}
	})

	t.Run("it unsets ZMX_SESSION, TMUX, TMUX_PANE and the window environment's unset, then assigns its set verbatim and in order", func(t *testing.T) {
		env := WindowEnvironment{
			Unset: []string{"XDG_CONFIG_HOME", "PORTAL_LOG_LEVEL"},
			Set:   []string{"PATH=/usr/bin", "PORTAL_THEMES_DIR=", "PORTAL_HOOKS_FILE=/Users/me/my config/hooks.json"},
		}
		got := composeOpenArgv("/abs/portal", env, Surface{Kind: SurfaceAttach, Value: "proj-abc123"}, "b1", "t1", nil)

		want := []string{"/usr/bin/env", "-u", "ZMX_SESSION", "-u", "TMUX", "-u", "TMUX_PANE", "-u", "XDG_CONFIG_HOME", "-u", "PORTAL_LOG_LEVEL"}
		want = append(append(want, env.Set...), "/abs/portal")
		if !slices.Equal(got[:len(want)], want) {
			t.Errorf("argv opens %#v, want %#v", got[:len(want)], want)
		}
		for _, elem := range got {
			for _, stripped := range []string{"ZMX_SESSION=", "TMUX=", "TMUX_PANE="} {
				if strings.HasPrefix(elem, stripped) {
					t.Errorf("argv assigns %s; want only its -u unset; argv = %#v", stripped, got)
				}
			}
		}
		for _, name := range []string{"ZMX_SESSION", "TMUX", "TMUX_PANE"} {
			if i := slices.Index(got, name); i < 1 || got[i-1] != "-u" {
				t.Errorf("argv does not unset %s with -u; argv = %#v", name, got)
			}
		}
	})

	t.Run("it keeps a session name with spaces as a single unquoted argv element", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceAttach, Value: "my session"}, "b1", "t1", nil)

		flagIdx := slices.Index(got, "--session")
		if flagIdx < 0 || flagIdx+1 >= len(got) {
			t.Fatalf("no '--session' element (or nothing after it) in argv %#v", got)
		}
		if name := got[flagIdx+1]; name != "my session" {
			t.Fatalf("session argv element = %q, want a single unquoted %q; argv = %#v", name, "my session", got)
		}
		for _, elem := range got {
			if strings.ContainsAny(elem, `"'`) {
				t.Errorf("argv element %q contains a shell quote, want none added; argv = %#v", elem, got)
			}
		}
	})

	t.Run("it keeps a mint directory with spaces as a single unquoted argv element", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceMint, Value: "/Users/me/my projects/foo"}, "b1", "t1", nil)

		flagIdx := slices.Index(got, "--path")
		if flagIdx < 0 || flagIdx+1 >= len(got) {
			t.Fatalf("no '--path' element (or nothing after it) in argv %#v", got)
		}
		if dir := got[flagIdx+1]; dir != "/Users/me/my projects/foo" {
			t.Fatalf("path argv element = %q, want a single unquoted %q; argv = %#v", dir, "/Users/me/my projects/foo", got)
		}
		for _, elem := range got {
			if strings.ContainsAny(elem, `"'`) {
				t.Errorf("argv element %q contains a shell quote, want none added; argv = %#v", elem, got)
			}
		}
	})

	t.Run("it uses the provided executable path rather than a bare portal lookup", func(t *testing.T) {
		got := composeOpenArgv("/usr/local/bin/portal-v2", testWindowEnv, Surface{Kind: SurfaceAttach, Value: "s1"}, "b1", "t1", nil)

		openIdx := slices.Index(got, "open")
		if openIdx < 1 {
			t.Fatalf("no 'open' element (or nothing before it) in argv %#v", got)
		}
		if exe := got[openIdx-1]; exe != "/usr/local/bin/portal-v2" {
			t.Errorf("executable argv element = %q, want %q", exe, "/usr/local/bin/portal-v2")
		}
		if slices.Contains(got, "portal") {
			t.Errorf("argv contains bare %q element, want the provided absolute path only; argv = %#v", "portal", got)
		}
	})

	t.Run("it appends --ack <batch>:<token> as the final two argv elements", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceAttach, Value: "s1"}, "batchA", "tokenB", nil)

		if len(got) < 2 {
			t.Fatalf("argv too short to carry the ack flag: %#v", got)
		}
		if flag := got[len(got)-2]; flag != "--ack" {
			t.Errorf("penultimate argv element = %q, want %q; argv = %#v", flag, "--ack", got)
		}
		if value := got[len(got)-1]; value != "batchA:tokenB" {
			t.Errorf("final argv element = %q, want the %q ack value; argv = %#v", value, "batchA:tokenB", got)
		}
		if slices.Contains(got[:len(got)-1], "--ack=batchA:tokenB") {
			t.Errorf("argv carries a joined --ack=value element, want two discrete elements; argv = %#v", got)
		}
	})

	t.Run("a mint surface WITH a command appends -- <cmd...> after --ack (whole-argv slice)", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceMint, Value: "/Users/me/projects/foo"}, "b1", "t1", []string{"claude", "--resume"})

		want := []string{
			"/usr/bin/env", "-u", "ZMX_SESSION", "-u", "TMUX", "-u", "TMUX_PANE", "-u", "PORTAL_HOOKS_FILE",
			"PATH=/opt/homebrew/bin:/usr/bin", "ZMX_DIR=/Users/me/.local/state/zmx", "PORTAL_STATE_DIR=/Users/me/portal-state",
			"/abs/portal", "open", "--path", "/Users/me/projects/foo",
			"--ack", "b1:t1",
			"--", "claude", "--resume",
		}
		if !slices.Equal(got, want) {
			t.Errorf("composeOpenArgv mint+command argv = %#v, want %#v", got, want)
		}
	})

	t.Run("an attach surface never carries the command even when one is passed", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceAttach, Value: "proj-abc123"}, "b1", "t1", []string{"claude"})

		want := []string{
			"/usr/bin/env", "-u", "ZMX_SESSION", "-u", "TMUX", "-u", "TMUX_PANE", "-u", "PORTAL_HOOKS_FILE",
			"PATH=/opt/homebrew/bin:/usr/bin", "ZMX_DIR=/Users/me/.local/state/zmx", "PORTAL_STATE_DIR=/Users/me/portal-state",
			"/abs/portal", "open", "--session", "proj-abc123",
			"--ack", "b1:t1",
		}
		if !slices.Equal(got, want) {
			t.Errorf("composeOpenArgv attach+command argv = %#v, want %#v (attach windows never carry the command)", got, want)
		}
		if slices.Contains(got, "--") {
			t.Errorf("attach argv carries a `--` passthrough terminator, want none; argv = %#v", got)
		}
	})

	t.Run("a mint surface with an empty command appends no bare -- terminator", func(t *testing.T) {
		for _, cmd := range [][]string{nil, {}} {
			got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceMint, Value: "/Users/me/projects/foo"}, "b1", "t1", cmd)

			want := []string{
				"/usr/bin/env", "-u", "ZMX_SESSION", "-u", "TMUX", "-u", "TMUX_PANE", "-u", "PORTAL_HOOKS_FILE",
				"PATH=/opt/homebrew/bin:/usr/bin", "ZMX_DIR=/Users/me/.local/state/zmx", "PORTAL_STATE_DIR=/Users/me/portal-state",
				"/abs/portal", "open", "--path", "/Users/me/projects/foo",
				"--ack", "b1:t1",
			}
			if !slices.Equal(got, want) {
				t.Errorf("composeOpenArgv mint+empty-command argv = %#v, want %#v (no bare --)", got, want)
			}
			if slices.Contains(got, "--") {
				t.Errorf("mint argv carries a bare `--` for an empty command; argv = %#v", got)
			}
		}
	})

	t.Run("a single-string command stays ONE argv element after -- (no word-splitting)", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", testWindowEnv, Surface{Kind: SurfaceMint, Value: "/repo/api"}, "b1", "t1", []string{"npm run dev"})

		want := []string{
			"/usr/bin/env", "-u", "ZMX_SESSION", "-u", "TMUX", "-u", "TMUX_PANE", "-u", "PORTAL_HOOKS_FILE",
			"PATH=/opt/homebrew/bin:/usr/bin", "ZMX_DIR=/Users/me/.local/state/zmx", "PORTAL_STATE_DIR=/Users/me/portal-state",
			"/abs/portal", "open", "--path", "/repo/api",
			"--ack", "b1:t1",
			"--", "npm run dev",
		}
		if !slices.Equal(got, want) {
			t.Errorf("composeOpenArgv single-string command argv = %#v, want %#v (one element after --, no word-split)", got, want)
		}
		dashIdx := slices.Index(got, "--")
		if dashIdx < 0 || dashIdx+1 >= len(got) {
			t.Fatalf("no `--` element (or nothing after it) in argv %#v", got)
		}
		if rest := got[dashIdx+1:]; len(rest) != 1 || rest[0] != "npm run dev" {
			t.Errorf("post-`--` argv = %#v, want exactly [%q] (one unsplit unit)", rest, "npm run dev")
		}
	})
}

func TestAttachSurfaces(t *testing.T) {
	t.Run("it maps each name to a SurfaceAttach surface carrying that name in list order", func(t *testing.T) {
		got := AttachSurfaces([]string{"alpha", "beta", "gamma"})

		want := []Surface{
			{Kind: SurfaceAttach, Value: "alpha"},
			{Kind: SurfaceAttach, Value: "beta"},
			{Kind: SurfaceAttach, Value: "gamma"},
		}
		if !slices.Equal(got, want) {
			t.Errorf("AttachSurfaces = %#v, want %#v", got, want)
		}
	})

	t.Run("it returns an empty (non-nil) slice for an empty name list", func(t *testing.T) {
		got := AttachSurfaces([]string{})
		if len(got) != 0 {
			t.Errorf("AttachSurfaces([]) = %#v, want empty", got)
		}
	})
}
