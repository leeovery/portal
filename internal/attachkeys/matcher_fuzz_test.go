package attachkeys_test

import (
	"bytes"
	"strings"
	"testing"
	"unicode"

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
	{"\x0b", attachkeys.Kill},
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

// terminalReplies are what a terminal writes to its input other than keys:
// replies to queries, and the reports it sends unasked.
var terminalReplies = []string{
	"\x1b[12;40R", "\x1b[1;3R", "\x1b[1;5R", // cursor position, f3's bytes with modifiers
	"\x1b[?12;40;1R",                      // DECXCPR
	"\x1b[0n",                             // device status
	"\x1b[?62;22;52c", "\x1b[>1;4000;15c", // DA1, DA2
	"\x1b[?2004;1$y", "\x1b[2026;2$y", // DECRPM, private and ANSI
	"\x1b[?31u",                    // kitty keyboard flags
	"\x1b[?997;1n", "\x1b[?997;2n", // colour scheme
	"\x1b[I", "\x1b[O", // focus in and out
	"\x1b[<0;10;20M", "\x1b[<0;10;20m", "\x1b[<64;1;1M", // SGR mouse
	"\x1b[32;10;20M",                                     // urxvt mouse
	"\x1b[M !!", "\x1b[M\x7f\x7f\x7f", "\x1b[M#\xff\xa0", // X10 mouse
	"\x1b[8;24;80t", "\x1b[4;600;800t", // window size
	"\x1b]11;rgb:1a1a/1b1b/2626\x07", "\x1b]11;rgb:1a1a/1b1b/2626\x1b\\", // OSC 11
	"\x1b]10;rgb:ffff/ffff/ffff\x1b\\", "\x1b]4;1;rgb:cdcd/0000/0000\x07", // OSC 10, OSC 4
	"\x1b]52;c;aGVsbG8gd29ybGQ=\x07",                    // OSC 52
	"\x1bP>|kitty(0.40.1)\x1b\\", "\x1bP>|tmux 3.5\x07", // XTVERSION
	"\x1bP1$r0;1m\x1b\\", "\x1bP0$r\x1b\\", // DECRQSS
	"\x1bP1+r544e=787465726d2d6b69747479\x1b\\",                  // XTGETTCAP
	"\x1b_Gi=31;OK\x1b\\", "\x1b_Gi=31;ENOENT:no such image\x07", // kitty graphics
}

// withText, set in a pick, puts the text after the reply it picks.
const withText = 0x80

// splice joins the picked replies, each pick naming one and whether the text
// follows it.
func splice(picks, text []byte) []byte {
	var stream []byte
	for _, p := range picks {
		stream = append(stream, terminalReplies[int(p&^withText)%len(terminalReplies)]...)
		if p&withText != 0 {
			stream = append(stream, text...)
		}
	}
	return stream
}

// cutInto cuts stream into reads, each one byte longer than its cut, and the
// rest into one last read.
func cutInto(stream, cuts []byte) [][]byte {
	var reads [][]byte
	for _, c := range cuts {
		if len(stream) == 0 {
			break
		}
		n := min(int(c)+1, len(stream))
		reads, stream = append(reads, stream[:n]), stream[n:]
	}
	if len(stream) > 0 {
		reads = append(reads, stream)
	}
	return reads
}

func printable(s string) []byte {
	return []byte(strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return -1
		}
		return r
	}, s))
}

// FuzzScan_RepliesFindNothing splices terminal replies with typed text and
// cuts the stream into reads where the fuzzer chooses. No reply is a key,
// whole or in pieces, so under every binding at once each read is forwarded
// whole and nothing is found.
func FuzzScan_RepliesFindNothing(f *testing.F) {
	// The seeds pick every reply, with and without text after it, and read
	// them all at once, a reply to a read, and in reads of one to four bytes
	// at every offset.
	const seedText = "hé ✓ d"
	typed := printable(seedText)
	picks := make([]byte, len(terminalReplies))
	var byReply []byte
	for i, reply := range terminalReplies {
		picks[i] = byte(i)
		byReply = append(byReply, byte(len(reply)-1))
		if i%2 == 1 {
			picks[i] |= withText
			byReply = append(byReply, byte(len(typed)-1))
		}
	}
	f.Add(seedText, picks, []byte{})
	f.Add(seedText, picks, byReply)
	stream := splice(picks, typed)
	for width := 1; width <= 4; width++ {
		for offset := range width {
			var cuts []byte
			if offset > 0 {
				cuts = append(cuts, byte(offset-1))
			}
			f.Add(seedText, picks, append(cuts, bytes.Repeat([]byte{byte(width - 1)}, len(stream)/width+1)...))
		}
	}
	bindings := everyKey(f)

	f.Fuzz(func(t *testing.T, text string, picks, cuts []byte) {
		m := attachkeys.NewMatcher(bindings)
		for _, read := range cutInto(splice(picks, printable(text)), cuts) {
			forward, action, found := m.Scan(read)
			if found || action != 0 || !bytes.Equal(forward, read) {
				t.Fatalf("Scan(%q) = (%q, %d, %v); want the whole read forwarded, nothing found", read, forward, action, found)
			}
		}
	})
}
