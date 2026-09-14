package cmd

import (
	"strings"
	"testing"
)

// Keywords rather than a golden string, so accurate copy edits do not churn
// this.
func TestOpenHelpMetadata_DescribesSearchForm(t *testing.T) {
	if openCmd.Long == "" {
		t.Fatal("openCmd.Long is empty; expected a description of the search form")
	}
	long := strings.ToLower(openCmd.Long)

	t.Run("it describes the search form's recognition rule", func(t *testing.T) {
		for _, want := range []string{"open /term", "beginning with /", "no further /"} {
			if !strings.Contains(long, want) {
				t.Errorf("openCmd.Long omits %q; the recognition rule is not stated:\n%s", want, openCmd.Long)
			}
		}
	})

	t.Run("it describes the term-less form", func(t *testing.T) {
		if !strings.Contains(long, "open /\n") && !strings.Contains(long, "open /  ") {
			t.Errorf("openCmd.Long does not name the term-less form `open /`:\n%s", openCmd.Long)
		}
		if !strings.Contains(long, "ready to type") {
			t.Errorf("openCmd.Long does not say the term-less form opens a filter ready to type:\n%s", openCmd.Long)
		}
	})

	t.Run("it gives the outcomes by match count", func(t *testing.T) {
		for _, want := range []string{"one match", "attaches", "picker pre-filtered", "never an error"} {
			if !strings.Contains(long, want) {
				t.Errorf("openCmd.Long omits %q; the outcomes by match count are incomplete:\n%s", want, openCmd.Long)
			}
		}
	})

	t.Run("it names what the term is matched against", func(t *testing.T) {
		for _, want := range []string{"case-folded", "contiguous run", "recorded directory"} {
			if !strings.Contains(long, want) {
				t.Errorf("openCmd.Long omits %q; the match domain is not stated:\n%s", want, openCmd.Long)
			}
		}
	})

	t.Run("it states that the search form composes with nothing", func(t *testing.T) {
		if !strings.Contains(long, "composes with nothing") {
			t.Errorf("openCmd.Long does not state the composition rule:\n%s", openCmd.Long)
		}
		if !strings.Contains(long, "usage error") {
			t.Errorf("openCmd.Long does not name the refusal as a usage error:\n%s", openCmd.Long)
		}
	})

	t.Run("it names the single-segment directory cost and the -p escape", func(t *testing.T) {
		if !strings.Contains(long, "single-segment absolute directory") {
			t.Errorf("openCmd.Long does not name the shadowed single-segment directory:\n%s", openCmd.Long)
		}
		if !strings.Contains(long, "-p /tmp") {
			t.Errorf("openCmd.Long does not give the -p escape for it:\n%s", openCmd.Long)
		}
	})

	t.Run("it distinguishes -f from the search form by outcome", func(t *testing.T) {
		block := searchFormHelpBlockNaming(openCmd.Long, "-f", "/term")
		if block == "" {
			t.Fatalf("openCmd.Long has no single block naming both -f and /term:\n%s", openCmd.Long)
		}
		for _, want := range []string{"always opens the picker", "keybinding", "interactive"} {
			if !strings.Contains(strings.ToLower(block), want) {
				t.Errorf("the -f / /term block omits %q; the pair is not separated by outcome and use:\n%s", want, block)
			}
		}
	})
}

// searchFormHelpBlockNaming returns the blank-line-separated paragraph of text
// naming every one of want, or the empty string when no single paragraph does.
func searchFormHelpBlockNaming(text string, want ...string) string {
	for block := range strings.SplitSeq(text, "\n\n") {
		if namesAll(block, want) {
			return block
		}
	}
	return ""
}

func namesAll(block string, want []string) bool {
	for _, w := range want {
		if !strings.Contains(block, w) {
			return false
		}
	}
	return true
}

func TestOpenFilterFlagUsage_StaysAOneLiner(t *testing.T) {
	filter := openCmd.Flags().Lookup("filter")
	if filter == nil {
		t.Fatal("open must expose --filter")
	}
	if strings.Contains(filter.Usage, "\n") {
		t.Errorf("-f/--filter usage must stay a single line: %q", filter.Usage)
	}
	usage := strings.ToLower(filter.Usage)
	for _, unwanted := range []string{"/term", "search", "match"} {
		if strings.Contains(usage, unwanted) {
			t.Errorf("-f/--filter usage must not carry the search form or a match count (%q): %q", unwanted, filter.Usage)
		}
	}
}
