package spawn

import (
	"strings"
	"testing"
)

// realAttachArgv is a representative composed attach argv (as Task 2.3 builds
// it) — it carries no quotes or backslashes, so its embedding is escape-neutral.
func realAttachArgv() []string {
	return []string{
		"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE",
		"PATH=/opt/homebrew/bin:/usr/bin",
		"/abs/portal", "attach", "proj-abc123",
	}
}

func TestGhosttyOpenArgv(t *testing.T) {
	t.Run("it wraps the script as osascript -e <script>", func(t *testing.T) {
		cmd := realAttachArgv()

		argv := ghosttyOpenArgv(cmd)

		if len(argv) != 3 {
			t.Fatalf("argv length = %d, want 3; argv = %#v", len(argv), argv)
		}
		if argv[0] != "osascript" {
			t.Errorf("argv[0] = %q, want %q", argv[0], "osascript")
		}
		if argv[1] != "-e" {
			t.Errorf("argv[1] = %q, want %q", argv[1], "-e")
		}
		if argv[2] != ghosttyOpenScript(cmd) {
			t.Errorf("argv[2] = %q, want the built script %q", argv[2], ghosttyOpenScript(cmd))
		}
	})
}

func TestGhosttyOpenScript(t *testing.T) {
	t.Run("it builds a new window with configuration carrying a command property", func(t *testing.T) {
		script := ghosttyOpenScript(realAttachArgv())

		wants := []string{
			`tell application "Ghostty"`,
			"new window",
			"with configuration",
			`command:"`,
			"wait after command",
			"end tell",
		}
		for _, want := range wants {
			if !strings.Contains(script, want) {
				t.Errorf("script missing %q; script:\n%s", want, script)
			}
		}

		// "surface configuration" only ever existed in the old invalid
		// `make new surface configuration with properties {…}` form; the
		// sdef-correct `new window with configuration {…}` template must not
		// carry that keyword.
		if strings.Contains(script, "surface configuration") {
			t.Errorf("script still contains stale keyword %q; script:\n%s", "surface configuration", script)
		}
	})

	t.Run("it keeps a percent in the payload inert", func(t *testing.T) {
		// The single %s is a fmt.Sprintf ARGUMENT, never a format-verb source,
		// so a `%` in the payload passes through literally rather than being
		// interpreted as a (malformed) verb.
		script := ghosttyOpenScript([]string{"echo 100%done"})

		if !strings.Contains(script, `command:"echo 100%done"`) {
			t.Errorf("script does not carry the literal percent payload %q; script:\n%s", `command:"echo 100%done"`, script)
		}
	})

	t.Run("it embeds the composed attach argv verbatim after escaping", func(t *testing.T) {
		cmd := realAttachArgv()
		script := ghosttyOpenScript(cmd)

		// A quote/backslash-free argv embeds verbatim as a single space-joined
		// string, so the window runs exactly that command.
		embedded := strings.Join(cmd, " ")
		if ghosttyEmbed(cmd) != embedded {
			t.Errorf("ghosttyEmbed = %q, want the verbatim space-join %q", ghosttyEmbed(cmd), embedded)
		}
		if !strings.Contains(script, `command:"`+embedded+`"`) {
			t.Errorf("script does not embed the command property %q; script:\n%s", `command:"`+embedded+`"`, script)
		}
	})

	t.Run("it is pure — identical output for the same input", func(t *testing.T) {
		cmd := realAttachArgv()

		if a, b := ghosttyOpenScript(cmd), ghosttyOpenScript(cmd); a != b {
			t.Errorf("ghosttyOpenScript is not pure: first call = %q, second = %q", a, b)
		}
	})
}

func TestGhosttyEmbed(t *testing.T) {
	t.Run("it AppleScript-escapes embedded double quotes and backslashes in the composed command", func(t *testing.T) {
		// The element carries BOTH a backslash and a double quote. Backslash
		// must be escaped FIRST (\ -> \\), then the quote (" -> \"). Escaping
		// the quote first would then double the escaping backslash and corrupt
		// the literal, yielding a\\b\\"c instead of the correct a\\b\"c.
		got := ghosttyEmbed([]string{`a\b"c`})

		want := `a\\b\"c`
		if got != want {
			t.Fatalf("ghosttyEmbed(%q) = %q, want %q (backslash escaped before quote)", `a\b"c`, got, want)
		}

		// Embedded inside the AppleScript literal, no unescaped payload quote
		// survives to prematurely close the double-quoted string.
		script := ghosttyOpenScript([]string{`a\b"c`})
		if !strings.Contains(script, `command:"`+want+`"`) {
			t.Errorf("script does not carry the escaped command %q; script:\n%s", want, script)
		}
	})
}
