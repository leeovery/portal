package vtcapture_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/vtcapture"
)

// captureForm is a capture by the pinned zmx build: the modes a session's
// script leaves on, the rows it leaves on its 120x24 screen, and the screen
// state that closes them. The integration lane holds each to what the build
// answers.
type captureForm struct {
	name, script       string
	lead, rows, closer string
}

var captureForms = []captureForm{
	{
		name:   "the cursor's placement and its pen",
		script: `printf 'row-1\r\nrow-2'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m",
	},
	{
		name:   "a pen left set",
		script: `printf 'row-1\r\nrow-2\033[1;31m'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m\x1b[1m\x1b[38;5;1m",
	},
	{
		name:   "a pen of direct colours",
		script: `printf 'row-1\r\nrow-2\033[3;38;2;10;200;30;48;5;4m'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m\x1b[3m\x1b[38;2;10;200;30m\x1b[48;5;4m",
	},
	{
		name:   "the working directory",
		script: `printf 'row-1\r\nrow-2\033]7;file://example-host/srv/demo\033\\'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m\x1b]7;file://example-host/srv/demo\x1b\\",
	},
	{
		name:   "a scrolling region",
		script: `printf 'row-1\r\nrow-2\033[3;20r'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[3;20r\x1b[1;1H\x1b[0m",
	},
	{
		name:   "modifyOtherKeys",
		script: `printf 'row-1\r\nrow-2\033[>4;2m'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[>4;2m\x1b[2;6H\x1b[0m",
	},
	{
		name:   "a wrap pending after the edge's cell",
		script: `printf 'row-1\r\n%0120d' 0`,
		rows:   "row-1\r\n" + strings.Repeat("0", 120),
		closer: "\x1b[2;120H0\x1b[0m",
	},
	{
		name:   "a wrap pending after a styled cell",
		script: `printf 'row-1\r\n%0119d\033[1;31mZ' 0`,
		rows:   "row-1\r\n" + strings.Repeat("0", 119) + "\x1b[0m\x1b[1m\x1b[38;5;1mZ\x1b[0m",
		closer: "\x1b[2;120H\x1b[0m\x1b[1m\x1b[38;5;1mZ\x1b[0m\x1b[0m\x1b[1m\x1b[38;5;1m",
	},
	{
		name:   "a wrap pending over a blank cell",
		script: `printf 'row-1\r\n%0119d \0337\033[X\0338' 0`,
		rows:   "row-1\r\n" + strings.Repeat("0", 119),
		closer: "\x1b[2;120H \x1b7\x1b[X\x1b8\x1b[0m",
	},
	{
		name:   "an open hyperlink",
		script: `printf 'row-1\r\nrow-2\033]8;;https://example.com\033\\'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m\x1b]8;;https://example.com\x1b\\",
	},
	{
		name:   "protection",
		script: `printf 'row-1\r\nrow-2\033[1"q'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m\x1b[1\"q",
	},
	{
		name:   "kitty keyboard flags",
		script: `printf 'row-1\r\nrow-2\033[>1u'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m\x1b[=1;1u",
	},
	{
		name:   "character sets and their shifts",
		script: `printf 'row-1\r\nrow-2\033(0\033)A\016\033~'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m\x1b(0\x1b)A\x0e\x1b~",
	},
	{
		name:   "every closing part at once",
		script: `printf 'row-1\r\nrow-2\033[>4;2m\033[1m\033]8;;https://example.com\033\\\033[1"q\033[>1u\033(0\033]7;file://h/d\033\\'`,
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[>4;2m\x1b[2;6H\x1b[0m\x1b[1m\x1b]8;;https://example.com\x1b\\\x1b[1\"q\x1b[=1;1u\x1b(0\x1b]7;file://h/d\x1b\\",
	},
	{
		name:   "a blank screen",
		script: `:`,
		closer: "\x1b[1;1H\x1b[0m",
	},
	{
		name:   "focus and mouse reporting left on",
		script: `printf '\033[?1004h\033[?1000hrow-1\r\nrow-2'`,
		lead:   "\x1b[?1000h\x1b[?1004h",
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m",
	},
	{
		name:   "the modes a program like Claude Code leaves on",
		script: `printf '\033[?2004h\033[?1004h\033[?25l\033[?7l\033[>1urow-1\r\nrow-2'`,
		lead:   "\x1b[?7l\x1b[?25l\x1b[?1004h\x1b[?2004h",
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m\x1b[=1;1u",
	},
	{
		name:   "ANSI modes, then DEC ones",
		script: `printf '\033[4h\033[20h\033[?1006h\033[?1002hrow-1\r\nrow-2'`,
		lead:   "\x1b[4h\x1b[20h\x1b[?1002h\x1b[?1006h",
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m",
	},
	{
		name:   "application cursor keys and keypad",
		script: `printf '\033[?1h\033=row-1\r\nrow-2'`,
		lead:   "\x1b[?1h\x1b[?66h",
		rows:   "row-1\r\nrow-2",
		closer: "\x1b[2;6H\x1b[0m",
	},
}

// The pinned build's own answer to History, kept by the zmx driver's tests.
func pinnedHistory(t *testing.T) []byte {
	t.Helper()
	history, err := os.ReadFile(filepath.Join("..", "backend", "zmx", "testdata", "history.vt"))
	if err != nil {
		t.Fatal(err)
	}
	return history
}

// rowsOf cuts capture as the hydrate helper does: the modes leading it, then
// the state closing what follows them.
func rowsOf(capture []byte) []byte {
	_, rows := vtcapture.Modes(capture)
	return vtcapture.TrimClose(rows)
}

func TestModesThenTrimClose(t *testing.T) {
	for _, tc := range captureForms {
		t.Run("it leaves the rows alone of a capture with "+tc.name, func(t *testing.T) {
			if got := rowsOf([]byte(tc.lead + tc.rows + tc.closer)); string(got) != tc.rows {
				t.Errorf("rows = %q, want %q", got, tc.rows)
			}
		})
	}

	t.Run("it leaves the rows alone of the pinned build's own History", func(t *testing.T) {
		history := pinnedHistory(t)
		const lead, closer = "\x1b[?2004h", "\x1b[>4;2m\x1b[24;120H0\x1b[0m\x1b]7;file://example-host/srv/demo\x1b\\"
		if !bytes.HasPrefix(history, []byte(lead)) || !bytes.HasSuffix(history, []byte(closer)) {
			t.Fatalf("history.vt is not led by %q and closed by %q", lead, closer)
		}
		if got, want := rowsOf(history), history[len(lead):len(history)-len(closer)]; !bytes.Equal(got, want) {
			t.Errorf("rows = %q, want %q", got, want)
		}
	})
}

func TestTrimClose(t *testing.T) {
	for _, tc := range captureForms {
		t.Run("it cuts the state closing a capture with "+tc.name, func(t *testing.T) {
			if got := vtcapture.TrimClose([]byte(tc.lead + tc.rows + tc.closer)); string(got) != tc.lead+tc.rows {
				t.Errorf("TrimClose = %q, want %q", got, tc.lead+tc.rows)
			}
		})
	}

	t.Run("it cuts the same state from the end of a capture alone", func(t *testing.T) {
		capture := []byte("row-1\r\nrow-2\r\nrow-3\x1b[3;6H\x1b[0m")
		if got := vtcapture.TrimClose(capture[7:]); string(got) != "row-2\r\nrow-3" {
			t.Errorf("TrimClose = %q, want the rows of the end alone", got)
		}
	})

	for _, tc := range []struct{ name, capture string }{
		{name: "a transcript not from zmx, with no placement", capture: "\x1b[1mrow-1\x1b[0m\r\nrow-2\r\n"},
		{name: "a placement among the rows", capture: "row-1\x1b[1;1H\x1b[0m\r\nrow-2"},
		{name: "a placement with no pen after it", capture: "row-1\r\nrow-2\x1b[2;6H"},
		{name: "a placement followed by a row", capture: "row-1\x1b[2;6H\x1b[0m\r\nrow-2\x1b[0m"},
		{name: "a state followed by more than zmx writes after it", capture: "row-1\x1b[1;6H\x1b[0m\x1b]7;file://h/d\x1b\\tail"},
	} {
		t.Run("it returns whole "+tc.name, func(t *testing.T) {
			if got := vtcapture.TrimClose([]byte(tc.capture)); string(got) != tc.capture {
				t.Errorf("TrimClose = %q, want it whole", got)
			}
		})
	}
}

func TestModes(t *testing.T) {
	t.Run("it reads each mode of the lead, in order, and returns what follows it", func(t *testing.T) {
		modes, rest := vtcapture.Modes([]byte("\x1b[4h\x1b[12l\x1b[?7l\x1b[?1;1004h\x1b[?2004hrow-1\x1b[?1049h"))
		want := []vtcapture.Mode{
			{Param: "4", Set: true},
			{Param: "12"},
			{Param: "7", Private: true},
			{Param: "1", Private: true, Set: true},
			{Param: "1004", Private: true, Set: true},
			{Param: "2004", Private: true, Set: true},
		}
		if !reflect.DeepEqual(modes, want) {
			t.Errorf("modes = %+v, want %+v", modes, want)
		}
		if string(rest) != "row-1\x1b[?1049h" {
			t.Errorf("rest = %q, want the rows on, a mode sequence among them unread", rest)
		}
	})

	for name, capture := range map[string]string{
		"rows":                   "row-1\r\nrow-2",
		"a pen":                  "\x1b[1mrow-1",
		"a placement":            "\x1b[1;1H\x1b[0m",
		"a step over blanks":     "\x1b[4Crow-1",
		"a sequence cut short":   "\x1b[?1049",
		"modifyOtherKeys":        "\x1b[>4;2mrow-1",
		"the text ?1049h":        "?1049h row-1",
		"nothing":                "",
		"a sequence ending in m": "\x1b[?1049mrow-1",
	} {
		t.Run("it reads no lead from a capture opening with "+name, func(t *testing.T) {
			if modes, rest := vtcapture.Modes([]byte(capture)); len(modes) != 0 || string(rest) != capture {
				t.Errorf("Modes = %+v, %q; want none, the capture whole", modes, rest)
			}
		})
	}
}

func TestAlternateScreen(t *testing.T) {
	for name, tc := range map[string]struct {
		vt   string
		want bool
	}{
		"vim, led by its modes":                 {"\x1b[?1h\x1b[?66h\x1b[?1049halt line\r\n\x1b[2;1H\x1b[0m", true},
		"1049 alone":                            {"\x1b[?1049halt line\x1b[2;6H\x1b[0m", true},
		"1047":                                  {"\x1b[?1047halt line", true},
		"47":                                    {"\x1b[?47halt line", true},
		"after modes zmx orders before it":      {"\x1b[?25l\x1b[?1049h\x1b[?2004halt line", true},
		"set with other modes in one sequence":  {"\x1b[?1;1049halt line", true},
		"after ANSI modes, IRM and LNM":         {"\x1b[4h\x1b[20h\x1b[?1049halt line", true},
		"after an ANSI mode reset, SRM":         {"\x1b[12l\x1b[?1049halt line", true},
		"after ANSI KAM, then DEC modes":        {"\x1b[2h\x1b[?1h\x1b[?1049halt line", true},
		"the main screen led by ANSI modes":     {"\x1b[4h\x1b[12l\x1b[?2004hmain line", false},
		"1049 as an ANSI mode, then main":       {"\x1b[1049h\x1b[?25lmain line", false},
		"the main screen":                       {"main line\r\nready\x1b[2;6H\x1b[0m", false},
		"the main screen led by its modes":      {"\x1b[?1h\x1b[?25l\x1b[?2004hmain line\r\n", false},
		"a mode sequence after the first cell":  {"main line\x1b[?1049halt", false},
		"a mode sequence after a cursor move":   {"\x1b[2;6H\x1b[?1049halt", false},
		"the alternate screen reset":            {"\x1b[?1049lmain line", false},
		"a mode that only ends in 1049":         {"\x1b[?11049hmain line", false},
		"a mode that only starts with 1049":     {"\x1b[?10490hmain line", false},
		"an ANSI mode, not a DEC private one":   {"\x1b[1049hmain line", false},
		"a sequence cut short":                  {"\x1b[?1049", false},
		"a sequence with no final mode byte":    {"\x1b[?1049mmain line", false},
		"the text ?1049h with no escape before": {"?1049h main line", false},
		"nothing":                               {"", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := vtcapture.AlternateScreen([]byte(tc.vt)); got != tc.want {
				t.Errorf("AlternateScreen(%q) = %v, want %v", tc.vt, got, tc.want)
			}
		})
	}
}
