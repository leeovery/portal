package tui

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// wrapAlphabet provokes the over-pack: hyphens and spaces beside words, line
// breaks with and without a carriage return, a tab, a rune two cells wide, an
// SGR pair, combining marks (a decomposed é and ä), a zero-width joiner and a
// zero-width space.
var wrapAlphabet = []string{"a", "a", "a", "-", " ", "\n", "\r", "\t", "世", "\x1b[1m", "\x1b[0m", "\u0301", "\u0308", "e\u0301", "\u200d", "\u200b"}

const (
	wrapPropertyCases  = 100_000
	wrapPropertyMaxLen = 40
	wrapPropertyWidths = 12
)

// wrapLeakCases are inputs that once let a row, a carry or a gap cross a line
// break, lost a blank line, or left a row showing nothing.
var wrapLeakCases = []struct {
	text  string
	width int
}{
	{"-aaaaa\nb", 5},
	{"-aaaaa\n\nb", 5},
	{" a-a--b\nnext", 4},
	{"\x1b[1m aaaaaa\nnext", 6},
	{"aaaa -\nb", 4},
	{"a ---------- b\nc", 6},
	{" aaaaaa \nb", 6},
	{" aaaaaa  \nb", 6},
	{" aaaaaa\r\nb", 6},
	{" aaaaaa\x1b[0m", 6},
	{"a\n\n\nb", 6},
	{"\n\na", 6},
	{"a\n\n", 6},
	{"e\u0301e\u0301e\u0301", 2},
	{strings.Repeat("a\u0308", 5), 2},
	{"/" + strings.Repeat("a", 47) + "Cafe\u0301/x --resume", 52},
	{"aaa \u200b", 3},
	{"\u200d\t世", 1},
	{"abc\tdef\tghi", 5},
	{"abc\rdef", 5},
	{"aaaa\n\tbbbb cccc", 12},
}

var wrapPaths = map[string]func(string, int) []wrappedRow{
	"prose":   wrappedRows,
	"command": spaceWrappedRows,
}

func TestWrap_Properties(t *testing.T) {
	for name, wrap := range wrapPaths {
		t.Run("the "+name+" path holds every rule on the inputs that broke one", func(t *testing.T) {
			for _, c := range wrapLeakCases {
				checkWrapRules(t, wrap, c.text, c.width)
			}
		})

		t.Run("the "+name+" path holds every rule on generated inputs", func(t *testing.T) {
			rng := rand.New(rand.NewPCG(1, 2))
			for range wrapPropertyCases {
				text, width := generatedWrapInput(rng)
				if !checkWrapRules(t, wrap, text, width) {
					return
				}
			}
		})
	}

	t.Run("a capped wrap holds no line break and no row too wide", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(3, 4))
		check := func(text string, width, maxRows int) bool {
			for _, row := range wrapCappedAtSpaces(text, width, maxRows, "…") {
				if strings.ContainsAny(row, "\n\t\r") || ansi.StringWidth(row) > width {
					t.Errorf("wrapCappedAtSpaces(%q, %d, %d) gives row %q", text, width, maxRows, row)
					return false
				}
			}
			return true
		}
		for _, c := range wrapLeakCases {
			check(c.text, c.width, 2)
		}
		for range wrapPropertyCases {
			text, width := generatedWrapInput(rng)
			if !check(text, width, 1+rng.IntN(4)) {
				return
			}
		}
	})
}

func TestWrap_LeakCasesReadAsBefore(t *testing.T) {
	for _, c := range []struct {
		text  string
		width int
		want  []string
	}{
		{"-aaaaa\nb", 5, []string{"-aaaa", "a", "b"}},
		{"-aaaaa\n\nb", 5, []string{"-aaaa", "a", "", "b"}},
		{" a-a--b\nnext", 4, []string{"a-a-", "-b", "next"}},
		{strings.Repeat("a\u0308", 5), 2, []string{"a\u0308a\u0308", "a\u0308a\u0308", "a\u0308"}},
		{"aaa \u200b", 3, []string{"aaa"}},
		{"\u200d\t世", 1, []string{""}},
		{"abc\tdef", 3, []string{"abc", "def"}},
		{"abc\rdef", 3, []string{"abc", "def"}},
	} {
		if got := wrappedLines(c.text, c.width); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q at a width of %d wraps to\n got: %q\nwant: %q", c.text, c.width, got, c.want)
		}
	}
	if got, want := wrapCappedAtSpaces("aaaa\n\tbbbb cccc", 12, 1, "…"), []string{"aaaa bbbb c…"}; !reflect.DeepEqual(got, want) {
		t.Errorf("a line break and the next line's leading whitespace join as one space\n got: %q\nwant: %q", got, want)
	}
	if got, want := resumeCommandLines("e\u0301e\u0301e\u0301", 2), []string{"e\u0301e\u0301", "e\u0301"}; !reflect.DeepEqual(got, want) {
		t.Errorf("a decomposed command's rows read\n got: %q\nwant: %q", got, want)
	}
}

func TestNoticeBand_DrawsATabWithinTheBand(t *testing.T) {
	th := testDarkTheme(t)
	band := renderNoticeBand(bandInfo, "abc\tdef\tghi jkl mno pqr stu vwx yz", th.TextOnSelection, 30, th, true)
	for i, row := range strings.Split(band, "\n") {
		if w := lipgloss.Width(row); w > 30 {
			t.Errorf("row %d is %d cells wide in a 30-cell band: %q", i, w, row)
		}
	}
}

func generatedWrapInput(rng *rand.Rand) (string, int) {
	var text strings.Builder
	for range rng.IntN(wrapPropertyMaxLen + 1) {
		text.WriteString(wrapAlphabet[rng.IntN(len(wrapAlphabet))])
	}
	return text.String(), 1 + rng.IntN(wrapPropertyWidths)
}

// checkWrapRules reports each rule the rows of text break:
//  1. no row's text or gap holds a line break;
//  2. no row is wider than the width as a renderer draws it, so none holds a
//     tab or carriage return, which ansi.StringWidth counts as no cells;
//  3. a line showing nothing gives one empty row, and any other line at least
//     one row, none of them showing nothing;
//  4. the rows of each line show its visible text in order: each grapheme
//     taking a cell on something other than whitespace, but for one wider
//     than the width, which fits no row;
//  5. the rows run through the lines in order, each from one line;
//  6. no row starts inside a grapheme the row before it ends on.
func checkWrapRules(t *testing.T, wrap func(string, int) []wrappedRow, text string, width int) bool {
	t.Helper()
	rows := wrap(text, width)
	fail := func(rule string) bool {
		t.Errorf("%q at a width of %d breaks %s: rows %+v", text, width, rule, rows)
		return false
	}
	byLine := map[int][]wrappedRow{}
	for i, row := range rows {
		if strings.Contains(row.text, "\n") || strings.Contains(row.gap, "\n") {
			return fail("rule 1, a row holding a line break")
		}
		if ansi.StringWidth(row.text) > width || strings.ContainsAny(row.text, "\t\r") {
			return fail("rule 2, a row wider than the width")
		}
		if i > 0 && row.line != rows[i-1].line && row.line != rows[i-1].line+1 {
			return fail("rule 5, rows out of line order")
		}
		if i > 0 && row.line == rows[i-1].line && splitsGrapheme(rows[i-1].text, row.text) {
			return fail("rule 6, a row starting inside a grapheme")
		}
		byLine[row.line] = append(byLine[row.line], row)
	}
	lines := strings.Split(text, "\n")
	if len(rows) == 0 || rows[0].line != 0 || rows[len(rows)-1].line != len(lines)-1 {
		return fail("rule 5, rows that do not run through every line")
	}
	for i, line := range lines {
		want := shownText(line, width)
		var got strings.Builder
		for _, row := range byLine[i] {
			if want != "" && !showsACell(row.text) {
				return fail("rule 3, a row showing nothing")
			}
			got.WriteString(shownText(row.text, width))
		}
		if want == "" && (len(byLine[i]) != 1 || byLine[i][0].text != "") {
			return fail("rule 3, a line showing nothing given other than one empty row")
		}
		if got.String() != want {
			return fail("rule 4, line " + line + " shown as " + got.String())
		}
	}
	return true
}

// shownText is what s shows: its graphemes that take a cell on something other
// than whitespace, but for those too wide for any row.
func shownText(s string, width int) string {
	var shown strings.Builder
	for _, cluster := range graphemes(s) {
		if shows(cluster) && ansi.StringWidth(cluster) <= width {
			shown.WriteString(cluster)
		}
	}
	return shown.String()
}

// showsACell reads s grapheme by grapheme for one that shows, apart from the
// code under test's own judgement.
func showsACell(s string) bool {
	return slices.ContainsFunc(graphemes(s), shows)
}

func shows(cluster string) bool {
	base, _ := utf8.DecodeRuneInString(cluster)
	return ansi.StringWidth(cluster) > 0 && !unicode.IsSpace(base)
}

// splitsGrapheme reports whether next opens with what continues the grapheme
// before ends on, each read as it shows.
func splitsGrapheme(before, next string) bool {
	clusters := graphemes(before)
	if len(clusters) == 0 {
		return false
	}
	last := clusters[len(clusters)-1]
	joined, _ := ansi.FirstGraphemeCluster(last+ansi.Strip(next), ansi.GraphemeWidth)
	return len(joined) > len(last)
}

func graphemes(s string) []string {
	var clusters []string
	for rest := ansi.Strip(s); rest != ""; {
		cluster, _ := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		clusters = append(clusters, cluster)
		rest = rest[len(cluster):]
	}
	return clusters
}
