package attachkeys_test

import (
	"bytes"
	"testing"

	"github.com/leeovery/portal/internal/attachkeys"
)

// isSubsequence reports whether sub is seq with zero or more bytes taken out.
func isSubsequence(sub, seq []byte) bool {
	for _, b := range sub {
		i := bytes.IndexByte(seq, b)
		if i < 0 {
			return false
		}
		seq = seq[i+1:]
	}
	return true
}

// FuzzScan feeds input to Scan as two reads, cut where the fuzzer chooses.
// forward is a prefix of the read unless a repeat or release was cut from
// it, so the property checked is the weaker one that holds for both.
func FuzzScan(f *testing.F) {
	for _, seed := range []string{
		"\x1bd", "\x0f", "\x1b[100;3u", "\x1b[100;3:3u", "\x1b[1;3P", "\x1b[15;5:2~",
		"\x1b[200~\x1bd\x1b[201~\x1bq", "\x1b]11;rgb:0/0/0\x07\x07", "\x1b]0;t\x1b\\\x1b\\",
		"\x1bP1$r\x1b\x1bd", "\x1b[49:33;4u", "hello\x1b\x1b[A", "\x1b[?15u\x1b[<0;1;1M",
	} {
		f.Add([]byte(seed), uint8(len(seed)/2))
	}
	bindings := bindingsFor(f, richKeys)

	f.Fuzz(func(t *testing.T, input []byte, cut uint8) {
		split := int(cut) % (len(input) + 1)
		bound := attachkeys.NewMatcher(bindings)
		none := attachkeys.NewMatcher(attachkeys.Bindings{})
		for _, read := range [][]byte{input[:split], input[split:]} {
			original := bytes.Clone(read)

			forward, action, found := bound.Scan(read)
			if !bytes.Equal(read, original) {
				t.Fatalf("Scan(%q) changed its read to %q", original, read)
			}
			if !isSubsequence(forward, read) {
				t.Fatalf("Scan(%q) forward = %q; want the read with only whole sequences taken out", read, forward)
			}
			if found != (action != 0) {
				t.Fatalf("Scan(%q) = (action %d, found %v); want found exactly when an action is", read, action, found)
			}

			forward, action, found = none.Scan(read)
			if !bytes.Equal(forward, read) || action != 0 || found {
				t.Fatalf("Scan(%q) with no bindings = (%q, %d, %v); want the whole read forwarded, nothing found", read, forward, action, found)
			}
		}
	})
}
