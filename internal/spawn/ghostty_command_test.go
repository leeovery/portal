package spawn

import (
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/shellquote"
)

func realAttachArgv() []string {
	return composeOpenArgv(
		"/abs/portal",
		"/opt/homebrew/bin:/usr/bin",
		Surface{Kind: SurfaceAttach, Value: "proj-abc123"},
		"batch1", "tok1",
		nil,
	)
}

func mintArgvWithSpecials() []string {
	return composeOpenArgv(
		"/abs/portal",
		"/opt/homebrew/bin:/usr/bin",
		Surface{Kind: SurfaceMint, Value: "/abs/dir"},
		"batch1", "tok1",
		[]string{`echo 'a';$x"b"`},
	)
}

// Duplicated rather than imported so the test pins the expected bytes
// independently of the production constant.
const shellFallbackSuffix = `; exec "$SHELL" -il`

// Hand-authored golden bytes: nothing here recomputes them from the production
// escaping helpers, so a symmetric encode/decode bug cannot pass falsely.
const wantAttachCommandBody = `'bash' '-lc' ''\\''/usr/bin/env'\\'' '\\''-u'\\'' '\\''TMUX'\\'' '\\''-u'\\'' '\\''TMUX_PANE'\\'' '\\''PATH=/opt/homebrew/bin:/usr/bin'\\'' '\\''/abs/portal'\\'' '\\''open'\\'' '\\''--session'\\'' '\\''proj-abc123'\\'' '\\''--ack'\\'' '\\''batch1:tok1'\\''; exec \"$SHELL\" -il'`

const wantMintCommandBody = `'bash' '-lc' ''\\''/usr/bin/env'\\'' '\\''-u'\\'' '\\''TMUX'\\'' '\\''-u'\\'' '\\''TMUX_PANE'\\'' '\\''PATH=/opt/homebrew/bin:/usr/bin'\\'' '\\''/abs/portal'\\'' '\\''open'\\'' '\\''--path'\\'' '\\''/abs/dir'\\'' '\\''--ack'\\'' '\\''batch1:tok1'\\'' '\\''--'\\'' '\\''echo '\\''\\'\\'''\\''a'\\''\\'\\'''\\'';$x\"b\"'\\''; exec \"$SHELL\" -il'`

// Undoes the two escape passes in reverse order. In the data ghosttyEmbed
// produces a backslash and a double quote are never adjacent pre-escape, so the
// reversal is unambiguous.
func reverseAppleScriptEscape(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

func decodeRenderedArgv(s string) []string {
	var argv []string
	var cur strings.Builder
	inSingle := false
	started := false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				cur.WriteByte(c)
			}
			i++
		case c == '\'':
			inSingle = true
			started = true
			i++
		case c == '\\':
			if i+1 < len(s) {
				cur.WriteByte(s[i+1])
				i += 2
			} else {
				cur.WriteByte(c)
				i++
			}
			started = true
		case c == ' ':
			if started {
				argv = append(argv, cur.String())
				cur.Reset()
				started = false
			}
			i++
		default:
			cur.WriteByte(c)
			started = true
			i++
		}
	}
	if started {
		argv = append(argv, cur.String())
	}
	return argv
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

func TestWrapWithShellFallback(t *testing.T) {
	t.Run("it returns [bash -lc <rendered argv>; exec \"$SHELL\" -il] exactly", func(t *testing.T) {
		cmd := realAttachArgv()

		wrapped := wrapWithShellFallback(cmd)

		if len(wrapped) != 3 {
			t.Fatalf("wrapped length = %d, want 3; wrapped = %#v", len(wrapped), wrapped)
		}
		if wrapped[0] != "bash" {
			t.Errorf("wrapped[0] = %q, want %q", wrapped[0], "bash")
		}
		if wrapped[1] != "-lc" {
			t.Errorf("wrapped[1] = %q, want %q", wrapped[1], "-lc")
		}
		wantPayload := shellquote.Join(cmd) + shellFallbackSuffix
		if wrapped[2] != wantPayload {
			t.Errorf("wrapped[2] = %q, want %q", wrapped[2], wantPayload)
		}
	})

	t.Run("it wraps a mint --path argv with the identical bash -lc shape (argv-agnostic)", func(t *testing.T) {
		attach := wrapWithShellFallback(realAttachArgv())
		mint := wrapWithShellFallback(mintArgvWithSpecials())

		if len(mint) != 3 {
			t.Fatalf("mint wrapped length = %d, want 3; wrapped = %#v", len(mint), mint)
		}
		if mint[0] != attach[0] || mint[1] != attach[1] {
			t.Errorf("mint wrapper prefix = [%q %q], want [%q %q] (identical to attach shape)",
				mint[0], mint[1], attach[0], attach[1])
		}
		if mint[0] != "bash" || mint[1] != "-lc" {
			t.Errorf("mint wrapper prefix = [%q %q], want [bash -lc]", mint[0], mint[1])
		}
		wantPayload := shellquote.Join(mintArgvWithSpecials()) + shellFallbackSuffix
		if mint[2] != wantPayload {
			t.Errorf("mint wrapped[2] = %q, want %q", mint[2], wantPayload)
		}
	})
}

func TestGhosttyOpenScript(t *testing.T) {
	t.Run("it builds a new window with configuration carrying a single command property", func(t *testing.T) {
		script := ghosttyOpenScript(realAttachArgv())

		wants := []string{
			`tell application "Ghostty"`,
			"new window",
			"with configuration",
			`command:"`,
			"end tell",
		}
		for _, want := range wants {
			if !strings.Contains(script, want) {
				t.Errorf("script missing %q; script:\n%s", want, script)
			}
		}

		if strings.Contains(script, "surface configuration") {
			t.Errorf("script still contains stale keyword %q; script:\n%s", "surface configuration", script)
		}
	})

	t.Run("it emits no wait after command for any input", func(t *testing.T) {
		inputs := [][]string{
			realAttachArgv(),
			mintArgvWithSpecials(),
			{"echo", "100%done"},
			{`a\b"c`},
		}
		for _, in := range inputs {
			if got := ghosttyOpenScript(in); strings.Contains(got, "wait after command") {
				t.Errorf("ghosttyOpenScript(%#v) still carries %q; script:\n%s", in, "wait after command", got)
			}
		}
	})

	t.Run("it embeds the bash -lc shell-fallback wrapper with the escaped exec tail", func(t *testing.T) {
		script := ghosttyOpenScript(realAttachArgv())

		if !strings.Contains(script, `command:"'bash' '-lc' `) {
			t.Errorf("script does not open with the bash -lc wrapper; script:\n%s", script)
		}
		if !strings.Contains(script, `exec \"$SHELL\" -il`) {
			t.Errorf("script does not carry the AppleScript-escaped exec fallback tail; script:\n%s", script)
		}
	})

	t.Run("it keeps a percent in the payload inert", func(t *testing.T) {
		script := ghosttyOpenScript([]string{"echo", "100%done"})

		if !strings.Contains(script, "100%done") {
			t.Errorf("script dropped the literal percent payload; script:\n%s", script)
		}
		if strings.Contains(script, "%!") {
			t.Errorf("script carries a fmt error verb (%%!); script:\n%s", script)
		}
	})

	t.Run("it preserves an argv element containing a space (spaced-session-name fix)", func(t *testing.T) {
		script := ghosttyOpenScript([]string{"/abs/portal", "open", "--session", "My Project-abc123"})

		if !strings.Contains(script, "My Project-abc123") {
			t.Errorf("script shredded the spaced session name; script:\n%s", script)
		}
	})
}

func TestGhosttyEmbed(t *testing.T) {
	t.Run("it round-trips a composed attach argv uncorrupted through the bash -lc wrapper", func(t *testing.T) {
		cmd := realAttachArgv()

		recovered := decodeRenderedArgv(reverseAppleScriptEscape(ghosttyEmbed(cmd)))

		if want := wrapWithShellFallback(cmd); !reflect.DeepEqual(recovered, want) {
			t.Fatalf("round-trip recovered %#v, want %#v", recovered, want)
		}
	})

	t.Run("it round-trips a quote-sensitive mint passthrough element uncorrupted", func(t *testing.T) {
		cmd := mintArgvWithSpecials()

		embed := ghosttyEmbed(cmd)

		if !strings.Contains(embed, `'\\''`) {
			t.Fatalf("embed missing the doubled single-quote escape signature '\\\\''; embed:\n%s", embed)
		}

		recovered := decodeRenderedArgv(reverseAppleScriptEscape(embed))
		if want := wrapWithShellFallback(cmd); !reflect.DeepEqual(recovered, want) {
			t.Fatalf("round-trip recovered %#v, want %#v", recovered, want)
		}

		payload := recovered[2]
		innerRendered := strings.TrimSuffix(payload, shellFallbackSuffix)
		if innerRendered == payload {
			t.Fatalf("payload missing the exec fallback suffix; payload=%q", payload)
		}
		if innerArgv := decodeRenderedArgv(innerRendered); !reflect.DeepEqual(innerArgv, cmd) {
			t.Fatalf("inner argv recovered %#v, want %#v", innerArgv, cmd)
		}
	})

	t.Run("it preserves the composed argv's PATH / -u TMUX prefix inside the wrapper", func(t *testing.T) {
		embed := ghosttyEmbed(mintArgvWithSpecials())

		for _, frag := range []string{
			"/usr/bin/env",
			"-u",
			"TMUX",
			"TMUX_PANE",
			"PATH=/opt/homebrew/bin:/usr/bin",
		} {
			if !strings.Contains(embed, frag) {
				t.Errorf("embed dropped env-prefix fragment %q; embed:\n%s", frag, embed)
			}
		}
	})
}

func TestGhosttyEmbedGoldenLiteral(t *testing.T) {
	t.Run("it emits the exact '\\''-escaped, AppleScript-escaped body for the canonical attach argv", func(t *testing.T) {
		body := ghosttyEmbed(realAttachArgv())

		if body != wantAttachCommandBody {
			t.Fatalf("attach command body mismatch:\n got: %s\nwant: %s", body, wantAttachCommandBody)
		}

		script := ghosttyOpenScript(realAttachArgv())
		if want := `command:"` + wantAttachCommandBody + `"`; !strings.Contains(script, want) {
			t.Errorf("script does not carry the golden command body verbatim; script:\n%s", script)
		}
	})

	t.Run("it emits the exact '\\''-escaped, AppleScript-escaped body for the quote-sensitive mint fixture", func(t *testing.T) {
		body := ghosttyEmbed(mintArgvWithSpecials())

		if body != wantMintCommandBody {
			t.Fatalf("mint command body mismatch:\n got: %s\nwant: %s", body, wantMintCommandBody)
		}

		script := ghosttyOpenScript(mintArgvWithSpecials())
		if want := `command:"` + wantMintCommandBody + `"`; !strings.Contains(script, want) {
			t.Errorf("script does not carry the golden command body verbatim; script:\n%s", script)
		}
	})
}
