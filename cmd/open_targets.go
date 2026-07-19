package cmd

import "strings"

// Target is one element of the ordered open target-set union: a value plus the
// domain it resolves under. Domain is a plain-string domain matching the
// resolver's vocabulary ("session"/"path"/"zoxide"/"alias") plus "bare" for a
// positional that runs the full precedence chain.
type Target struct {
	Value  string
	Domain string
}

// openTargetPins maps every known value-taking open flag (both long and short
// forms) to the Target domain its value belongs to. An excluded flag
// (-e/--exec, -f/--filter, --ack) maps to the empty domain: its value is still
// consumed off the argv, but it is never emitted as a target. Any flag-like
// token absent from this map is a flag cobra already validated (e.g. a boolean
// or --help) and is skipped without consuming a following value.
var openTargetPins = map[string]string{
	"-s": "session", "--session": "session",
	"-p": "path", "--path": "path",
	"-z": "zoxide", "--zoxide": "zoxide",
	"-a": "alias", "--alias": "alias",
	"-e": "", "--exec": "",
	"-f": "", "--filter": "",
	"--ack": "",
}

// orderedOpenTargets recovers the left-to-right union of positionals and every
// -s/-p/-z/-a pin occurrence from a raw open argv slice (the tokens following
// `portal open`, e.g. []string{"-s", "api", "~/new"}). cobra's StringP collapses
// repeated same-flag values and splits positionals from flags, losing true
// interleaved order and repeats; this raw scan preserves both.
//
// It is a pure classifier, not a validator: it assumes cobra already accepted
// the argv (RunE runs after the cobra parse), so it never rejects a token — it
// only attributes each to its domain. -e/--exec, -f/--filter, and --ack values
// are consumed but never emitted; everything after a bare `--` (command
// passthrough) is dropped. No dedup — repeats are honoured as intent.
func orderedOpenTargets(args []string) []Target {
	var targets []Target
	for i := 0; i < len(args); i++ {
		tok := args[i]

		// A bare `--` terminates flag/target parsing; everything after it is
		// command-passthrough, never a target.
		if tok == "--" {
			break
		}

		// A bare token (not a flag) is a positional target that runs the full
		// precedence chain. A lone "-" is not a flag either. The spec guarantees
		// no positional begins with "-".
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			targets = append(targets, Target{Value: tok, Domain: "bare"})
			continue
		}

		// Flag-like token. Split the equals form (-s=api / --session=api) on the
		// FIRST '='; the space form leaves value empty until the next token.
		name, value, hasInlineValue := strings.Cut(tok, "=")

		domain, known := openTargetPins[name]
		if !known {
			// A flag cobra already validated but that Portal's target scan does
			// not model (boolean, --help, …). Its arity is unknown, so skip it
			// WITHOUT consuming a following token as a value.
			continue
		}

		// Space form: the NEXT token is this flag's value; consume it so it is
		// never re-examined as a positional or flag. Consumed even for excluded
		// flags (domain == "") — only the emission is suppressed.
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
