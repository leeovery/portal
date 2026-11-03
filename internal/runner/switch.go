package runner

// switchSignature is what zmx's client writes as zmx switches it to another
// session in the same process: the terminal reset it leaves a session with,
// then the screen clear it begins each attach with. A detach writes the reset
// alone, and an attach's start the clear alone.
const switchSignature = terminalReset + "\x1b[0m\x1b[2J\x1b[H"

// signatureBorders holds, for each prefix of switchSignature, the length of
// the longest proper prefix of it that is also its suffix.
var signatureBorders = borders(switchSignature)

// switchWatch finds switchSignature in the child's output however its reads
// split it: a Knuth-Morris-Pratt matcher, its state how much of the signature
// the output read so far ends with.
type switchWatch struct{ matched int }

// scan reads the next of the output, and reports whether the signature ends
// in it.
func (w *switchWatch) scan(p []byte) bool {
	for _, c := range p {
		for w.matched > 0 && c != switchSignature[w.matched] {
			w.matched = signatureBorders[w.matched-1]
		}
		if c == switchSignature[w.matched] {
			w.matched++
		}
		if w.matched == len(switchSignature) {
			w.matched = signatureBorders[w.matched-1]
			return true
		}
	}
	return false
}

func borders(s string) []int {
	b := make([]int, len(s))
	for i, k := 1, 0; i < len(s); i++ {
		for k > 0 && s[i] != s[k] {
			k = b[k-1]
		}
		if s[i] == s[k] {
			k++
		}
		b[i] = k
	}
	return b
}
