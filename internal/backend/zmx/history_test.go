package zmx_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
)

// closingState is a History of the pinned zmx build: the rows a session's
// script leaves on its 120x24 screen, then the screen state zmx closes them
// with. The integration lane holds each to what the build answers.
type closingState struct {
	name, script, rows, state string
}

var closingStates = []closingState{
	{
		name:   "the cursor's placement and its pen",
		script: `printf 'row-1\r\nrow-2'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[2;6H\x1b[0m",
	},
	{
		name:   "a pen left set",
		script: `printf 'row-1\r\nrow-2\033[1;31m'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[2;6H\x1b[0m\x1b[1m\x1b[38;5;1m",
	},
	{
		name:   "a pen of direct colours",
		script: `printf 'row-1\r\nrow-2\033[3;38;2;10;200;30;48;5;4m'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[2;6H\x1b[0m\x1b[3m\x1b[38;2;10;200;30m\x1b[48;5;4m",
	},
	{
		name:   "the working directory",
		script: `printf 'row-1\r\nrow-2\033]7;file://example-host/srv/demo\033\\'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[2;6H\x1b[0m\x1b]7;file://example-host/srv/demo\x1b\\",
	},
	{
		name:   "a scrolling region",
		script: `printf 'row-1\r\nrow-2\033[3;20r'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[3;20r\x1b[1;1H\x1b[0m",
	},
	{
		name:   "modifyOtherKeys",
		script: `printf 'row-1\r\nrow-2\033[>4;2m'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[>4;2m\x1b[2;6H\x1b[0m",
	},
	{
		name:   "a wrap pending after the edge's cell",
		script: `printf 'row-1\r\n%0120d' 0`,
		rows:   "row-1\r\n" + strings.Repeat("0", 120),
		state:  "\x1b[2;120H0\x1b[0m",
	},
	{
		name:   "a wrap pending after a styled cell",
		script: `printf 'row-1\r\n%0119d\033[1;31mZ' 0`,
		rows:   "row-1\r\n" + strings.Repeat("0", 119) + "\x1b[0m\x1b[1m\x1b[38;5;1mZ\x1b[0m",
		state:  "\x1b[2;120H\x1b[0m\x1b[1m\x1b[38;5;1mZ\x1b[0m\x1b[0m\x1b[1m\x1b[38;5;1m",
	},
	{
		name:   "a wrap pending over a blank cell",
		script: `printf 'row-1\r\n%0119d \0337\033[X\0338' 0`,
		rows:   "row-1\r\n" + strings.Repeat("0", 119),
		state:  "\x1b[2;120H \x1b7\x1b[X\x1b8\x1b[0m",
	},
	{
		name:   "an open hyperlink",
		script: `printf 'row-1\r\nrow-2\033]8;;https://example.com\033\\'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[2;6H\x1b[0m\x1b]8;;https://example.com\x1b\\",
	},
	{
		name:   "protection",
		script: `printf 'row-1\r\nrow-2\033[1"q'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[2;6H\x1b[0m\x1b[1\"q",
	},
	{
		name:   "kitty keyboard flags",
		script: `printf 'row-1\r\nrow-2\033[>1u'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[2;6H\x1b[0m\x1b[=1;1u",
	},
	{
		name:   "character sets and their shifts",
		script: `printf 'row-1\r\nrow-2\033(0\033)A\016\033~'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[2;6H\x1b[0m\x1b(0\x1b)A\x0e\x1b~",
	},
	{
		name:   "every part at once",
		script: `printf 'row-1\r\nrow-2\033[>4;2m\033[1m\033]8;;https://example.com\033\\\033[1"q\033[>1u\033(0\033]7;file://h/d\033\\'`,
		rows:   "row-1\r\nrow-2",
		state:  "\x1b[>4;2m\x1b[2;6H\x1b[0m\x1b[1m\x1b]8;;https://example.com\x1b\\\x1b[1\"q\x1b[=1;1u\x1b(0\x1b]7;file://h/d\x1b\\",
	},
	{
		name:   "a blank screen",
		script: `:`,
		rows:   "",
		state:  "\x1b[1;1H\x1b[0m",
	},
}

func TestHistoryContent(t *testing.T) {
	for _, tc := range closingStates {
		t.Run("it cuts the screen state zmx closes a History with: "+tc.name, func(t *testing.T) {
			if got := zmx.HistoryContent([]byte(tc.rows + tc.state)); string(got) != tc.rows {
				t.Errorf("HistoryContent = %q, want the rows alone, %q", got, tc.rows)
			}
		})
	}

	t.Run("it cuts the state closing the pinned build's own History", func(t *testing.T) {
		history, err := os.ReadFile(filepath.Join("testdata", "history.vt"))
		if err != nil {
			t.Fatal(err)
		}
		const state = "\x1b[>4;2m\x1b[24;120H0\x1b[0m\x1b]7;file://example-host/srv/demo\x1b\\"
		if !bytes.HasSuffix(history, []byte(state)) {
			t.Fatalf("history.vt does not end in %q", state)
		}
		if got, want := zmx.HistoryContent(history), history[:len(history)-len(state)]; !bytes.Equal(got, want) {
			t.Errorf("HistoryContent cut %q, want %q", history[len(got):], state)
		}
	})

	t.Run("it cuts the same state from the end of a History alone", func(t *testing.T) {
		history := []byte("row-1\r\nrow-2\r\nrow-3\x1b[3;6H\x1b[0m")
		end := history[7:]
		if got := zmx.HistoryContent(end); string(got) != "row-2\r\nrow-3" {
			t.Errorf("HistoryContent = %q, want the rows of the end alone", got)
		}
	})

	for _, tc := range []struct{ name, history string }{
		{name: "a transcript not from zmx, with no placement", history: "\x1b[1mrow-1\x1b[0m\r\nrow-2\r\n"},
		{name: "a placement among the rows", history: "row-1\x1b[1;1H\x1b[0m\r\nrow-2"},
		{name: "a placement with no pen after it", history: "row-1\r\nrow-2\x1b[2;6H"},
		{name: "a placement followed by a row", history: "row-1\x1b[2;6H\x1b[0m\r\nrow-2\x1b[0m"},
		{name: "a state followed by more than zmx writes after it", history: "row-1\x1b[1;6H\x1b[0m\x1b]7;file://h/d\x1b\\tail"},
	} {
		t.Run("it returns whole "+tc.name, func(t *testing.T) {
			if got := zmx.HistoryContent([]byte(tc.history)); string(got) != tc.history {
				t.Errorf("HistoryContent = %q, want it whole", got)
			}
		})
	}
}
