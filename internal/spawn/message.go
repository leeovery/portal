package spawn

import "strings"

// QuoteJoin single-quotes each name and joins them with ", " — rendering 's2'
// for one name and 's2', 's4' for several. It is the shared renderer for the
// spawn one-line messages (the pre-flight gone-session error and the Phase-6
// picker's equivalents), so the CLI (cmd/spawn.go) and the picker (internal/tui)
// name sessions identically. An empty slice renders the empty string.
func QuoteJoin(names []string) string {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = "'" + name + "'"
	}
	return strings.Join(quoted, ", ")
}

// GoneVerb is the count-aware verb for the gone-session message: "is" for a
// single name (n == 1) and "are" for several (any other count, including zero).
// Paired with QuoteJoin so "'s2' is gone" / "'s2', 's4' are gone" agree in
// number.
func GoneVerb(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}
