package resolver

import "strings"

// SearchSubject is a session as a search sees it: the label its row shows,
// empty when it shows its name; its name; and its recorded directory, empty
// when none is recorded.
type SearchSubject struct {
	Label string
	Name  string
	Dir   string
}

// SearchFields is the field set a session is searchable by, in display order:
// its label when it has one other than its name, its name, and its recorded
// directory abbreviated against home when it has one, so a term hitting the
// home prefix cannot match a session on characters no row shows.
func SearchFields(home string, s SearchSubject) []string {
	fields := make([]string, 0, 3)
	if s.Label != "" && s.Label != s.Name {
		fields = append(fields, s.Label)
	}
	fields = append(fields, s.Name)
	if s.Dir != "" {
		fields = append(fields, AbbreviateHome(home, s.Dir))
	}
	return fields
}

// MatchesSearchTerm reports whether term appears as a contiguous run, case-folded,
// in one of the session's search fields under home. The fields are tested
// separately and never joined, so a run spanning the end of one and the start of
// the next is not a match. The term is literal text: a glob metacharacter in it
// is a character to find rather than a wildcard. An empty term matches nothing,
// so a caller missing its own guard narrows to nothing rather than to everything.
func MatchesSearchTerm(home, term string, s SearchSubject) bool {
	if term == "" {
		return false
	}

	folded := strings.ToLower(term)

	for _, field := range SearchFields(home, s) {
		if strings.Contains(strings.ToLower(field), folded) {
			return true
		}
	}

	return false
}
