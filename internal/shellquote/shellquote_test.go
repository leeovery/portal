package shellquote_test

import (
	"testing"

	"github.com/leeovery/portal/internal/shellquote"
)

func TestSingle(t *testing.T) {
	t.Run("it wraps a plain string in single quotes", func(t *testing.T) {
		got := shellquote.Single("/usr/local/bin/portal")
		const want = `'/usr/local/bin/portal'`
		if got != want {
			t.Errorf("Single = %q, want %q", got, want)
		}
	})

	t.Run("it re-quotes an embedded single quote", func(t *testing.T) {
		got := shellquote.Single("it's")
		const want = `'it'\''s'`
		if got != want {
			t.Errorf("Single = %q, want %q", got, want)
		}
	})

	t.Run("it quotes an empty string", func(t *testing.T) {
		got := shellquote.Single("")
		const want = `''`
		if got != want {
			t.Errorf("Single = %q, want %q (an empty argument must survive as one word)", got, want)
		}
	})
}

func TestJoin(t *testing.T) {
	t.Run("it POSIX-single-quotes each element and space-joins them", func(t *testing.T) {
		argv := []string{"/usr/bin/env", "-u", "TMUX", "-u", "TMUX_PANE", "PATH=/b", "/abs/portal", "open", "--session", "proj-x", "--ack", "b1:t1"}

		got := shellquote.Join(argv)

		want := "'/usr/bin/env' '-u' 'TMUX' '-u' 'TMUX_PANE' 'PATH=/b' '/abs/portal' 'open' '--session' 'proj-x' '--ack' 'b1:t1'"
		if got != want {
			t.Errorf("Join = %q, want %q", got, want)
		}
	})

	t.Run("it keeps an element containing a space as one quoted word so a shell re-split reproduces the argv", func(t *testing.T) {
		// A session name can carry a space; a naive space-join would let a
		// downstream shell re-split it and shred the attach target.
		got := shellquote.Join([]string{"/abs/portal", "open", "My Project-abc123"})

		want := "'/abs/portal' 'open' 'My Project-abc123'"
		if got != want {
			t.Errorf("Join = %q, want %q (spaced element stays one quoted word)", got, want)
		}
	})

	t.Run("it escapes an embedded single quote with the close-escape-reopen sequence", func(t *testing.T) {
		got := shellquote.Join([]string{"it's"})

		want := `'it'\''s'`
		if got != want {
			t.Errorf("Join = %q, want %q", got, want)
		}
	})

	t.Run("it renders an empty argv as the empty string", func(t *testing.T) {
		got := shellquote.Join(nil)
		if got != "" {
			t.Errorf("Join = %q, want the empty string", got)
		}
	})

	t.Run("it keeps an empty element as an empty quoted word", func(t *testing.T) {
		got := shellquote.Join([]string{"portal", ""})

		const want = `'portal' ''`
		if got != want {
			t.Errorf("Join = %q, want %q (an empty argument must survive as one word)", got, want)
		}
	})
}

func TestReadable(t *testing.T) {
	t.Run("it leaves a word a shell reads back unchanged bare", func(t *testing.T) {
		got := shellquote.Readable([]string{"state", "hydrate", "--session", "proj-x_1", "--file", "/a/b.bin", "--ack", "b1:t1", "PATH=/b", "a@b%c+d,e"})

		const want = "state hydrate --session proj-x_1 --file /a/b.bin --ack b1:t1 PATH=/b a@b%c+d,e"
		if got != want {
			t.Errorf("Readable = %q, want %q", got, want)
		}
	})

	t.Run("it quotes a word a shell would split, expand or glob", func(t *testing.T) {
		got := shellquote.Readable([]string{"--session", "portal-xn85z5 zmw rework", "$HOME", "~/x", "*.go", "a;b", "it's", "café"})

		want := `--session 'portal-xn85z5 zmw rework' '$HOME' '~/x' '*.go' 'a;b' 'it'\''s' 'café'`
		if got != want {
			t.Errorf("Readable = %q, want %q", got, want)
		}
	})

	t.Run("it keeps an empty element as an empty quoted word", func(t *testing.T) {
		if got, want := shellquote.Readable([]string{"portal", ""}), `portal ''`; got != want {
			t.Errorf("Readable = %q, want %q (an empty argument must survive as one word)", got, want)
		}
	})

	t.Run("it renders an empty argv as the empty string", func(t *testing.T) {
		if got := shellquote.Readable(nil); got != "" {
			t.Errorf("Readable = %q, want the empty string", got)
		}
	})
}
