package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// For text whose hyphens belong to its words, such as a command's flags, where
// a row ending "--" cut from the word after it misreads the text.
func wrapCappedAtSpaces(text string, width, maxRows int, ellipsis string) []string {
	return capRows(spaceWrappedRows(text, width), width, maxRows, ellipsis)
}

// The rows past the cap are re-joined onto the last kept one and truncated
// there, so the ellipsis marks the cut rather than the rows leaving without one.
func capRows(rows []wrappedRow, width, maxRows int, ellipsis string) []string {
	if len(rows) <= maxRows {
		return rowTexts(rows)
	}
	kept := rowTexts(rows[:maxRows-1])
	var beyond strings.Builder
	beyond.WriteString(rows[maxRows-1].text)
	for i := maxRows; i < len(rows); i++ {
		beyond.WriteString(joiningGap(rows[i-1], rows[i], beyond.String()))
		beyond.WriteString(rows[i].text)
	}
	return append(kept, cutRow(strings.TrimLeftFunc(beyond.String(), breaksWrap), width, ellipsis))
}

// A row opening a line of the text joins the one before by exactly one space,
// the line break and the line's leading whitespace collapsed into it.
func joiningGap(before, row wrappedRow, joined string) string {
	switch {
	case row.line == before.line:
		return row.gap
	case strings.HasSuffix(joined, " "):
		return ""
	default:
		return " "
	}
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
	// The source's whitespace between the row before and this one on its line:
	// none where the break fell inside a word or straight after a hyphen.
	gap string
	// The index of the text's line the row is from.
	line int
}

func rowTexts(rows []wrappedRow) []string {
	texts := make([]string, len(rows))
	for i, row := range rows {
		texts[i] = row.text
	}
	return texts
}

// Each line of the text is wrapped on its own, so no row, carry or gap crosses
// a line break, and every line gives at least one row: a line holding nothing
// visible gives one empty row. A tab or carriage return reads as one space,
// since ansi.StringWidth counts either as no cells and a renderer draws a tab
// as several, so a row holding one could not be measured.
func wrappedRows(text string, width int) []wrappedRow {
	var rows []wrappedRow
	for i, line := range strings.Split(text, "\n") {
		rows = append(rows, lineRows(withoutMarksOnSpace(cellSpaces.Replace(line)), width, i)...)
	}
	return rows
}

// A combining mark, joiner or other zero-width character after whitespace sits
// on that whitespace and shows nothing, so it is dropped: carried with the
// whitespace a row trims, it would land on a letter it never followed.
func withoutMarksOnSpace(line string) string {
	var kept strings.Builder
	var state byte
	afterSpace := false
	for line != "" {
		seq, width, n, next := ansi.DecodeSequence(line, state, nil)
		line, state = line[n:], next
		base, size := utf8.DecodeRuneInString(seq)
		switch {
		case isEscape(seq):
			kept.WriteString(seq)
		case unicode.IsSpace(base):
			kept.WriteString(seq[:size])
			afterSpace = true
		case width == 0 && afterSpace:
		default:
			kept.WriteString(seq)
			afterSpace = false
		}
	}
	return kept.String()
}

var cellSpaces = strings.NewReplacer("\t", " ", "\r", " ")

// ansi.Wrap rather than ansi.Wordwrap: a word longer than the limit must be
// broken at it rather than left to overflow the frame. Every line comes back no
// wider than the width asked for, which ansi.Wrap on its own does not promise —
// it over-packs a run of hyphens past the limit at ordinary widths. A row a
// cell too wide cannot be padded back down, and the frame built around it
// cannot hold it, so the overshoot re-flows onto the row after it rather than
// being cut off the end.
func lineRows(line string, width, index int) []wrappedRow {
	wrapped := strings.Split(ansi.Wrap(line, width, ""), "\n")
	walk := rowWalk{width: width, line: index}
	rest := line
	for i, part := range wrapped {
		rest = strings.TrimPrefix(rest, part)
		gap := droppedGap(rest, wrapped[i+1:])
		rest = rest[len(gap):]
		walk.take(walk.carry + part + gap)
	}
	for walk.carry != "" {
		walk.take(walk.carry)
	}
	return walk.done()
}

// ansi.Wrap breaks after a hyphen whatever breakpoints it is given, so each
// hyphen is wrapped as a one-cell stand-in the text does not hold and handed
// back on every row; a word wider than the row still breaks at the width. The
// text must carry no escape sequence, as a hyphen inside one would be swapped
// too.
func spaceWrappedRows(text string, width int) []wrappedRow {
	standIn := string(absentPrivateUseRune(text))
	rows := wrappedRows(strings.ReplaceAll(text, "-", standIn), width)
	for i := range rows {
		rows[i].text = strings.ReplaceAll(rows[i].text, standIn, "-")
	}
	return rows
}

func absentPrivateUseRune(text string) rune {
	r := rune(0xe000)
	for strings.ContainsRune(text, r) {
		r++
	}
	return r
}

// The whitespace trimmed off a row's edges is kept rather than discarded: the
// trailing run of one row and the leading run of the next are together the
// source's whitespace between them.
type rowWalk struct {
	width, line  int
	rows         []wrappedRow
	carry, trail string
	// Escape sequences met before the line's first row, which open it.
	pending string
	// The last grapheme met was dropped as too wide for any row, so marks
	// opening what comes next went with it.
	orphaned bool
}

// A row showing nothing is no row, as the empty one ansi.Wrap leaves after a
// line it over-packed: its whitespace joins the gap before the next row, and
// its escape sequences, which take no width, the row before it. ansi.Wrap
// breaks a word between a letter and the combining mark after it, so the marks
// a row opens on go back to the row before, where their grapheme starts; a
// zero-width mark never widens it.
func (w *rowWalk) take(line string) {
	body := strings.TrimLeftFunc(line, breaksWrap)
	head, carry, dropped := rowAndCarry(body, w.width)
	text := strings.TrimRightFunc(head, breaksWrap)
	gap := w.trail + line[:len(line)-len(body)]
	w.trail, w.carry = head[len(text):], carry
	marks := leadingMarks(text)
	switch {
	case w.orphaned:
		w.attach(escapesOf(text[:marks]))
		text = text[marks:]
	case len(w.rows) > 0:
		w.attach(text[:marks])
		text = text[marks:]
	}
	if dropped || !showsNothing(text) {
		w.orphaned = dropped && showsNothing(carry)
	}
	if showsNothing(text) {
		w.trail = gap + w.trail
		w.attach(escapesOf(text))
		return
	}
	w.rows = append(w.rows, wrappedRow{text: w.pending + text, gap: gap, line: w.line})
	w.pending = ""
}

func (w *rowWalk) attach(escapes string) {
	if len(w.rows) == 0 {
		w.pending += escapes
		return
	}
	w.rows[len(w.rows)-1].text += escapes
}

func (w *rowWalk) done() []wrappedRow {
	if len(w.rows) == 0 {
		return []wrappedRow{{line: w.line}}
	}
	return w.rows
}

func showsNothing(text string) bool {
	return ansi.StringWidth(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text)) == 0
}

// leadingMarks is the length of text's opening run of escape sequences and
// zero-width characters, up to the last zero-width character in it: what
// continues the grapheme the row before ends on.
func leadingMarks(text string) int {
	marks := 0
	var state byte
	for at := 0; at < len(text); {
		seq, width, n, next := ansi.DecodeSequence(text[at:], state, nil)
		if width > 0 || unicode.IsSpace([]rune(seq)[0]) {
			break
		}
		at, state = at+n, next
		if !isEscape(seq) {
			marks = at
		}
	}
	return marks
}

func escapesOf(text string) string {
	var escapes strings.Builder
	var state byte
	for text != "" {
		seq, _, n, next := ansi.DecodeSequence(text, state, nil)
		if isEscape(seq) {
			escapes.WriteString(seq)
		}
		text, state = text[n:], next
	}
	return escapes.String()
}

func isEscape(seq string) bool {
	return seq[0] == ansi.ESC
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
func rowAndCarry(line string, width int) (head, carry string, dropped bool) {
	if ansi.StringWidth(strings.TrimRightFunc(line, breaksWrap)) <= width {
		return line, "", false
	}
	return splitAtWidth(line, width)
}

// A grapheme wider than the width fits in no row at all: ansi.Truncate drops it
// and ansi.TruncateLeft hands it back whole, so it is dropped here rather than
// left to re-present the same line forever. The escape sequences before it are
// kept, as a sequence cut in two would show as text. A grapheme the cut falls
// across is dropped by both, and the marks after it go with it. dropped says
// whether a grapheme was.
func splitAtWidth(line string, width int) (head, rest string, dropped bool) {
	head, rest = ansi.Truncate(line, width, ""), ansi.TruncateLeft(line, width, "")
	switch lineWidth := ansi.StringWidth(line); {
	case ansi.StringWidth(rest) == lineWidth:
		return head, withoutFirstShown(line), true
	case ansi.StringWidth(head)+ansi.StringWidth(rest) < lineWidth:
		return head, withoutLeadingMarks(rest, 0), true
	}
	return head, rest, false
}

// withoutFirstShown drops the first grapheme that takes width, with the marks
// after it that an escape sequence parted from it and that still show on it.
func withoutFirstShown(line string) string {
	var state byte
	for at := 0; at < len(line); {
		_, width, n, next := ansi.DecodeSequence(line[at:], state, nil)
		if width > 0 {
			return line[:at] + withoutLeadingMarks(line[at+n:], next)
		}
		at, state = at+n, next
	}
	return ""
}

func withoutLeadingMarks(line string, state byte) string {
	var kept strings.Builder
	for line != "" {
		seq, width, n, next := ansi.DecodeSequence(line, state, nil)
		base, _ := utf8.DecodeRuneInString(seq)
		if width > 0 || unicode.IsSpace(base) {
			break
		}
		if isEscape(seq) {
			kept.WriteString(seq)
		}
		line, state = line[n:], next
	}
	return kept.String() + line
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
