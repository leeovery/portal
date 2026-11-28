package cmd

import (
	"strings"

	"github.com/leeovery/portal/internal/resolver"
)

// DomainBare marks a positional that runs the full precedence chain.
type Target struct {
	Value  string
	Domain resolver.Domain
}

// openTargetPins is a hand-maintained mirror of openCmd's live flag set: a new
// value-taking flag needs an entry here (long form and any short form), or
// orderedOpenTargets treats it as arity-0 and misroutes its value as a bare
// positional. An excluded flag maps to the empty domain — its value is consumed
// off the argv but never emitted as a target. Bundled value shorthands (`-sf`)
// are deliberately absent, so such a token is skipped rather than attributed a
// value; that divergence from cobra's bundling is intended.
var openTargetPins = map[string]resolver.Domain{
	"-s": resolver.DomainSession, "--session": resolver.DomainSession,
	"-p": resolver.DomainPath, "--path": resolver.DomainPath,
	"-z": resolver.DomainZoxide, "--zoxide": resolver.DomainZoxide,
	"-a": resolver.DomainAlias, "--alias": resolver.DomainAlias,
	"-e": "", "--exec": "",
	"-f": "", "--filter": "",
	"--ack": "", "--cwd-file": "",
}

// orderedOpenTargets recovers the interleaved order and repeats that cobra's
// StringP parsing loses. It is a pure classifier, not a validator: cobra already
// accepted the argv, so no token is rejected, only attributed. Repeats are
// honoured, never deduped.
func orderedOpenTargets(args []string) []Target {
	var targets []Target
	for i := 0; i < len(args); i++ {
		tok := args[i]

		// Everything after a bare `--` is command-passthrough, never a target.
		if tok == "--" {
			break
		}

		// A lone "-" is a positional, not a flag; no positional begins with "-".
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			targets = append(targets, Target{Value: tok, Domain: resolver.DomainBare})
			continue
		}

		// The -s=api form splits here; the space form leaves value empty for the
		// next token to fill.
		name, value, hasInlineValue := strings.Cut(tok, "=")

		domain, known := openTargetPins[name]
		if !known {
			// An unmodelled flag (boolean, --help, …) has unknown arity, so skip it
			// without consuming a following token as its value.
			continue
		}

		// Consumed even for excluded flags, so the value is never re-examined as a
		// positional; only the emission is suppressed.
		if !hasInlineValue && i+1 < len(args) {
			value = args[i+1]
			i++
		}

		if domain != "" {
			targets = append(targets, Target{Value: value, Domain: domain})
		}
	}
	return targets
}
