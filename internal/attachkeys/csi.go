package attachkeys

import (
	"strconv"
	"strings"
)

// A CSI key event's type, in the kitty protocol.
const (
	press   = 1
	release = 3
)

// lockMods are caps lock and num lock: ambient state, not part of a key.
const lockMods mods = 64 | 128

// modifyOtherKeysNumber begins xterm's modifyOtherKeys form, CSI 27;mods;code~.
const modifyOtherKeysNumber = "27"

// csiEvent is a key event read from one CSI sequence.
type csiEvent struct {
	key   csiKey
	event int
}

// parseCSIKey reads a key event from a CSI sequence's parameter bytes and final
// byte. It reads the kitty keyboard protocol's grammar, which a function key's
// legacy form also follows, and xterm's modifyOtherKeys form:
//
//	CSI code[:shifted[:base]] [; mods[:event] [; text]] u
//	CSI number [; mods[:event]] P|Q|R|S|~
//	CSI 27 ; mods ; code ~
func parseCSIKey(params []byte, final byte) (csiEvent, bool) {
	fields := strings.Split(string(params), ";")
	if final == '~' && len(fields) == 3 && fields[0] == modifyOtherKeysNumber {
		return modifyOtherKey(fields[1], fields[2])
	}
	if len(fields) > maxFields(final) {
		return csiEvent{}, false
	}
	number, ok := keyField(fields[0], final)
	if !ok {
		return csiEvent{}, false
	}
	modsText := "1"
	if len(fields) > 1 {
		modsText = fields[1]
	}
	m, event, ok := modsField(modsText)
	if !ok {
		return csiEvent{}, false
	}
	if len(fields) > 2 && !isCodepoints(fields[2]) {
		return csiEvent{}, false
	}
	return csiEvent{key: canonical(csiKey{number: number, final: final, mods: m}), event: event}, true
}

func modifyOtherKey(modsText, codeText string) (csiEvent, bool) {
	m, event, ok := modsField(modsText)
	if !ok {
		return csiEvent{}, false
	}
	code, ok := decimal(codeText)
	if !ok {
		return csiEvent{}, false
	}
	return csiEvent{key: canonical(csiKey{number: code, final: 'u', mods: m}), event: event}, true
}

// canonical names a letter by its lowercase code, as kitty sends it.
// modifyOtherKeys, fixterms and iTerm2's CSI u mode send the character typed:
// uppercase beside the shift bit, or with caps lock on, which they carry no
// bit for.
func canonical(k csiKey) csiKey {
	if k.final == 'u' && k.number >= 'A' && k.number <= 'Z' {
		k.number += 'a' - 'A'
	}
	return k
}

// maxFields is how many ;-separated fields a key sequence with this final
// byte holds; zero for a final byte no key ends in.
func maxFields(final byte) int {
	switch final {
	case 'u':
		return 3
	case 'P', 'Q', 'R', 'S', '~':
		return 2
	}
	return 0
}

// keyField reads the number that names the key. Only the CSI-u form carries
// alternates beside its code: the shifted key, set aside, and the key's place
// on a US layout. That names the key when the code is not ASCII, as on a
// Cyrillic layout, which kitty sends it for. On a Latin layout the code
// stays: an AZERTY a, in a US q's place, is not alt+q.
func keyField(field string, final byte) (int, bool) {
	codes := strings.Split(field, ":")
	if len(codes) > 3 || len(codes) > 1 && final != 'u' {
		return 0, false
	}
	number, ok := decimal(codes[0])
	if !ok {
		return 0, false
	}
	if len(codes) > 1 && codes[1] != "" {
		if _, ok := decimal(codes[1]); !ok {
			return 0, false
		}
	}
	if len(codes) < 3 {
		return number, true
	}
	base, ok := decimal(codes[2])
	if !ok {
		return 0, false
	}
	if number > 0x7f {
		number = base
	}
	return number, true
}

// modsField reads "mods[:event]", where mods is one more than the bitfield.
func modsField(text string) (mods, int, bool) {
	modText, eventText, hasEvent := strings.Cut(text, ":")
	m, ok := decimal(modText)
	if !ok || m < 1 {
		return 0, 0, false
	}
	event := press
	if hasEvent {
		if event, ok = decimal(eventText); !ok || event < press || event > release {
			return 0, 0, false
		}
	}
	return mods(m-1) &^ lockMods, event, true
}

func isCodepoints(text string) bool {
	for code := range strings.SplitSeq(text, ":") {
		if _, ok := decimal(code); !ok {
			return false
		}
	}
	return true
}

// decimal reads an unsigned decimal of up to seven digits, enough for any
// codepoint and short of overflow.
func decimal(s string) (int, bool) {
	if s == "" || len(s) > 7 {
		return 0, false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}
