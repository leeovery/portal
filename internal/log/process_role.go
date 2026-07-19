package log

import "strings"

// Process-role values. This is the CLOSED 6-value space for the per-record
// process_role baseline attr — every ResolveProcessRole result is exactly one of
// these. Identifies which portal binary invocation emitted a log line, which is
// critical for multi-writer disambiguation on reboot-recovery days.
const (
	roleDaemon   = "daemon"
	roleHydrate  = "hydrate"
	roleHooksCLI = "hooks_cli"
	roleClean    = "clean"
	roleTUI      = "tui"
	// roleBootstrap is the explicit default/fallback: any invocation not matched
	// by the table resolves here, so the closed space is fully covered and no
	// invocation is left unmapped.
	roleBootstrap = "bootstrap"
)

// ResolveProcessRole maps a process invocation's argument vector (os.Args[1:])
// to one of the six closed process_role values.
//
// Init is called from main before Cobra parses argv, so the role must be
// resolved from a lightweight os.Args inspection rather than a full parse. The
// algorithm strips flag tokens (anything starting with "-") so flags interleaved
// among the subcommand-path tokens are ignored, then longest-prefix-matches the
// leading subcommand-path tokens against a small static table with first-match,
// longest-prefix-wins semantics:
//
//	state daemon                      -> daemon
//	state hydrate / state signal-hydrate -> hydrate
//	hooks …                           -> hooks_cli
//	clean                             -> clean
//	open … / x … / bare               -> tui
//	anything else (incl. bare state)  -> bootstrap (explicit default)
//
// The function is PURE — it reads nothing global, so it is unit-testable without
// process state. main calls ResolveProcessRole(os.Args[1:]) and passes the
// result to log.Init.
func ResolveProcessRole(args []string) string {
	path := subcommandPath(args)

	// Longest-prefix first: the two-token `state …` arms are checked before any
	// single-token arm so the shared `state` prefix disambiguates correctly.
	if len(path) >= 2 && path[0] == "state" {
		switch path[1] {
		case "daemon":
			return roleDaemon
		case "hydrate", "signal-hydrate":
			return roleHydrate
		}
		// `state <unknown>` (and `state` with a non-matching second token) falls
		// through to the default below — it is neither daemon nor hydrate.
	}

	if len(path) == 0 {
		// Bare `portal` (no subcommand token at all) is the TUI picker.
		return roleTUI
	}

	switch path[0] {
	case "hook", "hooks":
		// `hook` is the canonical resume-hooks verb; `hooks` is its permanent
		// silent cobra alias (spec § Back-Compat). Both map to the same role so a
		// hook-mutation log line carries process_role=hooks_cli regardless of the
		// spelling the caller (incl. the machine-generated SessionStart skill) used.
		return roleHooksCLI
	case "clean":
		return roleClean
	case "open", "x":
		return roleTUI
	}

	return roleBootstrap
}

// subcommandPath returns args with flag tokens removed, preserving order. A flag
// token is any token beginning with "-" (covering both "-x" short flags and
// "--long" long flags); the lone "-" stdin convention is also treated as a flag
// since it is never a subcommand path token. The result is the leading
// subcommand-path tokens used for matching.
func subcommandPath(args []string) []string {
	path := make([]string, 0, len(args))
	for _, tok := range args {
		if strings.HasPrefix(tok, "-") {
			continue
		}
		path = append(path, tok)
	}
	return path
}
