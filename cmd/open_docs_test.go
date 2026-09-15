package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// openSectionHeading is the README heading whose section documents the session
// verb. A rename must move the assertions with it rather than emptying them.
const openSectionHeading = "### `x` (open)"

// readmeOpenSection returns the body of the README's open section, fatal when the
// heading is gone or its section is empty, so the cause is reported once rather
// than as one failure per assertion below.
func readmeOpenSection(t *testing.T) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(sourceguardtest.ProjectRoot(t), "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	_, after, found := strings.Cut(string(body), openSectionHeading)
	if !found {
		t.Fatalf("README.md carries no %q heading", openSectionHeading)
	}
	section, _, _ := strings.Cut(after, "\n### ")
	if strings.TrimSpace(section) == "" {
		t.Fatalf("README.md's %q section is empty", openSectionHeading)
	}
	return section
}

// readmeOpenExamples returns the lines of the open section's first fenced block,
// so an example assertion is independent of the prose around it: the section's
// sentences carry `/port` and `/` too, and a whole-section match would be
// satisfied by those alone.
func readmeOpenExamples(t *testing.T, section string) []string {
	t.Helper()

	_, afterFence, found := strings.Cut(section, "```bash\n")
	if !found {
		t.Fatalf("README.md's %q section carries no bash example block", openSectionHeading)
	}
	block, _, found := strings.Cut(afterFence, "```")
	if !found {
		t.Fatalf("README.md's %q example block is unterminated", openSectionHeading)
	}
	return strings.Split(strings.TrimSpace(block), "\n")
}

// hasCommentedExample reports whether the block holds an `x <argument>` example
// carrying a comment — the two halves the section's examples are asserted on.
func hasCommentedExample(lines []string, argument string) bool {
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "x" && fields[1] == argument && strings.Contains(line, "#") {
			return true
		}
	}
	return false
}

// hasRowStarting reports whether some line of the section begins with prefix
// once trimmed — what anchors a table-row assertion to the row itself, since the
// prose above the table carries the same tokens.
func hasRowStarting(section, prefix string) bool {
	for line := range strings.SplitSeq(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true
		}
	}
	return false
}

func TestReadmeDocumentsSearchForm(t *testing.T) {
	section := readmeOpenSection(t)

	examples := readmeOpenExamples(t, section)
	for description, argument := range map[string]string{
		"the search-form example": "/port",
		"the term-less example":   "/",
	} {
		if !hasCommentedExample(examples, argument) {
			t.Errorf("README's open example block documents %s: want a commented %q line", description, "x "+argument)
		}
	}

	const resolutionRow = "| `/<term>`"
	if !hasRowStarting(section, resolutionRow) {
		t.Errorf("README's open section documents the resolution-table row: want a line beginning %q in it", resolutionRow)
	}

	tokens := map[string]string{
		"the single-segment directory escape": "-p /tmp",
		"the completion correction's owner":   "portal init",
		"the filter flag's own table row":     "-f, --filter",
	}
	for description, token := range tokens {
		if !strings.Contains(section, token) {
			t.Errorf("README's open section documents %s: want %q somewhere in it", description, token)
		}
	}
}
