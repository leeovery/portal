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
		// interpreted as a (malformed) verb. The element is shell-quoted, so it
		// lands inside the single-quoted word.
		script := ghosttyOpenScript([]string{"echo 100%done"})

		if !strings.Contains(script, `command:"'echo 100%done'"`) {
			t.Errorf("script does not carry the literal percent payload %q; script:\n%s", `command:"'echo 100%done'"`, script)
		}
	})

	t.Run("it single-quotes each argv element before embedding", func(t *testing.T) {
		cmd := realAttachArgv()
		script := ghosttyOpenScript(cmd)

		// Each element is POSIX single-quoted so Ghostty's bash -c re-split
		// reproduces the exact argv; a quote/backslash-free argv needs no further
		// AppleScript escaping, so the embed is just the single-quoted join.
		quoted := make([]string, len(cmd))
		for i, a := range cmd {
			quoted[i] = "'" + a + "'"
		}
		embedded := strings.Join(quoted, " ")
		if ghosttyEmbed(cmd) != embedded {
			t.Errorf("ghosttyEmbed = %q, want the single-quoted join %q", ghosttyEmbed(cmd), embedded)
		}
		if !strings.Contains(script, `command:"`+embedded+`"`) {
			t.Errorf("script does not embed the command property %q; script:\n%s", `command:"`+embedded+`"`, script)
		}
	})

	t.Run("it preserves an argv element containing a space (spaced-session-name fix)", func(t *testing.T) {
		// A spaced session name ("My Project-abc123") must survive Ghostty's
		// bash -c word-split as ONE element, not be shredded into "My" and
		// "Project-abc123" — the defect this fix closes.
		embedded := ghosttyEmbed([]string{"/abs/portal", "attach", "My Project-abc123"})

		if !strings.Contains(embedded, `'My Project-abc123'`) {
			t.Errorf("ghosttyEmbed = %q, want the spaced session name quoted as one word", embedded)
		}
	})
}

func TestGhosttyEmbed(t *testing.T) {
	t.Run("it AppleScript-escapes embedded double quotes and backslashes in the composed command", func(t *testing.T) {
		// The element carries BOTH a backslash and a double quote. It is first
		// POSIX-single-quoted (a\b"c -> 'a\b"c'), then AppleScript-escaped:
		// backslash FIRST (\ -> \\), then the quote (" -> \"). Escaping the quote
		// first would then double the escaping backslash and corrupt the literal,
		// yielding 'a\\b\\"c' instead of the correct 'a\\b\"c'.
		got := ghosttyEmbed([]string{`a\b"c`})

		want := `'a\\b\"c'`
		if got != want {
			t.Fatalf("ghosttyEmbed(%q) = %q, want %q (single-quoted, backslash escaped before quote)", `a\b"c`, got, want)
		}

		// Embedded inside the AppleScript literal, no unescaped payload quote
		// survives to prematurely close the double-quoted string.
		script := ghosttyOpenScript([]string{`a\b"c`})
		if !strings.Contains(script, `command:"`+want+`"`) {
			t.Errorf("script does not carry the escaped command %q; script:\n%s", want, script)
		}
	})

	t.Run("it preserves a single quote through shell-quoting and AppleScript escaping", func(t *testing.T) {
		// it's -> shellQuote -> 'it'\''s' -> AppleScript doubles the backslash ->
		// 'it'\\''s'. Ghostty's AppleScript layer then un-doubles it and bash -c
		// reconstructs the literal it's.
		got := ghosttyEmbed([]string{"it's"})

		want := `'it'\\''s'`
		if got != want {
			t.Fatalf("ghosttyEmbed(%q) = %q, want %q", "it's", got, want)
		}
	})
}
