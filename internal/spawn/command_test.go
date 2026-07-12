package spawn

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// fixedExe builds an ExecutableResolver that always resolves to path with no
// error — the happy-path seam for command composition tests.
func fixedExe(path string) ExecutableResolver {
	return func() (string, error) { return path, nil }
}

func TestAttachCommand(t *testing.T) {
	t.Run("it composes env -u TMUX -u TMUX_PANE PATH=<full> <exe> attach <session>", func(t *testing.T) {
		getenv := mapGetenv(map[string]string{"PATH": "/opt/homebrew/bin:/usr/bin"})

		got, err := AttachCommand("proj-abc123", fixedExe("/abs/portal"), getenv)
		if err != nil {
			t.Fatalf("AttachCommand returned error: %v, want nil", err)
		}

		want := []string{
			"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
			"PATH=/opt/homebrew/bin:/usr/bin",
			"/abs/portal", "attach", "proj-abc123",
		}
		if !slices.Equal(got, want) {
			t.Errorf("AttachCommand argv = %#v, want %#v", got, want)
		}
	})

	t.Run("it injects only PATH and strips TMUX/TMUX_PANE even when composed inside tmux", func(t *testing.T) {
		// A live TMUX / TMUX_PANE in the picker's env (the composed-from-inside-
		// tmux case) must never ride into the argv as an assignment — only the
		// explicit -u unsets strip them, and PATH is the sole injected var.
		getenv := mapGetenv(map[string]string{
			"PATH":      "/opt/homebrew/bin:/usr/bin",
			"TMUX":      "/private/tmp/tmux-501/default,12345,0",
			"TMUX_PANE": "%3",
		})

		got, err := AttachCommand("proj-abc123", fixedExe("/abs/portal"), getenv)
		if err != nil {
			t.Fatalf("AttachCommand returned error: %v, want nil", err)
		}

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

		// The full argv is deterministic: the live TMUX/TMUX_PANE values are
		// nowhere in it — the argv is byte-identical to the no-TMUX case.
		want := []string{
			"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
			"PATH=/opt/homebrew/bin:/usr/bin",
			"/abs/portal", "attach", "proj-abc123",
		}
		if !slices.Equal(got, want) {
			t.Errorf("AttachCommand argv = %#v, want %#v", got, want)
		}
	})

	t.Run("it keeps a session name with spaces as a single unquoted argv element", func(t *testing.T) {
		getenv := mapGetenv(map[string]string{"PATH": "/usr/bin"})

		got, err := AttachCommand("my session", fixedExe("/abs/portal"), getenv)
		if err != nil {
			t.Fatalf("AttachCommand returned error: %v, want nil", err)
		}

		if len(got) == 0 || got[len(got)-1] != "my session" {
			t.Fatalf("tail argv element = %q, want a single unquoted %q; argv = %#v", got[len(got)-1], "my session", got)
		}
		// No shell quoting is added anywhere: no element carries a stray quote.
		for _, elem := range got {
			if strings.ContainsAny(elem, `"'`) {
				t.Errorf("argv element %q contains a shell quote, want none added; argv = %#v", elem, got)
			}
		}
	})

	t.Run("it uses the resolved executable path rather than a bare portal lookup", func(t *testing.T) {
		getenv := mapGetenv(map[string]string{"PATH": "/usr/bin"})

		got, err := AttachCommand("s1", fixedExe("/usr/local/bin/portal-v2"), getenv)
		if err != nil {
			t.Fatalf("AttachCommand returned error: %v, want nil", err)
		}

		// The exe element sits immediately before "attach"; it must be the
		// resolved absolute path, never a bare "portal" PATH lookup.
		attachIdx := slices.Index(got, "attach")
		if attachIdx < 1 {
			t.Fatalf("no 'attach' element (or nothing before it) in argv %#v", got)
		}
		if exe := got[attachIdx-1]; exe != "/usr/local/bin/portal-v2" {
			t.Errorf("executable argv element = %q, want %q", exe, "/usr/local/bin/portal-v2")
		}
		if slices.Contains(got, "portal") {
			t.Errorf("argv contains bare %q element, want the resolved absolute path only; argv = %#v", "portal", got)
		}
	})

	t.Run("it surfaces an os.Executable resolution error", func(t *testing.T) {
		sentinel := errors.New("os.Executable: readlink /proc/self/exe: no such file")
		failExe := func() (string, error) { return "", sentinel }
		getenv := mapGetenv(map[string]string{"PATH": "/usr/bin"})

		got, err := AttachCommand("s1", failExe, getenv)
		if got != nil {
			t.Errorf("argv = %#v, want nil on resolution error", got)
		}
		if err == nil {
			t.Fatal("AttachCommand error = nil, want a non-nil wrapped error")
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false, want true; err = %v", err)
		}
	})

	t.Run("it omits --spawn-ack in phase 2", func(t *testing.T) {
		getenv := mapGetenv(map[string]string{"PATH": "/usr/bin"})

		got, err := AttachCommand("s1", fixedExe("/abs/portal"), getenv)
		if err != nil {
			t.Fatalf("AttachCommand returned error: %v, want nil", err)
		}

		for _, elem := range got {
			if strings.Contains(elem, "--spawn-ack") {
				t.Errorf("argv element %q contains --spawn-ack, want it deferred to Phase 3; argv = %#v", elem, got)
			}
		}
	})
}
