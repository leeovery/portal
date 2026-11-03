package attachkeys

import (
	"bytes"
	"strings"
)

// seqKind is what kind of sequence a run of input is.
type seqKind uint8

const (
	text seqKind = iota
	control
	// escaped is ESC and one byte: an alt key's legacy form, or an escape and
	// a key typed together.
	escaped
	csiSeq
	ss3Seq
	stringSeq
)

const bel = 0x07

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

// Matcher finds bound keys in terminal input, one read at a time. Each read is
// parsed afresh, so a sequence a read leaves open, or a stray opener, never
// reaches into the next; only a bracketed paste, which spans reads by nature,
// carries over. A binding split across two reads is missed.
//
// A legacy alt key is matched only as a read of its own, the one write a
// terminal makes for it, since ESC and a byte inside a read may be an escape
// and a key typed together. Two keys arriving as one read of exactly those two
// bytes still match.
type Matcher struct {
	// c0 holds ctrl bindings, one C0 byte each.
	c0 map[byte]Action
	// escaped holds alt bindings by the byte after their ESC.
	escaped map[byte]Action
	// csi holds every binding's CSI-u form, which modifyOtherKeys is read
	// into, and function keys' CSI form.
	csi map[csiKey]Action

	pasting bool
	// marker is how much of a paste marker the last read ended on: the
	// start's outside a paste, the end's inside one.
	marker int
}

// NewMatcher builds a matcher for these bindings.
func NewMatcher(b Bindings) *Matcher {
	m := &Matcher{
		c0:      map[byte]Action{},
		escaped: map[byte]Action{},
		csi:     map[csiKey]Action{},
	}
	for action, keys := range b {
		for _, k := range keys {
			m.bind(k, action)
		}
	}
	return m
}

func (m *Matcher) bind(k Key, action Action) {
	m.csi[k.csi()] = action
	if k.fn != 0 {
		return
	}
	if seq := k.legacy(); len(seq) == 1 {
		m.c0[seq[0]] = action
	} else {
		m.escaped[seq[1]] = action
	}
}

// hitKind is what a sequence asks of the scan; the zero value is nothing.
type hitKind uint8

const (
	pressed hitKind = iota + 1
	// swallowed is a bound key's repeat or release: kept from the inner
	// program, which never saw its press, and asking for nothing.
	swallowed
)

type hit struct {
	kind   hitKind
	action Action
}

// Scan looks for a bound key in one read of terminal input. forward is the
// input before the first bound key, to pass on unchanged; found reports a
// key, and action is what it asks for. Input after the key is dropped:
// every action ends the attach. A bound key's repeat or release is cut from
// forward, and the scan goes on past it.
func (m *Matcher) Scan(read []byte) (forward []byte, action Action, found bool) {
	i, continued := m.continuePasteStart(read)
	if continued {
		return read, 0, false
	}
	out := forwarder{read: read}
	for i < len(read) {
		if m.pasting {
			i = m.pasteFrom(read, i)
			continue
		}
		s := nextSequence(read, i)
		switch h := m.take(read, s); h.kind {
		case pressed:
			return out.upTo(s.start), h.action, true
		case swallowed:
			out.drop(s.start, s.end)
		}
		i = s.end
	}
	if !m.pasting {
		m.marker = pasteStartSuffix(read)
	}
	return out.upTo(len(read)), 0, false
}

// continuePasteStart finishes a paste's start marker the last read ended
// partway through. It returns where the scan goes on, and whether the read
// did no more than carry the marker on.
func (m *Matcher) continuePasteStart(read []byte) (int, bool) {
	if m.pasting || m.marker == 0 {
		return 0, false
	}
	rest := pasteStart[m.marker:]
	switch {
	case bytes.HasPrefix(read, []byte(rest)):
		m.pasting, m.marker = true, 0
		return len(rest), false
	case strings.HasPrefix(rest, string(read)):
		m.marker += len(read)
		return len(read), true
	}
	m.marker = 0
	return 0, false
}

// pasteStartSuffix is how much of a paste's start marker read ends on. Any
// part of the marker short of the whole is a sequence still open, so the
// bytes alone decide it.
func pasteStartSuffix(read []byte) int {
	for n := min(len(read), len(pasteStart)-1); n > 0; n-- {
		if bytes.HasSuffix(read, []byte(pasteStart[:n])) {
			return n
		}
	}
	return 0
}

// pasteFrom reads a bracketed paste from i to its end marker, which may span
// reads, and returns where the paste ends in this read. The marker holds one
// ESC, at its start, so a mismatch restarts there.
func (m *Matcher) pasteFrom(read []byte, i int) int {
	for ; i < len(read); i++ {
		switch read[i] {
		case pasteEnd[m.marker]:
			m.marker++
		case esc:
			m.marker = 1
		default:
			m.marker = 0
		}
		if m.marker == len(pasteEnd) {
			m.pasting, m.marker = false, 0
			return i + 1
		}
	}
	return i
}

// take judges one sequence of a read, and opens a paste at its start marker.
func (m *Matcher) take(read []byte, s sequence) hit {
	switch s.kind {
	case control:
		if action, ok := m.c0[read[s.start]]; ok {
			return hit{kind: pressed, action: action}
		}
	case escaped:
		if s.start != 0 || s.end != len(read) {
			return hit{}
		}
		if action, ok := m.escaped[read[s.start+1]]; ok {
			return hit{kind: pressed, action: action}
		}
	case csiSeq:
		return m.takeCSI(read, s)
	}
	return hit{}
}

func (m *Matcher) takeCSI(read []byte, s sequence) hit {
	if s.final == 0 {
		return hit{}
	}
	if string(read[s.start:s.end]) == pasteStart {
		m.pasting = true
		return hit{}
	}
	ev, ok := parseCSIKey(read[s.start+2:s.end-1], s.final)
	if !ok {
		return hit{}
	}
	action, ok := m.csi[ev.key]
	switch {
	case !ok:
		return hit{}
	case ev.event == press:
		return hit{kind: pressed, action: action}
	default:
		return hit{kind: swallowed, action: action}
	}
}

// sequence is one run of a read: a byte of text or control, or an escape
// sequence, whole or cut short by the read's end or by the next ESC.
type sequence struct {
	start, end int
	kind       seqKind
	// final is a whole, well-formed CSI sequence's final byte; zero otherwise.
	final byte
}

func nextSequence(read []byte, i int) sequence {
	switch b := read[i]; {
	case b == esc:
		return escapeSequence(read, i)
	case b < 0x20:
		return sequence{start: i, end: i + 1, kind: control}
	}
	return sequence{start: i, end: i + 1, kind: text}
}

// escapeSequence reads the sequence an ESC begins. An ESC always begins one,
// ending whatever came before it, as in a terminal's own parser.
func escapeSequence(read []byte, i int) sequence {
	if i+1 == len(read) || read[i+1] == esc {
		return sequence{start: i, end: i + 1, kind: text}
	}
	intro, ok := introducers[read[i+1]]
	switch {
	case !ok:
		return sequence{start: i, end: i + 2, kind: escaped}
	case intro.kind == csiSeq:
		return csiSequence(read, i)
	case intro.kind == ss3Seq:
		return ss3Sequence(read, i)
	}
	return stringSequence(read, i)
}

// csiSequence reads parameter and intermediate bytes up to a final byte. A
// byte no CSI sequence holds, or an intermediate, leaves it naming no key.
func csiSequence(read []byte, i int) sequence {
	s := sequence{start: i, kind: csiSeq}
	wellFormed := true
	for j := i + 2; j < len(read); j++ {
		switch b := read[j]; {
		case b == esc:
			s.end = j
			return s
		case b >= 0x40 && b <= 0x7e:
			s.end = j + 1
			if wellFormed {
				s.final = b
			}
			return s
		case b < 0x30 || b > 0x3f:
			wellFormed = false
		}
	}
	s.end = len(read)
	return s
}

// ss3Sequence reads ESC O and the one byte it carries.
func ss3Sequence(read []byte, i int) sequence {
	end := min(i+3, len(read))
	if end == i+3 && read[i+2] == esc {
		end = i + 2
	}
	return sequence{start: i, end: end, kind: ss3Seq}
}

// stringSequence reads an OSC, DCS, SOS, PM or APC string to the BEL or ST
// that ends it.
func stringSequence(read []byte, i int) sequence {
	s := sequence{start: i, kind: stringSeq}
	for j := i + 2; j < len(read); j++ {
		if read[j] == bel {
			s.end = j + 1
			return s
		}
		if read[j] != esc {
			continue
		}
		s.end = j
		if j+1 < len(read) && read[j+1] == '\\' {
			s.end = j + 2
		}
		return s
	}
	s.end = len(read)
	return s
}

// forwarder builds a read's forward bytes: the read itself, until a sequence
// is cut from it.
type forwarder struct {
	read []byte
	kept []byte
	from int
	cut  bool
}

func (f *forwarder) drop(start, end int) {
	f.kept = append(f.kept, f.read[f.from:start]...)
	f.from = end
	f.cut = true
}

func (f *forwarder) upTo(end int) []byte {
	if !f.cut {
		return f.read[:end]
	}
	return append(f.kept, f.read[f.from:end]...)
}
