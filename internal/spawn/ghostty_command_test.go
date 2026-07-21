package spawn

import (
	"reflect"
	"strings"
	"testing"
)

// realAttachArgv is a representative composed attach argv, built through the real
// composeOpenArgv for a SurfaceAttach surface exactly as a spawned window's
// command is composed — the env-self-sufficient `/usr/bin/env … PATH=… -u TMUX
// -u TMUX_PANE` prefix in front of `open --session <name> --ack <batch>:<token>`
// (the retired `attach` verb is never emitted). None of its elements carry a
// single quote or backslash, so its embedding exercises the escape-neutral path
// — contrast mintArgvWithSpecials, whose passthrough element drives the nested
// single-quote escaping.
func realAttachArgv() []string {
	return composeOpenArgv(
		"/abs/portal",
		"/opt/homebrew/bin:/usr/bin",
		Surface{Kind: SurfaceAttach, Value: "proj-abc123"},
		"batch1", "tok1",
		nil,
	)
}

// mintArgvWithSpecials is a composed mint argv whose `-- <command…>` passthrough
// element carries the full quote-sensitive set (single quote, semicolon, dollar,
// double-quote), so the bash -lc wrapper's nested single-quote escaping is
// actually exercised rather than an escape-neutral attach argv.
func mintArgvWithSpecials() []string {
	return composeOpenArgv(
		"/abs/portal",
		"/opt/homebrew/bin:/usr/bin",
		Surface{Kind: SurfaceMint, Value: "/abs/dir"},
		"batch1", "tok1",
		[]string{`echo 'a';$x"b"`},
	)
}

// shellFallbackSuffix is the literal fallback tail wrapWithShellFallback appends
// after the composed argv rendering. It is duplicated here (not imported) so the
// test pins the exact expected bytes independently of the production constant.
const shellFallbackSuffix = `; exec "$SHELL" -il`

// wantAttachCommandBody and wantMintCommandBody are hand-authored golden literals:
// the EXACT bytes ghosttyEmbed must place inside the AppleScript `command:"…"`
// property for the canonical attach argv (realAttachArgv) and the quote-sensitive
// mint fixture (mintArgvWithSpecials). They were derived by hand-applying the two
// escaping layers ghosttyEmbed composes — the outer renderCommandString over the
// [bash -lc PAYLOAD] wrapper (POSIX single-quoting, re-escaping every inner single
// quote via the close-escape-reopen idiom '\'') then the AppleScript escape
// (backslash → doubled, then double-quote → \") — and are stored as fixed raw
// string literals. Nothing here recomputes the value from renderCommandString /
// wrapWithShellFallback / ghosttyEmbed / decodeRenderedArgv, so these constants
// are an INDEPENDENT oracle: a subtle change in renderCommandString's single-quote
// or the AppleScript escaping is caught by a mismatch rather than mirrored on both
// sides. The '\\''  runs are the doubled-backslash signature of the outer render
// re-escaping the wrapper's own single quotes, and \"$SHELL\" is the fallback
// tail's AppleScript-escaped double quotes.
const wantAttachCommandBody = `'bash' '-lc' ''\\''/usr/bin/env'\\'' '\\''-u'\\'' '\\''TMUX'\\'' '\\''-u'\\'' '\\''TMUX_PANE'\\'' '\\''PATH=/opt/homebrew/bin:/usr/bin'\\'' '\\''/abs/portal'\\'' '\\''open'\\'' '\\''--session'\\'' '\\''proj-abc123'\\'' '\\''--ack'\\'' '\\''batch1:tok1'\\''; exec \"$SHELL\" -il'`

const wantMintCommandBody = `'bash' '-lc' ''\\''/usr/bin/env'\\'' '\\''-u'\\'' '\\''TMUX'\\'' '\\''-u'\\'' '\\''TMUX_PANE'\\'' '\\''PATH=/opt/homebrew/bin:/usr/bin'\\'' '\\''/abs/portal'\\'' '\\''open'\\'' '\\''--path'\\'' '\\''/abs/dir'\\'' '\\''--ack'\\'' '\\''batch1:tok1'\\'' '\\''--'\\'' '\\''echo '\\''\\'\\'''\\''a'\\''\\'\\'''\\'';$x\"b\"'\\''; exec \"$SHELL\" -il'`

// reverseAppleScriptEscape reverses ghosttyEmbed's AppleScript-string escaping,
// undoing its two ReplaceAll passes in REVERSE order (quote-unescape before
// backslash-unescape), recovering the pre-escape shell-quoted string. For the
// data ghosttyEmbed produces, `\` and `"` are never adjacent before escaping (a
// backslash only ever precedes a single quote in the '\'' idiom; a double quote
// is always a standalone literal), so the reversal is unambiguous.
func reverseAppleScriptEscape(s string) string {
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	return s
}

// decodeRenderedArgv parses a renderCommandString output (space-joined,
// per-element POSIX single-quoted, with the '\'' close-escape-reopen idiom for an
// embedded single quote) back into its argv. It lets a test prove the wrapper
// round-trips uncorrupted via a deep-equal on the recovered argv, rather than
// string-matching a brittle hand-typed golden.
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
			// An escaped byte OUTSIDE single quotes — the '\'' idiom's middle \'.
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
		wantPayload := renderCommandString(cmd) + shellFallbackSuffix
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
		wantPayload := renderCommandString(mintArgvWithSpecials()) + shellFallbackSuffix
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

		// "surface configuration" only ever existed in the old invalid
		// `make new surface configuration with properties {…}` form; the
		// sdef-correct `new window with configuration {…}` template must not
		// carry that keyword.
		if strings.Contains(script, "surface configuration") {
			t.Errorf("script still contains stale keyword %q; script:\n%s", "surface configuration", script)
		}
	})

	t.Run("it emits no wait after command for any input", func(t *testing.T) {
		// The flag is dropped: the exec'd fallback shell now keeps the window
		// alive, so the property that produced the "Press any key to close the
		// terminal." dead-end is gone for every argv shape.
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

		// The wrapper's argv[0]/argv[1] render as the leading single-quoted words
		// inside the command property; the fallback tail's $SHELL double quotes are
		// AppleScript-escaped (`"` -> `\"`).
		if !strings.Contains(script, `command:"'bash' '-lc' `) {
			t.Errorf("script does not open with the bash -lc wrapper; script:\n%s", script)
		}
		if !strings.Contains(script, `exec \"$SHELL\" -il`) {
			t.Errorf("script does not carry the AppleScript-escaped exec fallback tail; script:\n%s", script)
		}
	})

	t.Run("it keeps a percent in the payload inert", func(t *testing.T) {
		// The single %s is a fmt.Sprintf ARGUMENT, never a format-verb source, so a
		// `%` in the payload passes through literally rather than being interpreted
		// as a (malformed) verb.
		script := ghosttyOpenScript([]string{"echo", "100%done"})

		if !strings.Contains(script, "100%done") {
			t.Errorf("script dropped the literal percent payload; script:\n%s", script)
		}
		if strings.Contains(script, "%!") {
			t.Errorf("script carries a fmt error verb (%%!); script:\n%s", script)
		}
	})

	t.Run("it preserves an argv element containing a space (spaced-session-name fix)", func(t *testing.T) {
		// A spaced session name ("My Project-abc123") must survive Ghostty's
		// bash -c word-split as ONE element inside the wrapper, not be shredded
		// into "My" and "Project-abc123".
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

		// The '\'' close-escape-reopen idiom, re-escaped by the outer render layer
		// then AppleScript-escaped, leaves the doubled-backslash signature '\\'' in
		// the embed — proof the nested single-quote escaping ran, NOT a naive
		// concatenation that would have let the first inner quote terminate the
		// outer quote and shred the payload.
		if !strings.Contains(embed, `'\\''`) {
			t.Fatalf("embed missing the doubled single-quote escape signature '\\\\''; embed:\n%s", embed)
		}

		// Full round-trip: reverse the AppleScript escape, decode the outer render,
		// and recover the exact wrapped argv.
		recovered := decodeRenderedArgv(reverseAppleScriptEscape(embed))
		if want := wrapWithShellFallback(cmd); !reflect.DeepEqual(recovered, want) {
			t.Fatalf("round-trip recovered %#v, want %#v", recovered, want)
		}

		// Peel the fallback suffix off the payload and decode the inner render to
		// prove the special-char passthrough element survived byte-identically —
		// the literal special bytes are not shredded across word boundaries.
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

		// PATH is NOT stripped by the wrap: the env-prefix fragments carry no
		// shell-special bytes, so they survive contiguously through both quoting
		// layers and the AppleScript escape.
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

// TestGhosttyEmbedGoldenLiteral pins the exact escaped bytes ghosttyEmbed emits
// against a hand-authored golden literal — the PRIMARY oracle (acceptance
// criterion #7). Unlike the decoder round-trips above (kept as supplementary
// coverage), it never recomputes the expected value from a production function,
// so a symmetric encode/decode escaping bug cannot pass falsely: a change in
// renderCommandString's single-quote nesting or ghosttyEmbed's AppleScript escape
// diverges from the frozen literal and fails.
func TestGhosttyEmbedGoldenLiteral(t *testing.T) {
	t.Run("it emits the exact '\\''-escaped, AppleScript-escaped body for the canonical attach argv", func(t *testing.T) {
		body := ghosttyEmbed(realAttachArgv())

		if body != wantAttachCommandBody {
			t.Fatalf("attach command body mismatch:\n got: %s\nwant: %s", body, wantAttachCommandBody)
		}

		// The golden IS what lands inside the AppleScript command:"…" property.
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
