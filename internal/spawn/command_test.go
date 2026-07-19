package spawn

import (
	"slices"
	"strings"
	"testing"
)

// fixedExe builds an ExecutableResolver that always resolves to path with no
// error — the happy-path seam for the burster's command-composition tests.
func fixedExe(path string) ExecutableResolver {
	return func() (string, error) { return path, nil }
}

func TestComposeOpenArgv(t *testing.T) {
	t.Run("an attach surface composes env -u TMUX -u TMUX_PANE PATH=<full> <exe> open --session <name> --ack <batch>:<token>", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", "/opt/homebrew/bin:/usr/bin", Surface{Kind: SurfaceAttach, Value: "proj-abc123"}, "b1", "t1")

		want := []string{
			"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
			"PATH=/opt/homebrew/bin:/usr/bin",
			"/abs/portal", "open", "--session", "proj-abc123",
			"--ack", "b1:t1",
		}
		if !slices.Equal(got, want) {
			t.Errorf("composeOpenArgv attach argv = %#v, want %#v", got, want)
		}
	})

	t.Run("a mint surface composes env -u TMUX -u TMUX_PANE PATH=<full> <exe> open --path <literal-dir> --ack <batch>:<token>", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", "/opt/homebrew/bin:/usr/bin", Surface{Kind: SurfaceMint, Value: "/Users/me/projects/foo"}, "b1", "t1")

		want := []string{
			"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
			"PATH=/opt/homebrew/bin:/usr/bin",
			"/abs/portal", "open", "--path", "/Users/me/projects/foo",
			"--ack", "b1:t1",
		}
		if !slices.Equal(got, want) {
			t.Errorf("composeOpenArgv mint argv = %#v, want %#v", got, want)
		}
	})

	t.Run("it injects only PATH and strips TMUX/TMUX_PANE via explicit -u unsets", func(t *testing.T) {
		// The strip is structural: the argv carries the -u TMUX / -u TMUX_PANE
		// unsets and its ONLY env assignment is PATH= — never a TMUX=/TMUX_PANE=
		// assignment, so a picker composing from inside tmux cannot leak them.
		got := composeOpenArgv("/abs/portal", "/opt/homebrew/bin:/usr/bin", Surface{Kind: SurfaceAttach, Value: "proj-abc123"}, "b1", "t1")

		var pathAssignments, tmuxAssignments, tmuxPaneAssignments int
		for _, elem := range got {
			switch {
			case strings.HasPrefix(elem, "PATH="):
				pathAssignments++
			case strings.HasPrefix(elem, "TMUX="):
				tmuxAssignments++
			case strings.HasPrefix(elem, "TMUX_PANE="):
				tmuxPaneAssignments++
			}
		}
		if pathAssignments != 1 {
			t.Errorf("PATH= assignment count = %d, want exactly 1; argv = %#v", pathAssignments, got)
		}
		if tmuxAssignments != 0 {
			t.Errorf("TMUX= assignment count = %d, want 0 (only the -u unset); argv = %#v", tmuxAssignments, got)
		}
		if tmuxPaneAssignments != 0 {
			t.Errorf("TMUX_PANE= assignment count = %d, want 0 (only the -u unset); argv = %#v", tmuxPaneAssignments, got)
		}
		if !slices.Contains(got, "-u") {
			t.Errorf("argv missing the -u unset flag; argv = %#v", got)
		}
	})

	t.Run("it keeps a session name with spaces as a single unquoted argv element", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", "/usr/bin", Surface{Kind: SurfaceAttach, Value: "my session"}, "b1", "t1")

		// The name sits immediately after "--session"; it is a discrete argv
		// element (no shell quoting) even with an embedded space.
		flagIdx := slices.Index(got, "--session")
		if flagIdx < 0 || flagIdx+1 >= len(got) {
			t.Fatalf("no '--session' element (or nothing after it) in argv %#v", got)
		}
		if name := got[flagIdx+1]; name != "my session" {
			t.Fatalf("session argv element = %q, want a single unquoted %q; argv = %#v", name, "my session", got)
		}
		// No shell quoting is added anywhere: no element carries a stray quote.
		for _, elem := range got {
			if strings.ContainsAny(elem, `"'`) {
				t.Errorf("argv element %q contains a shell quote, want none added; argv = %#v", elem, got)
			}
		}
	})

	t.Run("it keeps a mint directory with spaces as a single unquoted argv element", func(t *testing.T) {
		got := composeOpenArgv("/abs/portal", "/usr/bin", Surface{Kind: SurfaceMint, Value: "/Users/me/my projects/foo"}, "b1", "t1")

		// The literal dir sits immediately after "--path"; it stays one discrete
		// argv element even with an embedded space.
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
		got := composeOpenArgv("/usr/local/bin/portal-v2", "/usr/bin", Surface{Kind: SurfaceAttach, Value: "s1"}, "b1", "t1")

		// The exe element sits immediately before "open"; it must be the provided
		// absolute path, never a bare "portal" PATH lookup.
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
		got := composeOpenArgv("/abs/portal", "/usr/bin", Surface{Kind: SurfaceAttach, Value: "s1"}, "batchA", "tokenB")

		if len(got) < 2 {
			t.Fatalf("argv too short to carry the ack flag: %#v", got)
		}
		if flag := got[len(got)-2]; flag != "--ack" {
			t.Errorf("penultimate argv element = %q, want %q; argv = %#v", flag, "--ack", got)
		}
		if value := got[len(got)-1]; value != "batchA:tokenB" {
			t.Errorf("final argv element = %q, want the %q ack value; argv = %#v", value, "batchA:tokenB", got)
		}
		// The ack flag is exactly two discrete argv elements — never a single
		// "--ack=batchA:tokenB" joined element and never shell-quoted.
		if slices.Contains(got[:len(got)-1], "--ack=batchA:tokenB") {
			t.Errorf("argv carries a joined --ack=value element, want two discrete elements; argv = %#v", got)
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
