package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// zmxHistory is the pinned zmx build's own History answer for the session its
// TestHistoryFixture draws, in the driver's testdata.
func zmxHistory(t *testing.T) []byte {
	t.Helper()
	vt, err := os.ReadFile(repoRelative("internal", "backend", "zmx", "testdata", "history.vt"))
	if err != nil {
		t.Fatal(err)
	}
	return vt
}

func historyRows(vt string) []string {
	return strings.Split(renderHistory([]byte(vt)), "\n")
}

func plainRows(vt string) []string {
	rows := historyRows(vt)
	for i, row := range rows {
		rows[i] = ansi.Strip(row)
	}
	return rows
}

// nonSGR lists every escape sequence in s other than SGR.
func nonSGR(s string) []string {
	p := ansi.NewParser()
	var found []string
	var state byte
	for len(s) > 0 {
		seq, width, n, next := ansi.DecodeSequence(s, state, p)
		state, s = next, s[n:]
		if width > 0 || seq == "\n" {
			continue
		}
		if ansi.HasCsiPrefix(seq) && p.Command() == 'm' {
			continue
		}
		found = append(found, seq)
	}
	return found
}

func TestRenderHistory_DrawsZmxsOwnHistoryAsTheTerminalWould(t *testing.T) {
	rendered := renderHistory(zmxHistory(t))
	rows := strings.Split(rendered, "\n")

	var want []string
	for i := 1; i <= 30; i++ {
		want = append(want, fmt.Sprintf("scrollback %d", i))
	}
	want = append(want,
		"bold red truecolour",
		"gap     after-tab",
		"blue to the edge"+strings.Repeat(" ", 104),
		"wide 中文 end",
		"link text",
		"erased",
		"last row "+strings.Repeat("0", 111),
	)
	plain := make([]string, len(rows))
	for i, row := range rows {
		plain[i] = ansi.Strip(row)
	}
	if !slices.Equal(plain, want) {
		t.Fatalf("rows =\n%q\nwant\n%q", plain, want)
	}
	if got := nonSGR(rendered); len(got) != 0 {
		t.Errorf("the drawn history carries non-SGR sequences %q; only SGR may reach the viewport", got)
	}
	if w := ansi.StringWidth(rows[len(rows)-1]); w != 120 {
		t.Errorf("last row is %d cells, want 120: the pending wrap's reprint must not land past the edge", w)
	}
}

func TestRenderHistory_KeepsTheSessionsColours(t *testing.T) {
	rows := strings.Split(renderHistory(zmxHistory(t)), "\n")

	bold := rows[30]
	if !strings.Contains(bold, "\x1b[38;5;1;1mbold red") {
		t.Errorf("bold red row lost its bold red: %q", bold)
	}
	if !strings.Contains(bold, "38;2;10;200;30mtruecolour") {
		t.Errorf("truecolour run lost its colour: %q", bold)
	}
	blue := rows[32]
	if !strings.Contains(blue, "\x1b[48;5;4mblue to the edge"+strings.Repeat(" ", 104)) {
		t.Errorf("the background run to the edge lost its background: %q", blue)
	}
}

func TestRenderHistory_StepsOverBlankCellsWithCUF(t *testing.T) {
	for vt, want := range map[string]string{
		"a\x1b[3Cb":  "a   b",
		"a\x1b[Cb":   "a b",
		"a\x1b[0Cb":  "a b",
		"\x1b[2Cend": "  end",
	} {
		if got := plainRows(vt); !slices.Equal(got, []string{want}) {
			t.Errorf("renderHistory(%q) = %q, want %q", vt, got, want)
		}
	}
}

func TestRenderHistory_ErasesWithTheBackgroundAndLeavesTheCursor(t *testing.T) {
	rows := historyRows("\x1b[44mab\x1b[3X\x1b[3Cc\x1b[0m")

	if got := ansi.Strip(rows[0]); got != "ab   c" {
		t.Fatalf("row = %q, want %q", got, "ab   c")
	}
	if !strings.Contains(rows[0], "44mab   c") {
		t.Errorf("erased cells lost the background: %q", rows[0])
	}
}

func TestRenderHistory_EndsTheRowsAtTheCursorPosition(t *testing.T) {
	// The pending-wrap reprint zmx writes after the cursor's CUP repeats the
	// cell already drawn there.
	got := plainRows("first\r\nlast row\x1b[2;8Hw\x1b[0m\x1b[1;24r\x1b]7;file://host/tmp\x1b\\")

	if !slices.Equal(got, []string{"first", "last row"}) {
		t.Errorf("rows = %q, want the rows before the CUP alone", got)
	}
}

func TestRenderHistory_DrawsNothingForTerminalState(t *testing.T) {
	// Modes before the rows, keyboard modes after: a CSI > 4 ; 2 m read as
	// SGR would underline and dim what follows.
	rows := historyRows("\x1b[?1049h\x1b[?2004h\x1b[4hplain\x1b[>4;2mstill plain\x1b[0 q")

	if got := ansi.Strip(rows[0]); got != "plainstill plain" {
		t.Fatalf("row = %q, want the text alone", got)
	}
	if rows[0] != "plainstill plain" {
		t.Errorf("row = %q, want no styling from a mode or keyboard sequence", rows[0])
	}
}

func TestRenderHistory_KeepsALinksTextButNotTheLink(t *testing.T) {
	rows := historyRows("\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\ text")

	if rows[0] != "link text" {
		t.Errorf("row = %q, want the link's text with no OSC 8", rows[0])
	}
}

func TestRenderHistory_LaysWideRunesOutByWidth(t *testing.T) {
	rows := historyRows("中文x")

	if rows[0] != "中文x" || ansi.StringWidth(rows[0]) != 5 {
		t.Errorf("row = %q (%d cells), want 中文x in 5 cells", rows[0], ansi.StringWidth(rows[0]))
	}
}

func TestRenderHistory_ABareLineFeedAlsoReturnsTheCarriage(t *testing.T) {
	if got := plainRows("one\ntwo\r\nthree"); !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Errorf("rows = %q, want one row per line", got)
	}
}

func TestRenderHistory_KeepsBlankRowsBetweenDrawnOnes(t *testing.T) {
	if got := plainRows("top\r\n\r\n\r\nbottom\r\n"); !slices.Equal(got, []string{"top", "", "", "bottom"}) {
		t.Errorf("rows = %q, want the blank rows kept and nothing after the last drawn row", got)
	}
}

func TestRenderHistory_ClosesEveryStyledRow(t *testing.T) {
	// A style left open at a row's end carries on to the next row's text, as
	// the terminal's pen does, but each row closes its own.
	rows := historyRows("\x1b[41mred\r\nstill red")

	for i, row := range rows {
		if !strings.HasSuffix(row, ansi.ResetStyle) {
			t.Errorf("row %d = %q, want it closed with a reset", i, row)
		}
		if !strings.Contains(row, "41m") {
			t.Errorf("row %d = %q, want the red background the pen carries", i, row)
		}
	}
}

// The screens below are written as ghostty's VT formatter writes them, which
// is how zmx serialises History: rows joined by CR LF, then terminal state.

func TestRenderHistory_DrawsTheAlternateScreensRows(t *testing.T) {
	// A session in a full-screen program: the mode leads, then that screen's
	// rows, then the cursor.
	got := plainRows("\x1b[?1049h\x1b[?25ltop bar\r\n\r\nstatus\x1b[3;7H\x1b[0m")

	if !slices.Equal(got, []string{"top bar", "", "status"}) {
		t.Errorf("rows = %q, want the alternate screen's rows as drawn", got)
	}
}

func TestRenderHistory_DrawsASoftWrappedRowAsTheScreenShowedIt(t *testing.T) {
	// A line longer than a 10-column screen wraps; the formatter, not
	// unwrapping, ends the screen row with CR LF like any other.
	got := plainRows("0123456789\r\nabc\x1b[2;4H")

	if !slices.Equal(got, []string{"0123456789", "abc"}) {
		t.Errorf("rows = %q, want the wrapped line on the two rows it took", got)
	}
}

func TestRenderHistory_DrawsAWideCharacterWrappedAtTheEdge(t *testing.T) {
	// "1A⚡" on a 3-column screen: ⚡ cannot start in the last column, which is
	// left a spacer head the formatter skips, and lands on the next row.
	rows := historyRows("1A\r\n⚡\x1b[2;3H")

	if !slices.Equal(rows, []string{"1A", "⚡"}) {
		t.Errorf("rows = %q, want 1A then ⚡ alone", rows)
	}
	if w := ansi.StringWidth(rows[1]); w != 2 {
		t.Errorf("⚡ row is %d cells, want 2", w)
	}
}

func TestRenderHistory_DrawsNothingOfABlankCellsPendingWrap(t *testing.T) {
	// A cursor left pending-wrap over a blank last cell is restored by
	// printing a space, then erasing it under a saved cursor.
	got := historyRows("full row\r\nnext\x1b[2;10H \x1b7\x1b[X\x1b8\x1b[0m")

	if !slices.Equal(got, []string{"full row", "next"}) {
		t.Errorf("rows = %q, want the rows alone, with no space drawn past them", got)
	}
}

func TestRenderHistory_DrawsNothingOfAScrollRegion(t *testing.T) {
	// Margins come after the rows, ahead of the cursor's CUP.
	got := plainRows("one\r\ntwo\r\nthree\x1b[2;3r\x1b[3;6H")

	if !slices.Equal(got, []string{"one", "two", "three"}) {
		t.Errorf("rows = %q, want the rows unmoved by the margins", got)
	}
}

func TestRenderHistory_JoinsACombiningMarkToItsBase(t *testing.T) {
	// The decoder hands a mark after an ASCII base over on its own, with no
	// width; it belongs to the cell before it.
	rows := historyRows("café ok")

	if rows[0] != "café ok" {
		t.Errorf("row = %q, want café ok with its accent", rows[0])
	}
	if w := ansi.StringWidth(rows[0]); w != 7 {
		t.Errorf("row is %d cells, want 7", w)
	}
}

func TestRenderHistory_DrawsAKeycapAtTheWidthItIsMeasuredAt(t *testing.T) {
	// The variation selector makes 1️⃣ an emoji, two cells wide as the viewport
	// measures it, so the text after it lands past both.
	rows := historyRows("1️⃣ x")

	if rows[0] != "1️⃣ x" {
		t.Errorf("row = %q, want the whole keycap then x", rows[0])
	}
	if w := ansi.StringWidth(rows[0]); w != 4 {
		t.Errorf("keycap row is %d cells, want 4: keycap 2, space, x", w)
	}
}

func TestRenderHistory_DropsAMarkWithNothingToJoin(t *testing.T) {
	if got := plainRows("a\r\ńb"); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("rows = %q, want a mark that follows no cell on its row dropped", got)
	}
}

func TestRenderHistory_BoundsAStepOrEraseCount(t *testing.T) {
	for _, vt := range []string{"a\x1b[100000Cb", "a\x1b[100000Xb"} {
		rows := historyRows(vt)

		if w := ansi.StringWidth(rows[0]); w > historyMaxColumns+1 {
			t.Errorf("renderHistory(%q) drew a row %d cells wide, want at most %d", vt, w, historyMaxColumns+1)
		}
	}
}
