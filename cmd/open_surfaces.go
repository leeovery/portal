package cmd

import (
	"errors"

	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/spawn"
)

// Strictly read-only — no mint, no tmux mutation — so the pre-flight can abort a
// burst with nothing opened. Every mint surface carries a literal existing
// directory: the query must never travel to the spawned window.
func resolveOpenSurfaces(qr *resolver.QueryResolver, targets []Target) (surfaces []spawn.Surface, results []resolver.QueryResult, misses []string, err error) {
	// results stays in lockstep with surfaces: results[i] is the resolver result
	// that produced surfaces[i], carrying the true domain provenance a Surface loses.
	collect := func(classified []resolver.QueryResult) {
		for _, r := range classified {
			switch res := r.(type) {
			case *resolver.SessionResult:
				surfaces = append(surfaces, spawn.Surface{Kind: spawn.SurfaceAttach, Value: res.Name})
				results = append(results, res)
			case *resolver.KeptResult:
				// Its window runs open --session, which brings it back, as the
				// trigger's connect does.
				surfaces = append(surfaces, spawn.Surface{Kind: spawn.SurfaceAttach, Value: res.Name})
				results = append(results, res)
			case *resolver.PathResult:
				surfaces = append(surfaces, spawn.Surface{Kind: spawn.SurfaceMint, Value: res.Path})
				results = append(results, res)
			case *resolver.MissResult:
				misses = append(misses, res.Target)
			}
		}
	}

	for _, t := range targets {
		switch t.Domain {
		case resolver.DomainBare:
			// A glob is deterministic, not a guess: emitResolveDecision gates it itself,
			// so results[0] only ever reads a single-result non-glob resolve.
			results, _ := qr.ResolveBareAll(t.Value)
			emitResolveDecision(t.Value, results[0])
			collect(results)
		case resolver.DomainSession:
			results, _ := qr.ResolveSessionPinAll(t.Value)
			collect(results)
		case resolver.DomainAlias:
			results, _ := qr.ResolveAliasPinAll(t.Value)
			collect(results)
		case resolver.DomainPath:
			r, perr := qr.ResolvePathPin(t.Value)
			if perr != nil {
				misses = append(misses, t.Value)
				continue
			}
			collect([]resolver.QueryResult{r})
		case resolver.DomainZoxide:
			// A missing zoxide is an environment fault, not a per-target miss, so it
			// aborts the whole resolve rather than joining the collected misses.
			r, zerr := qr.ResolveZoxidePin(t.Value)
			if zerr != nil {
				if errors.Is(zerr, resolver.ErrZoxideNotInstalled) {
					return nil, nil, nil, zerr
				}
				misses = append(misses, t.Value)
				continue
			}
			collect([]resolver.QueryResult{r})
		}
	}

	return surfaces, results, misses, nil
}
