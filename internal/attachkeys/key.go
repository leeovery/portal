package attachkeys

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// mods is a set of modifiers in the kitty keyboard protocol's bit layout, so a
// CSI-u modifier field compares against it directly.
type mods int

const (
	modShift mods = 1 << iota
	modAlt
	modCtrl
)

// modifierNames is also the order a key's canonical form names them in.
var modifierNames = []struct {
	name string
	mod  mods
}{
	{"ctrl", modCtrl},
	{"alt", modAlt},
	{"shift", modShift},
}

const esc = 0x1b

// Key is one bindable key: modifiers plus a character or a function key.
type Key struct {
	mods mods
	// char is a lowercase letter, a digit or a punctuation mark; zero for a
	// function key.
	char byte
	// fn is 1–12 for a function key.
	fn int
}

// String is the key's canonical form: lowercase, modifiers in a fixed order.
func (k Key) String() string {
	var b strings.Builder
	for _, m := range modifierNames {
		if k.mods&m.mod != 0 {
			b.WriteString(m.name + "+")
		}
	}
	if k.fn != 0 {
		b.WriteString("f" + strconv.Itoa(k.fn))
	} else {
		b.WriteByte(k.char)
	}
	return b.String()
}

func (k Key) without(m mods) Key {
	k.mods &^= m
	return k
}

// parseKey reads a key as written, case-insensitively. Its errors complete a
// sentence whose subject is the key.
func parseKey(text string) (Key, error) {
	parts := strings.Split(strings.ToLower(text), "+")
	// "+" is a key as well as the separator, so a trailing "++" names it.
	if n := len(parts); n >= 2 && parts[n-1] == "" && parts[n-2] == "" {
		parts = append(parts[:n-2], "+")
	}

	var k Key
	for _, part := range parts[:len(parts)-1] {
		m, err := modifierOf(part)
		if err != nil {
			return Key{}, err
		}
		if k.mods&m != 0 {
			return Key{}, fmt.Errorf("names %s twice", part)
		}
		k.mods |= m
	}
	if err := k.setKey(parts[len(parts)-1]); err != nil {
		return Key{}, err
	}
	if k.mods&(modCtrl|modAlt) == 0 {
		return Key{}, errors.New("needs ctrl or alt")
	}
	return k, nil
}

func modifierOf(part string) (mods, error) {
	for _, m := range modifierNames {
		if m.name == part {
			return m.mod, nil
		}
	}
	return 0, fmt.Errorf("has an unknown modifier %q", part)
}

func (k *Key) setKey(text string) error {
	if text == "" {
		return errors.New("has no key")
	}
	if len(text) == 1 && text[0] > ' ' && text[0] < 0x7f {
		k.char = text[0]
		return nil
	}
	for n := 1; n < len(fnCodes); n++ {
		if text == "f"+strconv.Itoa(n) {
			k.fn = n
			return nil
		}
	}
	return fmt.Errorf("has an unknown key %q", text)
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z'
}

// ctrlByte is the C0 byte a terminal sends for ctrl and c without the newer
// encoding: kitty's table, which Ghostty follows. A key it has no byte for
// loses its ctrl.
func ctrlByte(c byte) (byte, bool) {
	if isLetter(c) {
		return c - 'a' + 1, true
	}
	switch c {
	case '2', '@':
		return 0x00, true
	case '3', '[':
		return esc, true
	case '4', '\\':
		return 0x1c, true
	case '5', ']':
		return 0x1d, true
	case '6', '^', '~':
		return 0x1e, true
	case '7', '/', '_':
		return 0x1f, true
	case '8', '?':
		return 0x7f, true
	}
	return 0, false
}

// fnCodes is each function key's number and final byte in its CSI form, which
// both encodings share.
var fnCodes = [...]struct {
	number int
	final  byte
}{
	1: {1, 'P'}, 2: {1, 'Q'}, 3: {1, 'R'}, 4: {1, 'S'},
	5: {15, '~'}, 6: {17, '~'}, 7: {18, '~'}, 8: {19, '~'},
	9: {20, '~'}, 10: {21, '~'}, 11: {23, '~'}, 12: {24, '~'},
}

// csiKey is a key as a CSI sequence names it: the number before the
// modifiers, the final byte, and the intentional modifiers.
type csiKey struct {
	number int
	final  byte
	mods   mods
}

// csi is k's CSI-u form, or a function key's CSI form.
func (k Key) csi() csiKey {
	if k.fn != 0 {
		code := fnCodes[k.fn]
		return csiKey{number: code.number, final: code.final, mods: k.mods}
	}
	return csiKey{number: int(k.char), final: 'u', mods: k.mods}
}

// legacy is k's bytes without the newer encoding. It is only meaningful for a
// key whose ctrl has a byte and whose shift is a letter's, as refusal ensures.
func (k Key) legacy() string {
	if k.fn != 0 {
		code := fnCodes[k.fn]
		return fmt.Sprintf("\x1b[%d;%d%c", code.number, k.mods+1, code.final)
	}
	b := k.char
	switch {
	case k.mods&modCtrl != 0:
		b, _ = ctrlByte(k.char)
	case k.mods&modShift != 0:
		b = k.char - 'a' + 'A'
	}
	if k.mods&modAlt != 0 {
		return string([]byte{esc, b})
	}
	return string([]byte{b})
}
