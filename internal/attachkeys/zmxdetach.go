package attachkeys

import "strings"

// zmxDetachKey is ctrl+\, zmx's own detach key, which no binding may take.
var zmxDetachKey = Key{mods: modCtrl, char: '\\'}

// HoldsZmxDetach reports whether read, input on its way to zmx's client,
// holds zmx's own detach key pressed in a form the client takes for it: its
// C0 byte, or a press or repeat in the CSI-u or modifyOtherKeys form. zmx
// looks for the C0 byte only at the start of a read, but its reads need not
// split where the input's do, so the byte counts anywhere in read.
func HoldsZmxDetach(read []byte) bool {
	if strings.Contains(string(read), zmxDetach) {
		return true
	}
	for i := 0; i < len(read); {
		s := nextSequence(read, i)
		if s.kind == csiSeq && s.final != 0 {
			ev, ok := parseCSIKey(read[s.start+2:s.end-1], s.final)
			if ok && ev.key == zmxDetachKey.csi() && ev.event != release {
				return true
			}
		}
		i = s.end
	}
	return false
}
