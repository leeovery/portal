package fuzzy_test

import (
	"testing"

	"github.com/leeovery/portal/internal/fuzzy"
)

func TestMatch(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		pattern string
		want    bool
	}{
		{
			name:    "empty pattern matches any text",
			text:    "hello",
			pattern: "",
			want:    true,
		},
		{
			name:    "empty pattern matches empty text",
			text:    "",
			pattern: "",
			want:    true,
		},
		{
			name:    "non-empty pattern does not match empty text",
			text:    "",
			pattern: "a",
			want:    false,
		},
		{
			name:    "exact match",
			text:    "hello",
			pattern: "hello",
			want:    true,
		},
		{
			name:    "subsequence match",
			text:    "my-project",
			pattern: "mpr",
			want:    true,
		},
		{
			name:    "no match when characters are not in order",
			text:    "abc",
			pattern: "cb",
			want:    false,
		},
		{
			name:    "no match when pattern has characters not in text",
			text:    "hello",
			pattern: "xyz",
			want:    false,
		},
		{
			name:    "case sensitive mismatch",
			text:    "Hello",
			pattern: "hello",
			want:    false,
		},
		{
			name:    "case sensitive exact match",
			text:    "Hello",
			pattern: "Hello",
			want:    true,
		},
		{
			name:    "pattern longer than text",
			text:    "ab",
			pattern: "abc",
			want:    false,
		},
		{
			name:    "single character match",
			text:    "abc",
			pattern: "b",
			want:    true,
		},
		{
			name:    "single character no match",
			text:    "abc",
			pattern: "z",
			want:    false,
		},
		{
			name:    "repeated characters in pattern",
			text:    "aabbcc",
			pattern: "abc",
			want:    true,
		},
		{
			name:    "subsequence at end of text",
			text:    "xyzabc",
			pattern: "abc",
			want:    true,
		},
		{
			name:    "subsequence spread across text",
			text:    "a1b2c3",
			pattern: "abc",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzy.Match(tt.text, tt.pattern)
			if got != tt.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.text, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	items := []string{"Alpha", "Bravo", "Charlie", "Delta", "ECHO"}
	nameOf := func(s string) string { return s }

	tests := []struct {
		name   string
		items  []string
		filter string
		want   []string
	}{
		{
			name:   "empty filter returns all items",
			items:  items,
			filter: "",
			want:   items,
		},
		{
			name:   "non-matching filter returns empty",
			items:  items,
			filter: "zzz",
			want:   nil,
		},
		{
			name:   "partial match returns matching items",
			items:  items,
			filter: "al",
			want:   []string{"Alpha", "Charlie"},
		},
		{
			name:   "case insensitive matching",
			items:  items,
			filter: "ALPHA",
			want:   []string{"Alpha"},
		},
		{
			name:   "case insensitive filter with uppercase item",
			items:  items,
			filter: "echo",
			want:   []string{"ECHO"},
		},
		{
			name:   "nil items returns nil for non-empty filter",
			items:  nil,
			filter: "a",
			want:   nil,
		},
		{
			name:   "nil items returns nil for empty filter",
			items:  nil,
			filter: "",
			want:   nil,
		},
		{
			name:   "single item match",
			items:  []string{"Portal"},
			filter: "ptl",
			want:   []string{"Portal"},
		},
		{
			name:   "single item no match",
			items:  []string{"Portal"},
			filter: "xyz",
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzy.Filter(tt.items, tt.filter, nameOf)
			if len(got) != len(tt.want) {
				t.Fatalf("Filter() returned %d items, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Filter()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
