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

// csiEvent is a key event read from one CSI sequence.
type csiEvent struct {
	key csiKey
	// shifted is the CSI-u shifted-key alternate; zero when absent.
	shifted int
	event   int
}

// parseCSIKey reads a key event from a CSI sequence's parameter bytes and final
// byte, in the kitty keyboard protocol's grammar, which a function key's
// legacy form also follows:
//
//	CSI code[:shifted[:base]] [; mods[:event] [; text]] u
//	CSI number [; mods[:event]] P|Q|R|S|~
func parseCSIKey(params []byte, final byte) (csiEvent, bool) {
	fields := strings.Split(string(params), ";")
	if len(fields) > maxFields(final) {
		return csiEvent{}, false
	}
	number, shifted, ok := keyField(fields[0], final)
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
	return csiEvent{key: csiKey{number: number, final: final, mods: m}, shifted: shifted, event: event}, true
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

// keyField reads the key code and the CSI-u alternates beside it. Only the
// CSI-u form carries alternates; a base-layout key is checked and set aside.
func keyField(text string, final byte) (number, shifted int, ok bool) {
	codes := strings.Split(text, ":")
	if len(codes) > 3 || len(codes) > 1 && final != 'u' {
		return 0, 0, false
	}
	if number, ok = decimal(codes[0]); !ok {
		return 0, 0, false
	}
	if len(codes) > 1 && codes[1] != "" {
		if shifted, ok = decimal(codes[1]); !ok {
			return 0, 0, false
		}
	}
	if len(codes) > 2 {
		if _, ok = decimal(codes[2]); !ok {
			return 0, 0, false
		}
	}
	return number, shifted, true
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
