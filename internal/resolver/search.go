package resolver

import "strings"

// SearchFields is the field set a session is searchable by, in display order:
// its name, and its recorded directory abbreviated against home when it has
// one, so a term hitting the home prefix cannot match a session on characters
// no row shows. A session with no recorded directory is searchable by its name
// alone.
func SearchFields(home, name, recordedDir string) []string {
	if recordedDir == "" {
		return []string{name}
	}
	return []string{name, AbbreviateHome(home, recordedDir)}
}

// MatchesSearchTerm reports whether term appears as a contiguous run, case-folded,
// in one of the session's search fields under home. The fields are tested
// separately and never joined, so a run spanning the end of one and the start of
// the next is not a match. The term is literal text: a glob metacharacter in it
// is a character to find rather than a wildcard. An empty term matches nothing,
// so a caller missing its own guard narrows to nothing rather than to everything.
func MatchesSearchTerm(home, term, name, recordedDir string) bool {
	if term == "" {
		return false
	}

	folded := strings.ToLower(term)

	for _, field := range SearchFields(home, name, recordedDir) {
		if strings.Contains(strings.ToLower(field), folded) {
			return true
		}
	}

	return false
}
