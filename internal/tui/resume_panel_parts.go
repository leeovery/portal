package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// Both screens' cards are built on this one width, so content cannot move
// their geometry.
const resumeCardContentWidth = destructiveBodyWidth

const resumeCommandMaxLines = 3

const resumeEllipsis = "…"

// A registered command is arbitrary user-authored text: an ESC left in it would
// repaint the card around it, and a newline or tab would move its geometry.
func sanitiseCommandText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

func resumeCommandLines(command string, width int) []string {
	lines := wrappedLines(sanitiseCommandText(command), width)
	if len(lines) <= resumeCommandMaxLines {
		return lines
	}
	kept := lines[: resumeCommandMaxLines-1 : resumeCommandMaxLines-1]
	beyond := strings.Join(lines[resumeCommandMaxLines-1:], " ")
	return append(kept, strings.Trim(ansi.Truncate(beyond, width, resumeEllipsis), " "))
}

// ansi.Wrap rather than ansi.Wordwrap: a word longer than the limit must be
// broken at it rather than left to overflow the card. Every line comes back no
// wider than the width asked for, which ansi.Wrap on its own does not promise —
// it over-packs a run of hyphens past the limit at ordinary widths. A row a
// cell too wide cannot be padded back down, and widens the card built around
// it, so the overshoot re-flows onto the line after it rather than being cut
// off the end. The gap is the whitespace the wrap consumed breaking a line and
// left in neither of them: without it the overshoot is carried onto the next
// line glued to text the command separates from it.
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

func resumeCommandRows(command string, width int, tok theme.Token, bold bool, th theme.Theme, colourless bool) []string {
	style := headerStyle(tok, th, colourless)
	if bold {
		style = style.Bold(true)
	}
	lines := resumeCommandLines(command, width)
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, resumePaddedRow(style.Render(line), width, th, colourless))
	}
	return rows
}

// Truncated rather than wrapped: a report that wrapped would change the card's
// height between two screens that are otherwise identical.
func resumeReportRow(report string, width int, th theme.Theme, colourless bool) (string, bool) {
	if report == "" {
		return "", false
	}
	text := ansi.Truncate(sanitiseCommandText(report), width, resumeEllipsis)
	seg := headerStyle(th.AccentAttention, th, colourless).Render(text)
	return resumePaddedRow(seg, width, th, colourless), true
}

func resumePaddedRow(seg string, width int, th theme.Theme, colourless bool) string {
	return headerPadRight(seg, lipgloss.Width(seg), width, th, colourless)
}
