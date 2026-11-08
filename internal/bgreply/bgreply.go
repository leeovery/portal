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

// Typed reports whether in, read so far, holds a byte no reply accounts for:
// one in no whole reply and in none cut off at its end, an escape alone there
// aside, since it may open the next.
func Typed(in []byte) bool {
	rest, _, head := Split(in)
	opens := len(head) == 0 && len(rest) == 1 && rest[0] == esc && in[len(in)-1] == esc
	return len(rest) > 0 && !opens
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
