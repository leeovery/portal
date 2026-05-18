package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// TestTruncateToCells covers the display-cell-aware truncation primitive used
// by the preview frame's chrome cascade (tier 1 truncate-window-name with …
// suffix, tier 2 8-cell minimum) per
// specification.md § Display-cell-aware truncation and § Width cascade > Tier 1.
//
// Each row asserts three universal invariants:
//   - utf8.ValidString(got) is true (no mid-rune cuts).
//   - runewidth.StringWidth(got) <= budget.
//   - strings.HasSuffix(got, "…") matches the row's expected truncation flag.
//
// Glyph classes covered: ASCII, CJK (2 cells/rune), emoji ZWJ sequences
// (2 cells, multi-codepoint), and combining marks (0 cells).
func TestTruncateToCells(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		budget    int
		want      string
		truncated bool
	}{
		{name: "ascii fits whole", input: "hello", budget: 10, want: "hello", truncated: false},
		{name: "ascii truncates", input: "hello world", budget: 8, want: "hello w…", truncated: true},
		{name: "cjk fits", input: "日本語", budget: 6, want: "日本語", truncated: false},
		{name: "cjk truncates", input: "日本語テスト", budget: 7, want: "日本語…", truncated: true},
		{name: "emoji zwj tight budget", input: "👨‍👩‍👧", budget: 3, want: "👨‍👩‍👧", truncated: false},
		{name: "combining marks fit", input: "áb́ć", budget: 3, want: "áb́ć", truncated: false},
		{name: "budget zero", input: "anything", budget: 0, want: "", truncated: false},
		{name: "budget one non-empty does not fit whole", input: "abc", budget: 1, want: "…", truncated: true},
		{name: "empty string positive budget", input: "", budget: 10, want: "", truncated: false},
		{name: "empty string zero budget", input: "", budget: 0, want: "", truncated: false},
		{name: "budget eight ascii fits", input: "abcdefgh", budget: 8, want: "abcdefgh", truncated: false},
		{name: "boundary budget equals width", input: "hello", budget: 5, want: "hello", truncated: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := truncateToCells(tc.input, tc.budget)
			if got != tc.want {
				t.Errorf("truncateToCells(%q, %d) = %q, want %q", tc.input, tc.budget, got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("truncateToCells(%q, %d) = %q produced invalid UTF-8", tc.input, tc.budget, got)
			}
			if w := runewidth.StringWidth(got); w > tc.budget {
				t.Errorf("truncateToCells(%q, %d) = %q has width %d > budget %d", tc.input, tc.budget, got, w, tc.budget)
			}
			if gotTrunc := strings.HasSuffix(got, "…"); gotTrunc != tc.truncated {
				t.Errorf("truncateToCells(%q, %d) = %q truncated=%v, want truncated=%v", tc.input, tc.budget, got, gotTrunc, tc.truncated)
			}
		})
	}
}

// TestTruncateToCells_ZWJSequenceTruncationInvariants exercises the truncation
// arm with a ZWJ-sequence input long enough to force the slow path. The
// current algorithm iterates codepoint-by-codepoint and is not
// grapheme-cluster aware, so the exact byte content of the truncated output
// is an implementation detail (a trailing ZWJ may dangle). This test asserts
// only the spec-mandated universal invariants — valid UTF-8, width ≤ budget,
// ellipsis suffix when truncation occurred — to document that those hold
// even when the input crosses a ZWJ boundary at the cut point.
func TestTruncateToCells_ZWJSequenceTruncationInvariants(t *testing.T) {
	const input = "👨‍👩‍👧hello"
	const budget = 4
	got := truncateToCells(input, budget)
	if !utf8.ValidString(got) {
		t.Errorf("truncateToCells(%q, %d) = %q produced invalid UTF-8", input, budget, got)
	}
	if w := runewidth.StringWidth(got); w > budget {
		t.Errorf("truncateToCells(%q, %d) = %q has width %d > budget %d", input, budget, got, w, budget)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncateToCells(%q, %d) = %q missing ellipsis suffix on a truncating input", input, budget, got)
	}
}
