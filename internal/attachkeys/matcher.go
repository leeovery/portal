package attachkeys

// scanState is where the matcher stands in the input: between sequences, or
// inside one a read left open.
type scanState uint8

const (
	ground scanState = iota
	inEscape
	inCSI
	inSS3
	inString
	// inStringEscape is an ESC inside a string: its terminator, or the start
	// of a new sequence.
	inStringEscape
	inPaste
)

const bel = 0x07

// maxCSIParams caps the parameter bytes kept for one CSI sequence; a longer
// one names no key.
const maxCSIParams = 64

const (
	pasteStartParams = "200"
	pasteEnd         = "\x1b[201~"
)

// Matcher finds bound keys in terminal input, one read at a time, judging each
// read in what the reads before it left open: a sequence, or a bracketed
// paste. A binding split across two reads is missed.
type Matcher struct {
	// c0 holds ctrl bindings, one C0 byte each.
	c0 map[byte]Action
	// escaped holds alt bindings by the byte after their ESC.
	escaped map[byte]Action
	// csi holds every binding's CSI-u form, and function keys' CSI form.
	csi map[csiKey]Action

	state scanState
	// start is where the open sequence began in this read, or -1 when an
	// earlier read began it: a sequence must begin in the read it ends in to
	// match.
	start           int
	params          []byte
	malformed       bool
	pasteEndMatched int
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

// hitKind is what a byte completed; the zero value is nothing.
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
	start  int
}

// Scan looks for a bound key in one read of terminal input. forward is the
// input before the first bound key, to pass on unchanged; found reports a
// key, and action is what it asks for. Input after the key is dropped:
// every action ends the attach. A bound key's repeat or release is cut from
// forward, and the scan goes on past it.
func (m *Matcher) Scan(read []byte) (forward []byte, action Action, found bool) {
	m.start = -1
	out := forwarder{read: read}
	for i, b := range read {
		h := m.step(b, i)
		switch h.kind {
		case pressed:
			return out.upTo(h.start), h.action, true
		case swallowed:
			out.drop(h.start, i+1)
		}
	}
	return out.upTo(len(read)), 0, false
}

func (m *Matcher) step(b byte, i int) hit {
	switch m.state {
	case inEscape:
		return m.afterEscape(b, i)
	case inCSI:
		return m.csiByte(b, i)
	case inSS3:
		m.ss3Byte(b, i)
	case inString:
		m.stringByte(b, i)
	case inStringEscape:
		return m.afterStringEscape(b, i)
	case inPaste:
		m.pasteByte(b)
	case ground:
		return m.groundByte(b, i)
	}
	return hit{}
}

func (m *Matcher) groundByte(b byte, i int) hit {
	if b == esc {
		m.escapeAt(i)
		return hit{}
	}
	if action, ok := m.c0[b]; ok {
		return hit{kind: pressed, action: action, start: i}
	}
	return hit{}
}

// escapeAt opens a sequence at an ESC. An ESC always does, ending whatever
// was open, as in a terminal's own parser.
func (m *Matcher) escapeAt(i int) {
	m.state = inEscape
	m.start = i
}

func (m *Matcher) afterEscape(b byte, i int) hit {
	if b == esc {
		m.escapeAt(i)
		return hit{}
	}
	if intro, ok := introducers[b]; ok {
		m.state = intro.state
		m.params = m.params[:0]
		m.malformed = false
		return hit{}
	}
	m.state = ground
	if action, ok := m.escaped[b]; ok && m.start >= 0 {
		return hit{kind: pressed, action: action, start: m.start}
	}
	return hit{}
}

func (m *Matcher) csiByte(b byte, i int) hit {
	switch {
	case b == esc:
		m.escapeAt(i)
	case b >= 0x30 && b <= 0x3f:
		m.param(b)
	case b >= 0x40 && b <= 0x7e:
		m.state = ground
		return m.csiFinal(b)
	default:
		// An intermediate byte, or one no CSI sequence holds: either way the
		// sequence names no key.
		m.malformed = true
	}
	return hit{}
}

func (m *Matcher) param(b byte) {
	if len(m.params) == maxCSIParams {
		m.malformed = true
		return
	}
	m.params = append(m.params, b)
}

// csiFinal judges a whole CSI sequence. A paste opens whichever read began
// its marker; a key must begin in this one.
func (m *Matcher) csiFinal(final byte) hit {
	if m.malformed {
		return hit{}
	}
	if final == '~' && string(m.params) == pasteStartParams {
		m.state = inPaste
		m.pasteEndMatched = 0
		return hit{}
	}
	if m.start < 0 {
		return hit{}
	}
	ev, ok := parseCSIKey(m.params, final)
	if !ok {
		return hit{}
	}
	action, ok := m.lookup(ev)
	switch {
	case !ok:
		return hit{}
	case ev.event == press:
		return hit{kind: pressed, action: action, start: m.start}
	default:
		return hit{kind: swallowed, action: action, start: m.start}
	}
}

// lookup finds the action a CSI key event is bound to. A punctuation mark
// typed with shift reports the unshifted key, so it is found by its shifted
// alternate when the terminal sends one.
func (m *Matcher) lookup(ev csiEvent) (Action, bool) {
	if action, ok := m.csi[ev.key]; ok {
		return action, true
	}
	if ev.shifted == 0 || ev.key.mods&modShift == 0 {
		return 0, false
	}
	action, ok := m.csi[csiKey{number: ev.shifted, final: 'u', mods: ev.key.mods &^ modShift}]
	return action, ok
}

// ss3Byte ends an SS3 sequence at the one byte it carries.
func (m *Matcher) ss3Byte(b byte, i int) {
	if b == esc {
		m.escapeAt(i)
		return
	}
	m.state = ground
}

// stringByte reads an OSC, DCS, SOS, PM or APC string, which BEL or ST ends.
func (m *Matcher) stringByte(b byte, i int) {
	switch b {
	case bel:
		m.state = ground
	case esc:
		m.state = inStringEscape
		m.start = i
	}
}

func (m *Matcher) afterStringEscape(b byte, i int) hit {
	if b == '\\' {
		m.state = ground
		return hit{}
	}
	m.state = inEscape
	return m.afterEscape(b, i)
}

// pasteByte watches a bracketed paste for its end marker, which may span
// reads. The marker holds one ESC, at its start, so a mismatch restarts there.
func (m *Matcher) pasteByte(b byte) {
	switch b {
	case pasteEnd[m.pasteEndMatched]:
		m.pasteEndMatched++
	case esc:
		m.pasteEndMatched = 1
	default:
		m.pasteEndMatched = 0
	}
	if m.pasteEndMatched == len(pasteEnd) {
		m.state = ground
	}
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
