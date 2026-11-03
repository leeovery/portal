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

// keyFinals are the final bytes every CSI key form ends in: a read holding
// none of them has no repeat or release to cut.
const keyFinals = "u~PQS"

// FuzzScan feeds input to Scan as two reads, cut where the fuzzer chooses.
// forward is a prefix of the read unless a repeat or release was cut from
// it, so with nothing found and nothing a key form could end in, it is the
// whole read.
func FuzzScan(f *testing.F) {
	for _, seed := range []string{
		"\x1bd", "\x0f", "\x1b[100;3u", "\x1b[100;3:3u", "\x1b[1;3P", "\x1b[15;5:2~", "\x1b[27;3;100~",
		"\x1b[200~\x1bd\x1b[201~\x1bq", "\x1b]11;rgb:0/0/0\x07\x0f", "\x1b]0;t\x1b\\\x1b\\",
		"\x1bP1$r\x1b\x1bd", "\x1b[68;4u", "hello\x1b\x1b[A", "\x1b[?15u\x1b[<0;1;1M", "\x1bqa",
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
			if found != (action != 0) {
				t.Fatalf("Scan(%q) = (action %d, found %v); want found exactly when an action is", read, action, found)
			}
			if !isSubsequence(forward, read) {
				t.Fatalf("Scan(%q) forward = %q; want the read with only whole sequences taken out", read, forward)
			}
			if !found && !bytes.ContainsAny(read, keyFinals) && !bytes.Equal(forward, read) {
				t.Fatalf("Scan(%q) found nothing and forward = %q; want the whole read", read, forward)
			}

			forward, action, found = none.Scan(read)
			if !bytes.Equal(forward, read) || action != 0 || found {
				t.Fatalf("Scan(%q) with no bindings = (%q, %d, %v); want the whole read forwarded, nothing found", read, forward, action, found)
			}
		}
	})
}

// richKeyForms are forms of richKeys' bindings that match anywhere in a read.
var richKeyForms = []struct {
	form   string
	action attachkeys.Action
}{
	{"\x0f", attachkeys.Detach},
	{"\x1d", attachkeys.Kill},
	{"\x1b[100;3u", attachkeys.Detach},
	{"\x1b[100;67:1u", attachkeys.Detach},
	{"\x1b[100;4u", attachkeys.DetachToShell},
	{"\x1b[68;4u", attachkeys.DetachToShell},
	{"\x1b[113;3u", attachkeys.Kill},
	{"\x1b[1;3P", attachkeys.Detach},
	{"\x1b[15;5~", attachkeys.Detach},
	{"\x1b[27;3;100~", attachkeys.Detach},
}

// FuzzScan_FindsAKeyAfterText puts a bound key's form after arbitrary text,
// with neither an ESC nor a control byte, and anything after it: the key is
// found, the text forwarded and the rest dropped.
func FuzzScan_FindsAKeyAfterText(f *testing.F) {
	f.Add([]byte("hello"), uint8(0), []byte("\x1bd"))
	f.Add([]byte("héllo ✓"), uint8(3), []byte("\x1b[200~"))
	bindings := bindingsFor(f, richKeys)

	f.Fuzz(func(t *testing.T, prefix []byte, which uint8, suffix []byte) {
		text := bytes.Map(func(r rune) rune {
			if r < 0x20 {
				return -1
			}
			return r
		}, prefix)
		key := richKeyForms[int(which)%len(richKeyForms)]
		read := append(append(bytes.Clone(text), key.form...), suffix...)

		forward, action, found := attachkeys.NewMatcher(bindings).Scan(read)
		if !found || action != key.action || !bytes.Equal(forward, text) {
			t.Fatalf("Scan(%q) = (%q, %d, %v); want (%q, %d, true)", read, forward, action, found, text, key.action)
		}
	})
}
