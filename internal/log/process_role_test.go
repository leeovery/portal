package log

import "testing"

func TestResolveProcessRole(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"state daemon", []string{"state", "daemon"}, "daemon"},
		{"state daemon with trailing flag", []string{"state", "daemon", "--foreground"}, "daemon"},

		{"state hydrate", []string{"state", "hydrate"}, "hydrate"},
		{"state signal-hydrate", []string{"state", "signal-hydrate"}, "hydrate"},
		{"state resume-draw", []string{"state", "resume-draw"}, "hydrate"},
		{"state resume-wait", []string{"state", "resume-wait"}, "hydrate"},
		{"state resume-recover", []string{"state", "resume-recover"}, "hydrate"},

		{"hook set on-resume", []string{"hook", "set", "--on-resume", "x"}, "hooks_cli"},
		{"hook alone", []string{"hook"}, "hooks_cli"},
		{"hooks set on-resume (alias)", []string{"hooks", "set", "--on-resume", "x"}, "hooks_cli"},
		{"hooks alone (alias)", []string{"hooks"}, "hooks_cli"},

		{"clean", []string{"clean"}, "clean"},
		{"clean with logs flag", []string{"clean", "--logs"}, "clean"},

		{"open path", []string{"open", "."}, "tui"},
		{"x alias", []string{"x"}, "tui"},
		{"bare portal", []string{}, "tui"},
		{"only flags, no subcommand", []string{"--verbose"}, "tui"},

		{"version", []string{"version"}, "bootstrap"},
		{"init", []string{"init"}, "bootstrap"},
		{"alias add", []string{"alias", "add"}, "bootstrap"},

		{"state alone", []string{"state"}, "bootstrap"},
		{"state unknown subcommand", []string{"state", "wat"}, "bootstrap"},

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

func TestResolveProcessRole_DriftTripwire(t *testing.T) {
	perRole := []struct {
		role string
		argv []string
	}{
		{roleDaemon, []string{"state", "daemon"}},
		{roleHydrate, []string{"state", "hydrate"}},
		{roleHooksCLI, []string{"hooks", "set", "--on-resume", "x"}},
		{roleClean, []string{"clean"}},
		{roleTUI, []string{"open", "."}},
	}

	seen := map[string]bool{}
	for _, tc := range perRole {
		t.Run(tc.role, func(t *testing.T) {
			if got := ResolveProcessRole(tc.argv); got != tc.role {
				t.Errorf("ResolveProcessRole(%q) = %q, want %q — role mapping drifted from the cmd/ command set", tc.argv, got, tc.role)
			}
		})
		seen[tc.role] = true
	}

	for _, role := range []string{roleDaemon, roleHydrate, roleHooksCLI, roleClean, roleTUI} {
		if !seen[role] {
			t.Errorf("non-default role %q has no canonical-argv case in TestResolveProcessRole_DriftTripwire — add one", role)
		}
	}

	if got := ResolveProcessRole([]string{"state", "signal-hydrate"}); got != roleHydrate {
		t.Errorf("ResolveProcessRole(state signal-hydrate) = %q, want %q", got, roleHydrate)
	}

	fallbackInputs := [][]string{
		{"version"},
		{"init", "zsh"},
		{"alias", "set", "foo", "."},
		{"state"},
		{"state", "wat"},
		{"totally", "unknown", "verb"},
	}
	for _, argv := range fallbackInputs {
		if got := ResolveProcessRole(argv); got != roleBootstrap {
			t.Errorf("ResolveProcessRole(%q) = %q, want %q (intentional default fallback)", argv, got, roleBootstrap)
		}
	}
}

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
