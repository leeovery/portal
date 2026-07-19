package cmd

import (
	"errors"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
)

// resolveOpenSurfaces is the read-only classify engine for the multi-target open
// burst: it walks the ordered target set and resolves each element into ordered
// attach/mint surfaces, expanding session/alias globs to K surfaces that join the
// list IN PLACE and reducing every mint to a literal existing directory so the
// spawned window never re-resolves (spec § Burst exec-argv & mint responsibility —
// alias/zoxide/-p all reduce to the literal dir; the query never travels).
//
// It is STRICTLY READ-ONLY (spec § Atomic pre-flight): it only READS (session set,
// alias store, zoxide, filesystem existence) and performs no mint and no tmux
// mutation. Net-N dispatch (Task 3-6) and the aggregated abort message that
// consumes misses (Task 3-4) live elsewhere; this task produces only the engine.
//
// The third return value distinguishes the two failure modes:
//   - IMMEDIATE HARD ERROR (returned err, aborting the whole resolve): ONLY
//     resolver.ErrZoxideNotInstalled — an environment fault (zoxide unavailable),
//     not a per-target resolution failure. Reporting it once immediately is
//     clearer than listing each -z query as an unresolvable target.
//   - COLLECTED MISS (raw target appended to misses, resolve continues): EVERY
//     other non-resolution — bare total miss, session not-found, alias
//     unknown-key/glob-zero, alias/zoxide gone-dir, -p non-existent dir, -p
//     non-directory file, -z no-match. The aggregated pre-flight (Task 3-4)
//     reports every unresolvable target, so a -p file (an unresolvable target) is
//     better reported alongside the others than promoted to an immediate abort;
//     only ErrZoxideNotInstalled is a clean errors.Is sentinel to branch on.
//     (Planner note: the tentative split of "non-directory -p" as an immediate
//     hard error is deliberately folded into COLLECTED MISS here — flagged for
//     review.)
func resolveOpenSurfaces(qr *resolver.QueryResolver, targets []Target) (surfaces []spawn.Surface, misses []string, err error) {
	// collect classifies a []QueryResult from an All-variant into the ordered
	// surface/miss slices: a SessionResult is an attach surface (Value = name), a
	// PathResult is a mint surface (Value = the resolved literal dir), a MissResult
	// appends its raw target. These are the only three shapes the All-variants and
	// the mint pins produce.
	collect := func(results []resolver.QueryResult) {
		for _, r := range results {
			switch res := r.(type) {
			case *resolver.SessionResult:
				surfaces = append(surfaces, spawn.Surface{Kind: spawn.SurfaceAttach, Value: res.Name})
			case *resolver.PathResult:
				surfaces = append(surfaces, spawn.Surface{Kind: spawn.SurfaceMint, Value: res.Path})
			case *resolver.MissResult:
				misses = append(misses, res.Target)
			}
		}
	}

	for _, t := range targets {
		switch t.Domain {
		case "bare":
			// The bare guessing chain. Emit the single resolve decision line ONCE
			// per non-glob bare target (spec § Wrong-guess feedback), reusing
			// resolveDecision on the sole result — a non-glob ResolveBareAll always
			// returns exactly one result. Globs are deterministic (session-domain by
			// construction), not guesses, so they emit no line.
			results, _ := qr.ResolveBareAll(t.Value)
			if !resolver.HasGlobMeta(t.Value) {
				domain, resolvedPath := resolveDecision(results[0])
				resolveLogger.Info("resolved", "target", t.Value, "domain", domain, "resolved_path", resolvedPath)
			}
			collect(results)
		case "session":
			results, _ := qr.ResolveSessionPinAll(t.Value)
			collect(results)
		case "alias":
			results, _ := qr.ResolveAliasPinAll(t.Value)
			collect(results)
		case "path":
			// Single-domain: reuse the existing single-result pin. A resolution
			// failure (non-existent dir, non-directory file) is a collected miss.
			r, perr := qr.ResolvePathPin(t.Value)
			if perr != nil {
				misses = append(misses, t.Value)
				continue
			}
			collect([]resolver.QueryResult{r})
		case "zoxide":
			// Single-domain: reuse the existing single-result pin. Only
			// ErrZoxideNotInstalled (an environment fault) aborts the whole resolve;
			// a no-match or gone best-match dir is a collected miss.
			r, zerr := qr.ResolveZoxidePin(t.Value)
			if zerr != nil {
				if errors.Is(zerr, resolver.ErrZoxideNotInstalled) {
					return nil, nil, zerr
				}
				misses = append(misses, t.Value)
				continue
			}
			collect([]resolver.QueryResult{r})
		}
	}

	return surfaces, misses, nil
}
