package project

import "strings"

// NormaliseTag converts a raw tag value into its canonical form: leading and
// trailing whitespace trimmed, then lower-cased. Internal whitespace is
// preserved. It returns ok==false (with an empty string) for input that is
// empty or whitespace-only, which callers treat as a rejected/no-op tag.
//
// This is the sole canonical-form function for tags. Every later tag
// comparison — per-project dedup, the cross-project union that defines which
// tags exist, and By-Tag grouping — MUST call it rather than re-implementing
// trim/lower-case, so the grouping key stays consistent everywhere.
func NormaliseTag(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	return strings.ToLower(trimmed), true
}
