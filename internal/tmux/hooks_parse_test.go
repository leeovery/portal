package tmux_test

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tmux"
)

func TestParseShowHooks(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		// want is keyed by event; each value is the ordered slice of HookEntry expected.
		want map[string][]tmux.HookEntry
	}{
		{
			name: "it returns an empty map for empty input",
			raw:  "",
			want: map[string][]tmux.HookEntry{},
		},
		{
			name: "it parses a single session-created entry",
			raw:  `session-created[0] => run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`,
			want: map[string][]tmux.HookEntry{
				"session-created": {
					{Index: 0, Command: `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`},
				},
			},
		},
		{
			name: "it parses multiple entries on the same event in index order",
			raw: strings.Join([]string{
				`session-created[2] => run-shell "third"`,
				`session-created[0] => run-shell "first"`,
				`session-created[1] => run-shell "second"`,
			}, "\n"),
			want: map[string][]tmux.HookEntry{
				"session-created": {
					{Index: 0, Command: `run-shell "first"`},
					{Index: 1, Command: `run-shell "second"`},
					{Index: 2, Command: `run-shell "third"`},
				},
			},
		},
		{
			name: "it handles sparse indices left by prior removals",
			raw:  `session-created[2] => run-shell "only-survivor"`,
			want: map[string][]tmux.HookEntry{
				"session-created": {
					{Index: 2, Command: `run-shell "only-survivor"`},
				},
			},
		},
		{
			name: "it parses every hyphenated event name Portal registers",
			raw: strings.Join([]string{
				`session-created[0] => run-shell "a"`,
				`session-closed[0] => run-shell "b"`,
				`session-renamed[0] => run-shell "c"`,
				`client-attached[0] => run-shell "d"`,
				`client-detached[0] => run-shell "e"`,
				`client-session-changed[0] => run-shell "f"`,
				`pane-focus-out[0] => run-shell "g"`,
				`window-layout-changed[0] => run-shell "h"`,
			}, "\n"),
			want: map[string][]tmux.HookEntry{
				"session-created":        {{Index: 0, Command: `run-shell "a"`}},
				"session-closed":         {{Index: 0, Command: `run-shell "b"`}},
				"session-renamed":        {{Index: 0, Command: `run-shell "c"`}},
				"client-attached":        {{Index: 0, Command: `run-shell "d"`}},
				"client-detached":        {{Index: 0, Command: `run-shell "e"`}},
				"client-session-changed": {{Index: 0, Command: `run-shell "f"`}},
				"pane-focus-out":         {{Index: 0, Command: `run-shell "g"`}},
				"window-layout-changed":  {{Index: 0, Command: `run-shell "h"`}},
			},
		},
		{
			name: "it tolerates leading whitespace on each line",
			raw: strings.Join([]string{
				`   session-created[0] => run-shell "a"`,
				"\t" + `client-attached[0] => run-shell "b"`,
			}, "\n"),
			want: map[string][]tmux.HookEntry{
				"session-created": {{Index: 0, Command: `run-shell "a"`}},
				"client-attached": {{Index: 0, Command: `run-shell "b"`}},
			},
		},
		{
			name: "it silently skips unrelated or malformed lines",
			raw: strings.Join([]string{
				`# this is a comment`,
				`not-a-hook-line`,
				`session-created[0] => run-shell "a"`,
				`12345 => garbage`,
				``,
			}, "\n"),
			want: map[string][]tmux.HookEntry{
				"session-created": {{Index: 0, Command: `run-shell "a"`}},
			},
		},
		{
			name: "it silently skips entries with non-numeric index",
			raw: strings.Join([]string{
				`session-created[abc] => run-shell "skip-me"`,
				`session-created[1] => run-shell "keep-me"`,
			}, "\n"),
			want: map[string][]tmux.HookEntry{
				"session-created": {{Index: 1, Command: `run-shell "keep-me"`}},
			},
		},
		{
			name: "it accepts both => and bare-whitespace separators",
			raw: strings.Join([]string{
				`session-created[0] => run-shell "arrow"`,
				`session-closed[0] run-shell "bare"`,
			}, "\n"),
			want: map[string][]tmux.HookEntry{
				"session-created": {{Index: 0, Command: `run-shell "arrow"`}},
				"session-closed":  {{Index: 0, Command: `run-shell "bare"`}},
			},
		},
		{
			name: "it preserves the inner command substring across tmux outer-quoting variations",
			raw: strings.Join([]string{
				`session-created[0] => "run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'"`,
				`session-created[1] => 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'`,
			}, "\n"),
			want: map[string][]tmux.HookEntry{
				"session-created": {
					{Index: 0, Command: `run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'`},
					{Index: 1, Command: `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`},
				},
			},
		},
		{
			name: "it returns portal state notify substring intact inside a double-quoted command",
			raw:  `session-created[0] => "run-shell \"command -v portal >/dev/null 2>&1 && portal state notify\""`,
			want: map[string][]tmux.HookEntry{
				"session-created": {
					{Index: 0, Command: `run-shell \"command -v portal >/dev/null 2>&1 && portal state notify\"`},
				},
			},
		},
		{
			name: "it returns portal state notify substring intact inside a single-quoted command",
			raw:  `session-created[0] => 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'`,
			want: map[string][]tmux.HookEntry{
				"session-created": {
					{Index: 0, Command: `run-shell "command -v portal >/dev/null 2>&1 && portal state notify"`},
				},
			},
		},
		{
			name: "it does not strip mismatched outer quotes",
			raw:  `session-created[0] => "foo'`,
			want: map[string][]tmux.HookEntry{
				"session-created": {{Index: 0, Command: `"foo'`}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tmux.ParseShowHooks(tt.raw)

			if got == nil {
				t.Fatal("ParseShowHooks returned nil; want non-nil map")
			}

			if len(got) != len(tt.want) {
				t.Fatalf("got %d events %v, want %d events %v", len(got), keysOf(got), len(tt.want), keysOf(tt.want))
			}

			for event, wantEntries := range tt.want {
				gotEntries, ok := got[event]
				if !ok {
					t.Errorf("missing event %q in result", event)
					continue
				}
				if len(gotEntries) != len(wantEntries) {
					t.Errorf("event %q: got %d entries %v, want %d entries %v", event, len(gotEntries), gotEntries, len(wantEntries), wantEntries)
					continue
				}
				for i, want := range wantEntries {
					if gotEntries[i].Index != want.Index {
						t.Errorf("event %q entry %d: Index = %d, want %d", event, i, gotEntries[i].Index, want.Index)
					}
					if gotEntries[i].Command != want.Command {
						t.Errorf("event %q entry %d: Command = %q, want %q", event, i, gotEntries[i].Command, want.Command)
					}
				}
			}
		})
	}
}

func TestParseShowHooks_PortalSubstringRecoverable(t *testing.T) {
	// Spec acceptance: `portal state notify` substring matchable across both
	// quoting variants. This guards against accidental over-stripping that
	// would mangle the substring used by content-based idempotency.
	t.Run("portal state notify substring is matchable across quoting variants", func(t *testing.T) {
		raw := strings.Join([]string{
			`session-created[0] => 'run-shell "command -v portal >/dev/null 2>&1 && portal state notify"'`,
			`session-closed[0] => "run-shell 'command -v portal >/dev/null 2>&1 && portal state notify'"`,
		}, "\n")

		got := tmux.ParseShowHooks(raw)

		for _, event := range []string{"session-created", "session-closed"} {
			entries, ok := got[event]
			if !ok || len(entries) != 1 {
				t.Fatalf("event %q: expected 1 entry, got %d", event, len(entries))
			}
			if !strings.Contains(entries[0].Command, "portal state notify") {
				t.Errorf("event %q: Command %q does not contain 'portal state notify'", event, entries[0].Command)
			}
		}
	})
}

func keysOf(m map[string][]tmux.HookEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
