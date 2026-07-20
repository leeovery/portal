package resolver

import (
	"path/filepath"
	"strings"
)

// globMeta is the set of glob metacharacters that mark a bare open target as
// session-domain by construction. A leading '[' opens a character class; a lone
// ']' is not a metacharacter starter, so it is deliberately absent.
const globMeta = "*?["

// HasGlobMeta reports whether s contains any glob metacharacter (*, ?, or [).
// A target that satisfies this predicate is resolved in the session domain by
// construction (spec § Glob targets) — expanded against live session names and
// never run through the path/alias/zoxide chain. It is exported so the open
// command body can gate its resolution log line on it.
func HasGlobMeta(s string) bool {
	return strings.ContainsAny(s, globMeta)
}

// MatchGlob returns the subset of names matching the glob pattern, in the
// order given. Matching is glob (not regex) via filepath.Match; a malformed
// pattern (filepath.ErrBadPattern, e.g. an unclosed '[') is treated as "no
// match for that name", so a bad glob yields zero matches (a hard fail) rather
// than an error or a panic. It is domain-agnostic — the names may be session
// names, alias keys, or any other string namespace.
func MatchGlob(pattern string, names []string) []string {
	matches := []string{}
	for _, name := range names {
		if ok, err := filepath.Match(pattern, name); err == nil && ok {
			matches = append(matches, name)
		}
	}
	return matches
}
