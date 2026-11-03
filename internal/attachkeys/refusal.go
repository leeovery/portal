package attachkeys

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// introducers are the bytes that, after ESC, begin a longer sequence in a
// terminal's input. The matcher reads sequences by this table, so the
// bindings it refuses are exactly the ones it could not tell apart.
var introducers = map[byte]struct {
	name string
	kind seqKind
}{
	'[': {"a CSI sequence", csiSeq},
	'O': {"an SS3 sequence", ss3Seq},
	']': {"an OSC string", stringSeq},
	'P': {"a DCS string", stringSeq},
	'X': {"an SOS string", stringSeq},
	'^': {"a PM string", stringSeq},
	'_': {"an APC string", stringSeq},
}

// otherKeyBytes are the control bytes keys no binding names send. A binding
// sending one would fire on that key.
var otherKeyBytes = map[byte]string{
	'\t': "tab",
	'\n': "linefeed",
	'\r': "enter",
	'\b': "backspace",
	0x7f: "backspace",
	esc:  "escape",
}

// replyTerminators end the strings terminals send in reply to a query. A read
// can begin with one, split from the rest of its reply, and the matcher reads
// it as typing.
var replyTerminators = map[string]string{
	"\x07":   "BEL",
	"\x1b\\": "ST",
}

// usShifted are the punctuation marks a US layout types with shift.
const usShifted = `!"#$%&()*+:<>?@^_{|}~`

// zmxDetach is ctrl+\ without the newer encoding: zmx's own detach key.
const zmxDetach = "\x1c"

// refusal says why k can't be bound, or nil. Each rule but the layout ones
// follows from the bytes k sends without the newer encoding: a binding must
// not fire on another key or on a terminal's reply, begin a longer sequence,
// or take zmx's detach key. Its errors complete a sentence whose subject is
// the key.
func refusal(k Key) error {
	if k.fn == 3 {
		return sameBytesAs("a cursor position report")
	}
	if k.fn != 0 {
		return nil
	}
	if err := layoutRefusal(k); err != nil {
		return err
	}
	if less, lost := lostModifier(k); lost {
		return sameBytesAs(strconv.Quote(less.String()))
	}
	return legacyRefusal(k)
}

// layoutRefusal turns down a key whose bytes depend on the keyboard layout:
// the CSI-u form names the unshifted key, which differs from layout to layout.
func layoutRefusal(k Key) error {
	switch {
	case k.mods&modShift != 0 && !isLetter(k.char):
		return errors.New("puts shift on a digit or punctuation mark, whose bytes depend on the keyboard layout")
	case strings.IndexByte(usShifted, k.char) >= 0:
		return errors.New("is typed with shift on a US layout, so its bytes depend on the keyboard layout")
	}
	return nil
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
	if name, ok := replyTerminators[seq]; ok {
		return fmt.Errorf("sends %s, which ends a terminal's reply", name)
	}
	last := seq[len(seq)-1]
	if name, ok := otherKeyBytes[last]; ok {
		if k.mods&modAlt != 0 {
			name = "alt+" + name
		}
		return sameBytesAs(name)
	}
	if aliases := ctrlAliases(k); len(aliases) > 0 {
		names := make([]string, 0, len(aliases))
		for _, a := range aliases {
			names = append(names, strconv.Quote(a.String()))
		}
		return sameBytesAs(strings.Join(names, " and "))
	}
	if intro, ok := introducers[last]; ok {
		return fmt.Errorf("is the start of %s", intro.name)
	}
	return nil
}

func sameBytesAs(other string) error {
	return fmt.Errorf("sends the same bytes as %s", other)
}
