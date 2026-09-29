package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// The rows past the cap are re-joined onto the last kept one and truncated
// there, so the ellipsis marks the cut rather than the rows leaving without one.
func wrapCapped(text string, width, maxRows int, ellipsis string) []string {
	rows := wrappedRows(text, width)
	if len(rows) <= maxRows {
		return rowTexts(rows)
	}
	kept := rowTexts(rows[:maxRows-1])
	var beyond strings.Builder
	beyond.WriteString(rows[maxRows-1].text)
	for _, row := range rows[maxRows:] {
		beyond.WriteString(row.gap)
		beyond.WriteString(row.text)
	}
	return append(kept, cutRow(strings.TrimLeftFunc(beyond.String(), breaksWrap), width, ellipsis))
}

// Text dropped as too wide for any row can leave what is past the cap fitting
// the width, and the cut is marked all the same.
func cutRow(beyond string, width int, ellipsis string) string {
	if ansi.StringWidth(beyond) > width {
		return ansi.Truncate(beyond, width, ellipsis)
	}
	return ansi.Truncate(beyond, width-ansi.StringWidth(ellipsis), "") + ellipsis
}

func wrappedLines(text string, width int) []string {
	return rowTexts(wrappedRows(text, width))
}

type wrappedRow struct {
	text string
	// The source's whitespace between the row before and this one: none where
	// the break fell inside a word or straight after a hyphen.
	gap string
}

func rowTexts(rows []wrappedRow) []string {
	texts := make([]string, len(rows))
	for i, row := range rows {
		texts[i] = row.text
	}
	return texts
}

// ansi.Wrap rather than ansi.Wordwrap: a word longer than the limit must be
// broken at it rather than left to overflow the frame. Every line comes back no
// wider than the width asked for, which ansi.Wrap on its own does not promise —
// it over-packs a run of hyphens past the limit at ordinary widths. A row a
// cell too wide cannot be padded back down, and the frame built around it
// cannot hold it, so the overshoot re-flows onto the line after it rather than
// being cut off the end.
func wrappedRows(text string, width int) []wrappedRow {
	wrapped := strings.Split(ansi.Wrap(text, width, ""), "\n")
	walk := rowWalk{width: width, rows: make([]wrappedRow, 0, len(wrapped))}
	rest := text
	for i, line := range wrapped {
		rest = strings.TrimPrefix(rest, line)
		gap := droppedGap(rest, wrapped[i+1:])
		rest = rest[len(gap):]
		walk.take(walk.carry + line + gap)
	}
	for walk.carry != "" {
		walk.take(walk.carry)
	}
	return walk.rows
}

// The whitespace trimmed off a row's edges is kept rather than discarded: the
// trailing run of one row and the leading run of the next are together the
// source's whitespace between them.
type rowWalk struct {
	width        int
	rows         []wrappedRow
	carry, trail string
}

func (w *rowWalk) take(line string) {
	body := strings.TrimLeftFunc(line, breaksWrap)
	head, carry := rowAndCarry(body, w.width)
	text := strings.TrimRightFunc(head, breaksWrap)
	w.rows = append(w.rows, wrappedRow{text: text, gap: w.trail + line[:len(line)-len(body)]})
	w.trail, w.carry = head[len(text):], carry
}

// The spaces the wrap dropped breaking a line: in neither of the two, and so on
// no row unless the carry is given them back. Any the following line kept for
// itself are still there, and are not handed over a second time.
func droppedGap(rest string, remaining []string) string {
	run := rest[:len(rest)-len(strings.TrimLeftFunc(rest, breaksWrap))]
	if len(remaining) == 0 {
		return run
	}
	kept := len(remaining[0]) - len(strings.TrimLeftFunc(remaining[0], breaksWrap))
	if kept >= len(run) {
		return ""
	}
	return run[kept:]
}

// The head is the line's leading width cells, trailing spaces included; what
// ran past them is carried, so a space it ends on stays with the carry and
// still separates it from the line it is carried onto.
func rowAndCarry(line string, width int) (head, carry string) {
	if ansi.StringWidth(strings.TrimRightFunc(line, breaksWrap)) <= width {
		return line, ""
	}
	return splitAtWidth(line, width)
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

// The whitespace ansi.Wrap breaks a line at and consumes: every space but the
// no-break one, which it keeps inside a word.
func breaksWrap(r rune) bool {
	return unicode.IsSpace(r) && r != '\u00a0'
}
