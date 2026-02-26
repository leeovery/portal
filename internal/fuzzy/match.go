// Package fuzzy provides subsequence-based fuzzy matching.
package fuzzy

// Match returns true if pattern is a subsequence of text.
// Each character in pattern must appear in text in order,
// but not necessarily consecutively.
func Match(text, pattern string) bool {
	pi := 0
	for i := 0; i < len(text) && pi < len(pattern); i++ {
		if text[i] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}
