package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The rows past the cap are re-joined onto the last kept one and truncated
// there, so the ellipsis marks the cut rather than the rows leaving without one.
func wrapCapped(text string, width, maxRows int, ellipsis string) []string {
	lines := wrappedLines(text, width)
	if len(lines) <= maxRows {
		return lines
	}
	kept := lines[: maxRows-1 : maxRows-1]
	beyond := strings.Join(lines[maxRows-1:], " ")
	return append(kept, strings.Trim(ansi.Truncate(beyond, width, ellipsis), " "))
}

// ansi.Wrap rather than ansi.Wordwrap: a word longer than the limit must be
// broken at it rather than left to overflow the frame. Every line comes back no
// wider than the width asked for, which ansi.Wrap on its own does not promise —
// it over-packs a run of hyphens past the limit at ordinary widths. A row a
// cell too wide cannot be padded back down, and the frame built around it
// cannot hold it, so the overshoot re-flows onto the line after it rather than
// being cut off the end. The gap is the whitespace the wrap consumed breaking a
// line and left in neither of them: without it the overshoot is carried onto
// the next line glued to text the source separates from it.
func wrappedLines(text string, width int) []string {
	wrapped := strings.Split(ansi.Wrap(text, width, ""), "\n")
	lines := make([]string, 0, len(wrapped))
	rest := text
	row, carry := "", ""
	for i, line := range wrapped {
		rest = strings.TrimPrefix(rest, line)
		gap := droppedGap(rest, wrapped[i+1:])
		rest = rest[len(gap):]
		row, carry = rowAndCarry(carry+line+gap, width)
		lines = append(lines, row)
	}
	for carry != "" {
		row, carry = rowAndCarry(carry, width)
		lines = append(lines, row)
	}
	return lines
}

// The spaces the wrap dropped breaking a line: in neither of the two, and so on
// no row unless the carry is given them back. Any the following line kept for
// itself are still there, and are not handed over a second time.
func droppedGap(rest string, remaining []string) string {
	run := rest[:len(rest)-len(strings.TrimLeft(rest, " "))]
	if len(remaining) == 0 {
		return run
	}
	kept := len(remaining[0]) - len(strings.TrimLeft(remaining[0], " "))
	if kept >= len(run) {
		return ""
	}
	return run[kept:]
}

// The row is the line's leading width cells with its edge spaces trimmed; what
// ran past them is carried. The line is split before its trailing spaces are
// trimmed, so a space it ends on stays with the carry and still separates it
// from the line it is carried onto.
func rowAndCarry(line string, width int) (row, carry string) {
	line = strings.TrimLeft(line, " ")
	if fits := strings.TrimRight(line, " "); ansi.StringWidth(fits) <= width {
		return fits, ""
	}
	head, rest := splitAtWidth(line, width)
	return strings.TrimRight(head, " "), rest
}

// A grapheme wider than the width fits in no row at all: ansi.Truncate drops it
// and ansi.TruncateLeft hands it back whole, so it is dropped here rather than
// left to re-present the same line forever.
func splitAtWidth(line string, width int) (head, rest string) {
	head, rest = ansi.Truncate(line, width, ""), ansi.TruncateLeft(line, width, "")
	if ansi.StringWidth(rest) < ansi.StringWidth(line) {
		return head, rest
	}
	cluster, _ := ansi.FirstGraphemeCluster(line, ansi.GraphemeWidth)
	return head, strings.TrimPrefix(line, cluster)
}

func clampToWidth(line string, width int) string {
	if ansi.StringWidth(line) <= width {
		return line
	}
	return ansi.Truncate(line, width, "")
}
