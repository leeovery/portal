package attachkeys

import (
	"errors"
	"fmt"
	"strconv"
)

// introducers are the bytes that, after ESC, begin a longer sequence in a
// terminal's input. The matcher reads sequences by this table, so the
// bindings it refuses are exactly the ones it could not tell apart.
var introducers = map[byte]struct {
	name  string
	state scanState
}{
	'[': {"a CSI sequence", inCSI},
	'O': {"an SS3 sequence", inSS3},
	']': {"an OSC string", inString},
	'P': {"a DCS string", inString},
	'X': {"an SOS string", inString},
	'^': {"a PM string", inString},
	'_': {"an APC string", inString},
}

// plainKeyBytes are the control bytes keys without modifiers send. A binding
// sending one would fire on that key.
var plainKeyBytes = map[byte]string{
	'\t': "tab",
	'\n': "linefeed",
	'\r': "enter",
	'\b': "backspace",
	0x7f: "backspace",
	esc:  "escape",
}

// zmxDetach is ctrl+\ without the newer encoding: zmx's own detach key.
const zmxDetach = "\x1c"

// refusal says why k can't be bound, or nil. Each rule but the layout one
// follows from the bytes k sends without the newer encoding: a binding must
// not fire on another key, begin a longer sequence, or take zmx's detach key.
// Its errors complete a sentence whose subject is the key.
func refusal(k Key) error {
	if k.fn == 3 {
		return sameBytesAs("a cursor position report")
	}
	if k.fn != 0 {
		return nil
	}
	if k.mods&modShift != 0 && !isLetter(k.char) {
		return errors.New("puts shift on a digit or punctuation mark, whose bytes depend on the keyboard layout")
	}
	if less, lost := lostModifier(k); lost {
		return sameBytesAs(strconv.Quote(less.String()))
	}
	return legacyRefusal(k)
}

// lostModifier is k less the modifier its legacy bytes drop: ctrl on a key
// with no C0 byte, or shift beside ctrl.
func lostModifier(k Key) (Key, bool) {
	if k.mods&modCtrl == 0 {
		return Key{}, false
	}
	if _, ok := ctrlByte(k.char); !ok {
		return k.without(modCtrl), true
	}
	if k.mods&modShift != 0 {
		return k.without(modShift), true
	}
	return Key{}, false
}

func legacyRefusal(k Key) error {
	seq := k.legacy()
	if seq == zmxDetach {
		if k.char == '\\' {
			return errors.New("is zmx's own detach key")
		}
		return sameBytesAs(`"ctrl+\", zmx's own detach key`)
	}
	last := seq[len(seq)-1]
	if name, ok := plainKeyBytes[last]; ok {
		if k.mods&modAlt != 0 {
			name = "alt+" + name
		}
		return sameBytesAs(name)
	}
	if intro, ok := introducers[last]; ok {
		return fmt.Errorf("is the start of %s", intro.name)
	}
	return nil
}

func sameBytesAs(other string) error {
	return fmt.Errorf("sends the same bytes as %s", other)
}
