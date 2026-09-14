package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/resolver"
)

// dirTruncationPrefix stands in for the head of a directory the column is too
// narrow to show whole. The cut it marks always lands on a separator, so what
// follows it is whole path segments.
const dirTruncationPrefix = "…"

// fitSessionDir renders a session's recorded directory in width cells: the
// home-abbreviated value whole where it fits, otherwise the longest
// separator-anchored tail behind dirTruncationPrefix. It returns the empty
// string when not even the last whole segment fits beside that prefix — a
// fragment of a segment reads as damage rather than as a path — and for an
// empty directory or a non-positive width. Not environment-independent: the
// abbreviation resolves the home directory, so one value renders differently
// under a different $HOME.
func fitSessionDir(dir string, width int) string {
	if dir == "" || width <= 0 {
		return ""
	}

	value := resolver.AbbreviateHome(dir)
	if lipgloss.Width(value) <= width {
		return value
	}

	// Left to right, so the first tail that fits is the longest one that does.
	for i, r := range value {
		if r != '/' {
			continue
		}
		tail := value[i:]
		if strings.Trim(tail, "/") == "" {
			continue
		}
		if candidate := dirTruncationPrefix + tail; lipgloss.Width(candidate) <= width {
			return candidate
		}
	}

	return ""
}
