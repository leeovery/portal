package zmx_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
)

func TestDisplayCommand(t *testing.T) {
	t.Run("it quotes the arguments zmx quotes, and only those", func(t *testing.T) {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{nil, ""},
			{[]string{"sleep", "1000"}, "sleep 1000"},
			{[]string{"portal", "state", "hydrate", "--session", "x_y.z", "--file", "/a/b.bin"}, "portal state hydrate --session x_y.z --file /a/b.bin"},
			{[]string{"sh", "-c", "echo a b"}, "sh -c 'echo a b'"},
			{[]string{"echo", ""}, "echo ''"},
			{[]string{"echo", "it's"}, `echo 'it'\''s'`},
			{[]string{"a\nb", "c\td", "$x", "`x`", "!", "(", "{", "[", "|", "&", ";", "<", "?", "*", "~", "#", `\`, `"`}, "'a\nb' 'c\td' '$x' '`x`' '!' '(' '{' '[' '|' '&' ';' '<' '?' '*' '~' '#' '\\' '\"'"},
			{[]string{"ünï", "a=b", "a:b", "@x", "%y", "+z", ","}, "ünï a=b a:b @x %y +z ,"},
		} {
			if got := zmx.DisplayCommand(tc.args); got != tc.want {
				t.Errorf("DisplayCommand(%q) = %q, want %q", tc.args, got, tc.want)
			}
		}
	})

	t.Run("it fills up to 256 bytes and marks the first argument that would pass them", func(t *testing.T) {
		fill := strings.Repeat("a", 250)
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{fill, "bbbbb"}, fill + " bbbbb"},
			{[]string{fill, "bbbbbb"}, fill + "..."},
			{[]string{fill, "bbbbbb", "c"}, fill + "..."},
			{[]string{strings.Repeat("a", 254), "bb"}, strings.Repeat("a", 254)},
			{[]string{strings.Repeat("a", 300)}, "..."},
		} {
			if got := zmx.DisplayCommand(tc.args); got != tc.want {
				t.Errorf("DisplayCommand of %d args = %d bytes %q…, want %d bytes", len(tc.args), len(got), got[max(0, len(got)-8):], len(tc.want))
			}
		}
	})
}

func TestCommandBegins(t *testing.T) {
	fill := strings.Repeat("a", 240)
	for _, tc := range []struct {
		name   string
		shown  string
		start  []string
		begins bool
	}{
		{"the start alone", "sh -c 'echo a b'", []string{"sh", "-c", "echo a b"}, true},
		{"the start, then an argument", "sh -c 'echo a b' x", []string{"sh", "-c", "echo a b"}, true},
		{"the start, then the mark", fill + " bb...", []string{fill, "bb"}, true},
		{"a start cut within, shown as it alone is", fill + " bb...", []string{fill, "bb", strings.Repeat("c", 20)}, true},
		{"a start cut within, the report going on past it", fill + " bb... x", []string{fill, "bb", strings.Repeat("c", 20)}, false},
		{"the start, then the mark, past 256 bytes", strings.Repeat("a", 254) + "...", []string{strings.Repeat("a", 254)}, false},
		{"the start, then more than the mark", fill + " bb....", []string{fill, "bb"}, false},
		{"a start whose last argument begins the next shown", "sh -c 'echo a b' xy", []string{"sh", "-c", "echo a b", "x"}, false},
		{"a start whose last argument begins the one cut", fill + " bb...", []string{fill, "b"}, false},
		{"a start shown unquoted where the report quotes it", "sh -c 'echo a b'", []string{"sh", "-c", "echo"}, false},
		{"another start", "vim notes.md", []string{"vi"}, false},
	} {
		t.Run(fmt.Sprintf("%s: %v", tc.name, tc.begins), func(t *testing.T) {
			if got := zmx.CommandBegins(tc.shown, tc.start); got != tc.begins {
				t.Errorf("CommandBegins(%q, %q) = %v, want %v", tc.shown, tc.start, got, tc.begins)
			}
		})
	}
}
