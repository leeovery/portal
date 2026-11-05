package tui

import (
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// wrapAlphabet provokes the over-pack: hyphens and spaces beside words, line
// breaks with and without a carriage return, a tab, a rune two cells wide and
// an SGR pair.
var wrapAlphabet = []string{"a", "a", "a", "-", " ", "\n", "\r", "\t", "世", "\x1b[1m", "\x1b[0m"}

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
			for _, row := range wrapCapped(text, width, maxRows, "…") {
				if strings.Contains(row, "\n") || ansi.StringWidth(row) > width {
					t.Errorf("wrapCapped(%q, %d, %d) gives row %q", text, width, maxRows, row)
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
	} {
		if got := wrappedLines(c.text, c.width); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q at a width of %d wraps to\n got: %q\nwant: %q", c.text, c.width, got, c.want)
		}
	}
	if got, want := wrapCapped("-aaaaa\n\nb", 5, 2, "…"), []string{"-aaaa", "a b…"}; !reflect.DeepEqual(got, want) {
		t.Errorf("the capped rows read\n got: %q\nwant: %q", got, want)
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
//  2. no row is wider than the width;
//  3. a line showing nothing gives one empty row, and any other line at least
//     one row, none of them showing nothing;
//  4. the rows of each line show its visible text in order, but for a
//     grapheme wider than the width, which fits no row;
//  5. the rows run through the lines in order, each from one line.
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
		if ansi.StringWidth(row.text) > width {
			return fail("rule 2, a row wider than the width")
		}
		if i > 0 && row.line != rows[i-1].line && row.line != rows[i-1].line+1 {
			return fail("rule 5, rows out of line order")
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
			if want != "" && showsNothing(row.text) {
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

// shownText is what s shows, whitespace aside, without the graphemes too wide
// for any row.
func shownText(s string, width int) string {
	var shown strings.Builder
	for rest := ansi.Strip(s); rest != ""; {
		cluster, w := ansi.FirstGraphemeCluster(rest, ansi.GraphemeWidth)
		rest = rest[len(cluster):]
		if w <= width && strings.TrimFunc(cluster, unicode.IsSpace) != "" {
			shown.WriteString(cluster)
		}
	}
	return shown.String()
}
