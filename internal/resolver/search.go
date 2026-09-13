package resolver

import "strings"

// MatchesSearchTerm reports whether term appears as a contiguous run, case-folded,
// in either the session name or its recorded directory in home-abbreviated form.
// The two fields are tested separately and never joined, so a run spanning the end
// of one and the start of the other is not a match. The term is literal text: a
// glob metacharacter in it is a character to find rather than a wildcard. An empty
// term matches nothing, so a caller missing its own guard narrows to nothing rather
// than to everything. A session with no recorded directory is judged on its name.
func MatchesSearchTerm(term, name, recordedDir string) bool {
	if term == "" {
		return false
	}

	folded := strings.ToLower(term)

	if strings.Contains(strings.ToLower(name), folded) {
		return true
	}

	if recordedDir == "" {
		return false
	}

	return strings.Contains(strings.ToLower(AbbreviateHome(recordedDir)), folded)
}
