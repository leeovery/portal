// Package fuzzy provides subsequence-based fuzzy matching.
package fuzzy

import "strings"

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

// Filter returns items whose names fuzzy-match the given filter string.
// Matching is case-insensitive. The nameOf function extracts the name from each item.
// If filter is empty, all items are returned.
func Filter[T any](items []T, filter string, nameOf func(T) string) []T {
	if filter == "" {
		return items
	}
	lowerFilter := strings.ToLower(filter)
	var result []T
	for _, item := range items {
		if Match(strings.ToLower(nameOf(item)), lowerFilter) {
			result = append(result, item)
		}
	}
	return result
}
