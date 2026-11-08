// Package bgreply reads a terminal's reply to the OSC 11 background-colour
// query out of a pane's input: the draw that asks it, and the waiter its late
// replies reach, both take every reply out before judging what was typed. It
// depends on the standard library alone, since the waiter must not import the
// rendering path.
package bgreply

import (
	"bytes"
	"time"
)

// Introducer opens every reply: OSC, then 11 and its separator.
const Introducer = "\x1b]11;"

// TailWait bounds how long the rest of a reply is waited for once its head has
// arrived. A terminal writes its reply whole, so only the link between them
// splits one, and the rest follows within far less than this.
const TailWait = 100 * time.Millisecond

const (
	bel = 0x07
	esc = 0x1b
	st  = 0x9c
)

// Split parts input read from a pane into the background replies in it, the
// payload between each one's introducer and its terminator (BEL, ESC \ or the
// C1 ST) in order, and the rest, which was typed. Every reply is taken out, as
// every terminal showing the pane answers. A reply cut off at the end, still
// arriving, is taken out too and handed back as head, as is the start of an
// introducer there, though an escape alone at the end is the Escape key. A
// reply broken by an escape that ends nothing is taken out up to that escape,
// which stays in the rest.
func Split(in []byte) (rest []byte, payloads []string, head []byte) {
	rest = []byte{}
	for len(in) > 0 {
		before, after, found := bytes.Cut(in, []byte(Introducer))
		if !found {
			kept := withoutIntroducerStart(in)
			return append(rest, kept...), payloads, cutHead(in[len(kept):])
		}
		rest = append(rest, before...)
		payload, tail, ended := replyPayload(after)
		if !ended {
			return rest, payloads, cutHead(in[len(before):])
		}
		if payload != nil {
			payloads = append(payloads, *payload)
		}
		in = tail
	}
	return rest, payloads, nil
}

// headKept bounds a reply's head kept for its rest. A reply is some 30 bytes,
// so a longer head is no reply's: a terminal answering something else entirely
// without ever terminating it.
const headKept = 256

// Stream is a pane's input read a piece at a time, by a reader that discards
// it, so a reply split across two pieces is still taken out whole.
type Stream struct {
	unfinished []byte
}

// Take reads in, the next piece of the input. It answers how many replies in
// finished whole and whether in held a byte no reply accounts for, and keeps
// the end where a reply may still be arriving, cut off or an escape alone that
// may open one, to read with the piece that follows. A head too long for a
// reply's is typed.
func (s *Stream) Take(in []byte) (replies int, typed bool) {
	joined := append(s.unfinished, in...)
	rest, payloads, head := Split(joined)
	if head == nil && len(rest) > 0 && rest[len(rest)-1] == esc && joined[len(joined)-1] == esc {
		rest, head = rest[:len(rest)-1], []byte{esc}
	}
	if len(head) > headKept {
		rest, head = append(rest, head...), nil
	}
	s.unfinished = head
	return len(payloads), len(rest) > 0
}

// Unfinished reports whether the input read ends where a reply may still be
// arriving.
func (s *Stream) Unfinished() bool {
	return s.unfinished != nil
}

// Quiet is the input falling quiet: an escape alone at its end was the Escape
// key, and opens nothing, while a reply cut off is kept for its rest.
func (s *Stream) Quiet() {
	if len(s.unfinished) == 1 && s.unfinished[0] == esc {
		s.unfinished = nil
	}
}

// cutHead is nil for no head, and a copy otherwise, so a head kept to be
// finished never shares the input's memory.
func cutHead(head []byte) []byte {
	if len(head) == 0 {
		return nil
	}
	return bytes.Clone(head)
}

// replyPayload reads a reply past its introducer: its payload, nil for one an
// escape broke off, and what follows it. ended is false for a reply with no
// terminator yet.
func replyPayload(after []byte) (payload *string, tail []byte, ended bool) {
	for i, b := range after {
		switch b {
		case bel, st:
			text := string(after[:i])
			return &text, after[i+1:], true
		case esc:
			if i+1 == len(after) {
				return nil, nil, false
			}
			if after[i+1] == '\\' {
				text := string(after[:i])
				return &text, after[i+2:], true
			}
			return nil, after[i:], true
		}
	}
	return nil, nil, false
}

func withoutIntroducerStart(in []byte) []byte {
	for n := min(len(Introducer)-1, len(in)); n >= 2; n-- {
		if bytes.HasSuffix(in, []byte(Introducer[:n])) {
			return in[:len(in)-n]
		}
	}
	return in
}
