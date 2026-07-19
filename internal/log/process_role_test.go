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

		// open / x / bare -> tui
		{"open path", []string{"open", "."}, "tui"},
		{"x alias", []string{"x"}, "tui"},
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

// TestResolveProcessRole_DriftTripwire is the drift guard tying the role table
// in process_role.go to the real portal command set. ResolveProcessRole derives
// the subcommand->role mapping from a hand-maintained longest-prefix match over
// os.Args because Init must run before Cobra parses argv (see process_role.go).
// That puts a SECOND, independent copy of command-routing knowledge in this
// package, divorced from cmd/'s Cobra registration. If a subcommand is renamed or
// added, the role table will NOT fail to compile — it will silently fall through
// to roleBootstrap, mis-attributing process_role (which the spec calls critical
// for multi-writer disambiguation on reboot-recovery days).
//
// CONTRIBUTOR NOTE: the verbs below mirror the Cobra command registration in
// cmd/ (root.AddCommand / stateCmd.AddCommand in cmd/state_daemon.go,
// cmd/state_hydrate.go, cmd/state_signal_hydrate.go, cmd/hooks.go, cmd/clean.go,
// cmd/open.go, ...). If you rename or add a subcommand that should
// map to a non-default role, you MUST update BOTH the table in
// ResolveProcessRole (process_role.go) AND the canonical argv shapes in this
// fixture. A removed/renamed mapping makes a non-default case here go red.
//
// An automated cross-boundary assertion enumerating the registered Cobra verbs
// directly is NOT possible: cmd imports internal/log, so importing cmd from this
// test would create an import cycle. This canonical-argv table is the substitute.
func TestResolveProcessRole_DriftTripwire(t *testing.T) {
	// One canonical argv shape per NON-DEFAULT role. Every distinct role the
	// table can route to (i.e. everything except the bootstrap fallback) MUST
	// appear here exactly once, so removing any single mapping in
	// ResolveProcessRole turns the corresponding case red rather than silently
	// degrading it to roleBootstrap.
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

	// Guard against the table acquiring a new non-default role that nobody wired
	// a canonical-argv case for. Every non-bootstrap role declared in
	// process_role.go must have been exercised above.
	for _, role := range []string{roleDaemon, roleHydrate, roleHooksCLI, roleClean, roleTUI} {
		if !seen[role] {
			t.Errorf("non-default role %q has no canonical-argv case in TestResolveProcessRole_DriftTripwire — add one", role)
		}
	}

	// The signal-hydrate verb also routes to hydrate and is part of the same
	// resolution arm; assert it explicitly so a divergence between the two
	// hydrate verbs is caught.
	if got := ResolveProcessRole([]string{"state", "signal-hydrate"}); got != roleHydrate {
		t.Errorf("ResolveProcessRole(state signal-hydrate) = %q, want %q", got, roleHydrate)
	}

	// Explicit default-fallback assertion. The bootstrap role is the INTENTIONAL
	// default for any unrouted verb (version, init, alias, bare `state`, and any
	// genuinely unknown token) — asserted here so the fallback is deliberate and
	// covered, NOT a silent catch-all that masks a dropped mapping.
	fallbackInputs := [][]string{
		{"version"},                    // registered cmd, no role -> default
		{"init", "zsh"},                // registered cmd, no role -> default
		{"alias", "set", "foo", "."},   // registered cmd, no role -> default
		{"state"},                      // bare `state` (no second token) -> default
		{"state", "wat"},               // unknown state subcommand -> default
		{"totally", "unknown", "verb"}, // never-registered verb -> default
	}
	for _, argv := range fallbackInputs {
		if got := ResolveProcessRole(argv); got != roleBootstrap {
			t.Errorf("ResolveProcessRole(%q) = %q, want %q (intentional default fallback)", argv, got, roleBootstrap)
		}
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
