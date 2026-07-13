package spawn

import "strings"

// friendlyAliases is Portal's shipped alias table: a short, friendly key the
// user can paste in terminals.json (for a terminal Portal already knows) mapped
// to that terminal's bundle-id family glob. Matching a friendly-alias key means
// MatchesFamily(id.BundleID, friendlyAliases[key]) — the alias is just a nicer
// spelling of the family, so config matching stays on bundle-id families
// (identical to native detection). Extend this table as more terminals ship.
var friendlyAliases = map[string]string{
	"ghostty": "com.mitchellh.ghostty*",
	"warp":    "dev.warp.Warp-*",
}

// Specificity tiers, highest-first. A user's exact override always beats their
// glob fallback: the exact raw bundle id (the copy-paste id Portal displays) is
// the most specific, a named form (friendly alias or .app name) is next, and a
// *-glob family is the broadest.
const (
	tierGlob     = 1
	tierNamed    = 2
	tierBundleID = 3
)

// matchScore ranks one matching config key by specificity. tier is the coarse
// form-class; literals refines glob matches so a longer/more-specific glob
// outscores a broader one (it carries no meaning for non-glob tiers, which are
// separated by tier alone).
type matchScore struct {
	tier     int
	literals int
}

// better reports whether s is strictly more specific than o: higher tier wins,
// and within the same tier more literals wins. Equal tier and literals is not
// "better" — matchConfig breaks that residual tie deterministically by key.
func (s matchScore) better(o matchScore) bool {
	if s.tier != o.tier {
		return s.tier > o.tier
	}
	return s.literals > o.literals
}

// matchConfig returns the single most-specific config key (and its entry) that
// matches the identity, or ok=false when no key matches. It is pure: it only
// ranks keys — no logging, no recipe validation, no adapter construction (those
// compose in a later task).
//
// Go map iteration order is randomised, so ties must not decide the winner: a
// strictly-better score replaces the incumbent, and a residual exact tie (same
// tier and literals) keeps the lexicographically-smaller key.
func matchConfig(cfg TerminalsConfig, id Identity) (key string, entry TerminalEntry, ok bool) {
	var bestScore matchScore
	for k, e := range cfg {
		score, matched := scoreKey(k, id)
		if !matched {
			continue
		}
		switch {
		case !ok, score.better(bestScore):
			key, entry, bestScore, ok = k, e, score, true
		case !bestScore.better(score) && k < key:
			// Exact tie on tier+literals — keep the smaller key.
			key, entry = k, e
		}
	}
	return key, entry, ok
}

// scoreKey classifies a config key against an identity by form and returns its
// specificity score, or matched=false when the key does not match. The order is
// load-bearing: a key containing "*" is always a glob first; the exact raw
// bundle id outranks the named forms (friendly alias / .app name), which share a
// tier and are separated from globs by tier alone.
func scoreKey(key string, id Identity) (matchScore, bool) {
	if strings.Contains(key, "*") {
		if MatchesFamily(id.BundleID, key) {
			return matchScore{tier: tierGlob, literals: countLiterals(key)}, true
		}
		return matchScore{}, false
	}

	if key == id.BundleID {
		return matchScore{tier: tierBundleID}, true
	}

	if family, isAlias := friendlyAliases[key]; isAlias && MatchesFamily(id.BundleID, family) {
		return matchScore{tier: tierNamed}, true
	}

	if id.Name != "" && key == id.Name {
		return matchScore{tier: tierNamed}, true
	}

	return matchScore{}, false
}

// countLiterals returns the number of non-"*" runes in a glob key — its literal
// specificity. A longer literal prefix means a more specific family, so a bare
// "*" (zero literals) is the lowest-ranked match of all.
func countLiterals(key string) int {
	n := 0
	for _, r := range key {
		if r != '*' {
			n++
		}
	}
	return n
}
