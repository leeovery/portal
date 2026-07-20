package cmd

import (
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

// runOpenBurstFunc is the seam the multi-target dispatch calls to open N≥2
// surfaces. Tests override it to capture the surfaces it is handed and assert
// routing without spawning host windows.
var runOpenBurstFunc = runOpenBurst

// runOpenBurst opens the N≥2 resolved surfaces of a multi-target open — spawning
// the N−1 non-trigger windows FIRST and self-connecting the trigger LAST (spec §
// The trigger absorbs the first target; § Burst exec-argv & mint responsibility).
// It is the thin production entry point: it resolves the burst seams via
// buildOpenBurstDeps and delegates to runOpenBurstWithDeps, which holds the
// testable body.
func runOpenBurst(cmd *cobra.Command, surfaces []spawn.Surface, command []string) error {
	return runOpenBurstWithDeps(cmd, surfaces, command, buildOpenBurstDeps(cmd))
}

// openRawArgs returns the process's raw argv. The multi-target gate needs the
// RAW args (not cobra's parsed buckets) to recover true left-to-right target
// order and repeated same-flag values (`-s a -s b`), both of which cobra
// collapses. Tests override it to inject a known argv.
var openRawArgs = func() []string { return os.Args }

// openOwnArgs strips the process-name + subcommand prefix from the raw argv,
// yielding just open's own args — the tokens a human typed after `portal open`.
//
// ASSUMPTION (documented, pragmatic — flagged for review): Portal is invoked as
// `portal open …`, with no value-taking global/persistent flag before the
// `open` subcommand token (none exist). The scan skips argv[0] (the process
// name) and returns everything AFTER the first `open` token. Absent an `open`
// token — e.g. under `go test`, where os.Args is the test binary's own argv —
// it returns nil, so the multi-target gate is inert and the single-target path
// is preserved byte-for-byte.
func openOwnArgs() []string {
	raw := openRawArgs()
	for i := 1; i < len(raw); i++ {
		if raw[i] == "open" {
			return raw[i+1:]
		}
	}
	return nil
}

// isMultiTarget decides whether an ordered target set routes through the burst
// resolver (spec § The trigger absorbs the first target; § Glob targets):
//   - 2+ targets always burst.
//   - a SINGLE glob-expandable target (a bare/session/alias value carrying glob
//     metacharacters) also bursts, because it may expand to K≥2 surfaces — this
//     overrides Phase 1's single-glob first-match.
//   - everything else (zero targets, or a single non-glob / -p / -z target) is
//     NOT multi and falls through to the unchanged single-target path.
func isMultiTarget(ordered []Target) bool {
	if len(ordered) >= 2 {
		return true
	}
	if len(ordered) == 1 {
		t := ordered[0]
		return globExpandableDomain(t.Domain) && resolver.HasGlobMeta(t.Value)
	}
	return false
}

// globExpandableDomain reports whether a domain expands a glob value against a
// finite Portal-owned namespace. Bare positionals are session-domain by glob
// construction; -s and -a expand over the session-name / alias-key sets. -p and
// -z never glob-expand (a literal path / a zoxide subsequence query).
func globExpandableDomain(domain resolver.Domain) bool {
	switch domain {
	case resolver.DomainBare, resolver.DomainSession, resolver.DomainAlias:
		return true
	default:
		return false
	}
}

// aggregatedMissError is the MULTI-target (N≥2) pre-flight abort message: it
// echoes the single-target "nothing resolved for" stem WITHOUT the -f suffix,
// because -f is mutually exclusive with targets and so cannot carry a
// multi-target intent (spec § Atomic pre-flight & partial failure). Every
// unresolvable target is listed via spawn.QuoteJoin so one re-run can fix them
// all. A plain (non-usage) error → exit 1.
func aggregatedMissError(misses []string) error {
	return fmt.Errorf("nothing resolved for: %s", spawn.QuoteJoin(misses))
}

// commandAttachOnlyMessage is the user-facing usage-error text for the
// command-on-attach guard. It is the SOLE authoring site for this wording,
// consumed by BOTH the single-target guard (openResolved's *SessionResult arm,
// cmd/open.go) and the multi-target zero-mint guard (runOpenBurstWithDeps,
// cmd/open_burst_run.go — the arity of the same rule) so the two cannot drift.
const commandAttachOnlyMessage = "a command (-e/--) can only run in a newly-created session, not an existing one"

// singleMissError is the single-target (N=1) "nothing resolved" hard-fail
// message (spec § Miss handling): it keeps the Phase-1 -f escape-hatch
// suggestion, substituting the target TWICE (quoted, then after -f). It is the
// SOLE authoring site for this wording — both the bare-positional path
// (cmd/open.go) and the N=1 glob-expanding-to-zero burst arm call it so the two
// cannot drift. The em-dash is U+2014. A plain (non-usage) error → exit 1.
func singleMissError(target string) error {
	return fmt.Errorf("nothing resolved for '%s' — try -f %s", target, target)
}

// dispatchOpenBurst runs the atomic read-only pre-flight for a multi-target open
// and routes the outcome (spec § Atomic pre-flight & partial failure):
//
//  1. build the query resolver;
//  2. resolve the whole ordered set READ-ONLY — an ErrZoxideNotInstalled err is
//     an immediate abort (nothing opens);
//  3. ANY miss aborts the whole set atomically BEFORE any surface opens — the
//     abort reports every unresolvable target. Arity keys the wording: a lone
//     target (a single glob expanding to zero) keeps the Phase-1 -f suggestion;
//     2+ targets use the aggregated (no -f) message;
//  4. a single surviving surface degenerates to a single connect through
//     openResolved (so the Task 2-6 command-on-attach guard + ack write + the
//     inside/outside dispatch all apply);
//  5. 2+ surviving surfaces run the burst.
func dispatchOpenBurst(cmd *cobra.Command, ordered []Target, command []string) error {
	qr, err := buildQueryResolver(cmd)
	if err != nil {
		return err
	}

	surfaces, results, misses, err := resolveOpenSurfaces(qr, ordered)
	if err != nil {
		// Only resolver.ErrZoxideNotInstalled reaches here — an environment fault
		// that aborts the whole resolve immediately; nothing opens.
		return err
	}

	if len(misses) > 0 {
		if len(ordered) == 1 {
			// A single glob expanding to zero is N=1 arity — keep the -f hint.
			return singleMissError(misses[0])
		}
		return aggregatedMissError(misses)
	}

	if len(surfaces) == 1 {
		// Degenerate single-surviving surface (only reachable from a single glob
		// expanding to exactly one match). Thread the TRUE resolver result —
		// results[0] is the exact QueryResult that produced surfaces[0], carrying
		// its real Domain provenance (DomainGlob for an attach, DomainAlias for a
		// mint) — rather than fabricating a domain from the lossy Surface. The
		// upstream resolve-decision line already fired in resolveOpenSurfaces; this
		// reuse of openResolved is only for the command guard + ack write + connector
		// dispatch, none of which read Domain, so behaviour is unchanged.
		return openResolved(cmd, results[0], command)
	}

	return runOpenBurstFunc(cmd, surfaces, command)
}
