package log

import "testing"

func TestResolveProcessRole(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		// state daemon -> daemon
		{"state daemon", []string{"state", "daemon"}, "daemon"},
		{"state daemon with trailing flag", []string{"state", "daemon", "--foreground"}, "daemon"},

		// state hydrate / state signal-hydrate -> hydrate
		{"state hydrate", []string{"state", "hydrate"}, "hydrate"},
		{"state signal-hydrate", []string{"state", "signal-hydrate"}, "hydrate"},

		// hooks ... -> hooks_cli
		{"hooks set on-resume", []string{"hooks", "set", "--on-resume", "x"}, "hooks_cli"},
		{"hooks alone", []string{"hooks"}, "hooks_cli"},

		// clean -> clean
		{"clean", []string{"clean"}, "clean"},
		{"clean with logs flag", []string{"clean", "--logs"}, "clean"},

		// open / x / attach / bare -> tui
		{"open path", []string{"open", "."}, "tui"},
		{"x alias", []string{"x"}, "tui"},
		{"attach foo", []string{"attach", "foo"}, "tui"},
		{"bare portal", []string{}, "tui"},
		{"only flags, no subcommand", []string{"--verbose"}, "tui"},

		// unknown -> bootstrap
		{"version", []string{"version"}, "bootstrap"},
		{"init", []string{"init"}, "bootstrap"},
		{"alias add", []string{"alias", "add"}, "bootstrap"},

		// state alone (no third token) falls through to bootstrap
		{"state alone", []string{"state"}, "bootstrap"},
		{"state unknown subcommand", []string{"state", "wat"}, "bootstrap"},

		// interleaved flags are ignored; match on path tokens only
		{"leading flag then state daemon", []string{"--verbose", "state", "daemon"}, "daemon"},
		{"flag between state and daemon", []string{"state", "--foo", "daemon"}, "daemon"},
		{"short flag between state and daemon", []string{"state", "-v", "daemon"}, "daemon"},
		{"leading flag then open", []string{"--verbose", "open", "."}, "tui"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveProcessRole(tc.args); got != tc.want {
				t.Errorf("ResolveProcessRole(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestResolveProcessRole_ClosedResultSpace asserts every result returned by the
// resolver is one of the six canonical role values — the closed space must never
// leak an out-of-band value regardless of input.
func TestResolveProcessRole_ClosedResultSpace(t *testing.T) {
	valid := map[string]bool{
		"daemon":    true,
		"hydrate":   true,
		"hooks_cli": true,
		"clean":     true,
		"tui":       true,
		"bootstrap": true,
	}

	inputs := [][]string{
		nil,
		{},
		{"--verbose"},
		{"state"},
		{"state", "daemon"},
		{"state", "hydrate"},
		{"state", "signal-hydrate"},
		{"hooks"},
		{"clean"},
		{"open", "."},
		{"x"},
		{"attach", "foo"},
		{"version"},
		{"totally", "unknown", "thing"},
		{"--a", "--b", "--c"},
	}

	for _, args := range inputs {
		got := ResolveProcessRole(args)
		if !valid[got] {
			t.Errorf("ResolveProcessRole(%q) = %q, not in the closed 6-value space", args, got)
		}
	}
}
