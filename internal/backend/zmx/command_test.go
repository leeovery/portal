package zmx_test

import (
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
			{[]string{"portal", "state", "hydrate", "--file", "/a/b.bin", "--hook-key=x_y.z"}, "portal state hydrate --file /a/b.bin --hook-key=x_y.z"},
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
