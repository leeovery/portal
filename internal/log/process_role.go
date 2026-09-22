package log

import "strings"

// The closed value space for the process_role baseline attr. roleClean is
// unreachable and its ResolveProcessRole arm is dead; both stay, because pruning
// a value from the closed space is a log-taxonomy change, not a cleanup.
const (
	roleDaemon    = "daemon"
	roleHydrate   = "hydrate"
	roleHooksCLI  = "hooks_cli"
	roleClean     = "clean"
	roleTUI       = "tui"
	roleBootstrap = "bootstrap"
)

// ResolveProcessRole maps os.Args[1:] to one of the closed process_role values,
// defaulting to bootstrap. It inspects argv rather than parsing it because Init
// runs from main before Cobra sees argv.
func ResolveProcessRole(args []string) string {
	path := subcommandPath(args)

	if len(path) >= 2 && path[0] == "state" {
		switch path[1] {
		case "daemon":
			return roleDaemon
		case "hydrate", "signal-hydrate", "resume-draw", "resume-wait", "resume-recover":
			return roleHydrate
		}
	}

	if len(path) == 0 {
		// Bare `portal` prints help; it does not launch the picker. roleTUI is kept
		// here only so the closed role space stays stable.
		return roleTUI
	}

	switch path[0] {
	case "hook", "hooks":
		return roleHooksCLI
	// Dead arm, retained with roleClean: see the const block.
	case "clean":
		return roleClean
	case "open", "x":
		return roleTUI
	}

	return roleBootstrap
}

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
