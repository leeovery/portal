package cmd

// Routing-classification guard for the typed resolver.Domain surface. It pins that
// globExpandableDomain classifies exactly the three glob-expandable domains
// (bare/session/alias) the prior string switch did and rejects the non-expandable
// ones (path/zoxide/glob/miss + the empty domain), now via compile-checked typed
// constants rather than string literals. No package-level state, no cobra Execute,
// no tmux — but package cmd, so per CLAUDE.md it MUST NOT use t.Parallel.

import (
	"testing"

	"github.com/leeovery/portal/internal/resolver"
)

func TestGlobExpandableDomain_TypedConstants(t *testing.T) {
	// Glob-expandable domains expand a glob value against a finite Portal-owned
	// namespace: bare positionals are session-domain by construction, -s expands
	// over session names, -a over alias keys.
	for _, d := range []resolver.Domain{resolver.DomainBare, resolver.DomainSession, resolver.DomainAlias} {
		if !globExpandableDomain(d) {
			t.Errorf("globExpandableDomain(%q) = false, want true", d)
		}
	}

	// Every other domain (including the empty zero value) never glob-expands: -p is
	// a literal path, -z a zoxide subsequence query, glob is already-expanded, miss
	// is a total miss.
	for _, d := range []resolver.Domain{resolver.DomainPath, resolver.DomainZoxide, resolver.DomainGlob, resolver.DomainMiss, resolver.Domain("")} {
		if globExpandableDomain(d) {
			t.Errorf("globExpandableDomain(%q) = true, want false", d)
		}
	}
}
