package cmd

import (
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/spf13/cobra"
)

var runOpenBurstFunc = runOpenBurst

// runOpenBurst opens surfaces, trigger being what the pre-flight resolved the
// first of them, the trigger, to.
func runOpenBurst(cmd *cobra.Command, surfaces []spawn.Surface, trigger resolver.QueryResult, command []string) error {
	deps, err := buildOpenBurstDeps(cmd, trigger)
	if err != nil {
		return err
	}
	return runOpenBurstWithDeps(cmd, surfaces, command, deps)
}

var openRawArgs = func() []string { return os.Args }

// openOwnArgs assumes no value-taking flag precedes the `open` token, which
// holds because Portal declares no such global flag. With no `open` token at all
// it returns nil, leaving the multi-target gate inert.
func openOwnArgs() []string {
	raw := openRawArgs()
	for i := 1; i < len(raw); i++ {
		if raw[i] == "open" {
			return raw[i+1:]
		}
	}
	return nil
}

// A single glob-expandable target counts as multi because it may expand to K≥2
// surfaces.
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

// A domain glob-expands only over a finite Portal-owned namespace: -p is a
// literal path and -z a zoxide subsequence query, so neither qualifies.
func globExpandableDomain(domain resolver.Domain) bool {
	switch domain {
	case resolver.DomainBare, resolver.DomainSession, resolver.DomainAlias:
		return true
	default:
		return false
	}
}

// The multi-target abort drops the single-target -f suggestion: -f is mutually
// exclusive with targets, so it cannot carry a multi-target intent.
func aggregatedMissError(misses []string) error {
	return fmt.Errorf("nothing resolved for: %s", spawn.QuoteJoin(misses))
}

const commandAttachOnlyMessage = "a command (-e/--) can only run in a newly-created session, not an existing one"

func singleMissError(query string) error {
	return fmt.Errorf("nothing resolved for '%s' — try -f %s", query, query)
}

// dispatchOpenBurst resolves the whole ordered target set read-only before
// anything opens, so any miss aborts the set atomically.
func dispatchOpenBurst(cmd *cobra.Command, ordered []Target, command []string) error {
	qr, err := buildQueryResolver(cmd)
	if err != nil {
		return err
	}
	return openBurstBy(cmd, qr, ordered, command)
}

// openBurstBy is dispatchOpenBurst over qr. A single surface that is a kept
// session is brought back, or, kept no longer under bootstrap.lock, the set is
// resolved again as if it had never been kept.
func openBurstBy(cmd *cobra.Command, qr *resolver.QueryResolver, ordered []Target, command []string) error {
	surfaces, results, misses, err := resolveOpenSurfaces(qr, ordered)
	if err != nil {
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
		// results[0] is threaded through rather than a domain reconstructed from
		// the lossy Surface, so the real provenance survives the degenerate case.
		return openOr(cmd, results[0], command, func() error {
			return openBurstBy(cmd, qr.WithKept(nil), ordered, command)
		})
	}

	return runOpenBurstFunc(cmd, surfaces, results[0], command)
}
