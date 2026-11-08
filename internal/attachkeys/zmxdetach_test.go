package attachkeys_test

import (
	"testing"

	"github.com/leeovery/portal/internal/attachkeys"
)

// zmx's client takes ctrl+\ for its detach as its C0 byte, or as a press or
// repeat in the CSI-u or modifyOtherKeys form, with lock bits tolerated.
func TestHoldsZmxDetach(t *testing.T) {
	for _, tc := range []struct {
		name string
		read string
		want bool
	}{
		{"the C0 byte alone", "\x1c", true},
		{"the C0 byte after typing, where a read of zmx's may begin", "ls\r\x1c", true},
		{"the C0 byte inside a paste", "\x1b[200~a\x1cb\x1b[201~", true},
		{"a CSI-u press", "\x1b[92;5u", true},
		{"a CSI-u press amid typing", "ab\x1b[92;5ucd", true},
		{"a CSI-u repeat", "\x1b[92;5:2u", true},
		{"a CSI-u press with lock bits", "\x1b[92;69u", true},
		{"a modifyOtherKeys press", "\x1b[27;5;92~", true},
		{"a CSI-u release", "\x1b[92;5:3u", false},
		{"ctrl+alt+\\", "\x1b[92;7u", false},
		{"a plain backslash", "\\", false},
		{"ctrl+] beside it", "\x1d", false},
		{"Portal's alt+d", "\x1bd", false},
		{"ordinary typing", "echo hello\r", false},
		{"nothing", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := attachkeys.HoldsZmxDetach([]byte(tc.read)); got != tc.want {
				t.Errorf("HoldsZmxDetach(%q) = %v, want %v", tc.read, got, tc.want)
			}
		})
	}
}
