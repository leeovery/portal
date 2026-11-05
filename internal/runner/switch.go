package runner

import (
	"slices"
	"syscall"
)

// screenClear is what zmx's client writes as it begins each attach.
const screenClear = "\x1b[0m\x1b[2J\x1b[H"

// zmx's client prints createdOpen, the session's name and createdClose as an
// attach makes the session it names (loop.zig's ensureSession), which a
// switch to a session that doesn't exist yet does between its reset and its
// screen clear.
const (
	createdOpen  = `session "`
	createdClose = "\" created\n"
)

// maxCreatedName bounds the name in zmx's created line: a session's name is
// part of its socket's path, which a unix socket's address holds less its
// NUL.
var maxCreatedName = len(syscall.RawSockaddrUnix{}.Path) - 1

// resetBorders holds, for each prefix of terminalReset, the length of the
// longest proper prefix of it that is also its suffix.
var resetBorders = borders(terminalReset)

// resetAfterClear holds, for each prefix of screenClear, how much of
// terminalReset that prefix ends with, so a clear broken off part-way leaves
// the reset matcher where its bytes would have.
var resetAfterClear = func() []int {
	states := make([]int, len(screenClear)+1)
	for i := range len(screenClear) {
		states[i+1] = advance(states[i], screenClear[i])
	}
	return states
}()

// switchPhase is how far into a switch the output read so far ends.
type switchPhase int

const (
	// seekingReset: before a reset, or past what broke one off.
	seekingReset switchPhase = iota
	// afterReset: a reset has just ended, and the clear or the created line
	// may follow.
	afterReset
	// inCreated: part of the created line follows the reset.
	inCreated
	// afterCreated: the created line has ended, and the clear must follow.
	afterCreated
)

// switchWatch finds what zmx's client writes as zmx switches it to another
// session in the same process, however the output's reads split it: the
// terminal reset it leaves a session with, the line it prints if the attach
// makes the session it switches to, then the screen clear it begins each
// attach with. A detach writes the reset alone, and an attach's start the
// clear alone. Nothing else of zmx's reaches the terminal between them: its
// logs go to its log file, and the daemon an attach forks writes to
// /dev/null. Anything else between the reset and the clear is no switch.
type switchWatch struct {
	phase switchPhase
	// reset is how much of terminalReset the output ends with, while seeking
	// one.
	reset int
	// clear is how much of screenClear follows the reset or the created line.
	clear int
	// created is the created line read so far.
	created []byte
}

// scan reads the next of the output, and reports whether a switch ends in it.
func (w *switchWatch) scan(p []byte) bool {
	return slices.ContainsFunc(p, w.step)
}

func (w *switchWatch) step(c byte) bool {
	switch w.phase {
	case afterReset:
		if w.clear == 0 && c == createdOpen[0] {
			w.phase, w.created = inCreated, append(w.created[:0], c)
			return false
		}
		return w.clearing(c)
	case inCreated:
		w.readCreated(c)
		return false
	case afterCreated:
		return w.clearing(c)
	}
	w.seek(advance(w.reset, c))
	return false
}

// seek seeks a reset, the output ending with reset bytes of one.
func (w *switchWatch) seek(reset int) {
	w.phase, w.reset, w.clear = seekingReset, reset, 0
	// No proper prefix of the reset is also its suffix, so a reset ended
	// leaves none of the next begun.
	if reset == len(terminalReset) {
		w.phase, w.reset = afterReset, 0
	}
}

// clearing reads c as the next of the clear, reporting whether it ends it. A
// byte that breaks it off is read as the reset matcher would have read it
// after the clear's bytes before it.
func (w *switchWatch) clearing(c byte) bool {
	if c != screenClear[w.clear] {
		w.seek(advance(resetAfterClear[w.clear], c))
		return false
	}
	w.clear++
	if w.clear < len(screenClear) {
		return false
	}
	w.seek(resetAfterClear[w.clear])
	return true
}

// readCreated reads c as the next of the created line. The line holds no ESC,
// which begins the reset, so none of its bytes can begin one, and a byte that
// breaks it off is read as the first after no reset.
func (w *switchWatch) readCreated(c byte) {
	w.created = append(w.created, c)
	switch line := w.created; {
	case len(line) <= len(createdOpen):
		if c != createdOpen[len(line)-1] {
			w.seek(advance(0, c))
		}
	case c == '\n':
		if createdLine(line) {
			w.phase, w.clear = afterCreated, 0
		} else {
			w.seek(advance(0, c))
		}
	case c == '\x1b' || len(line) > len(createdOpen)+maxCreatedName+len(createdClose)-1:
		w.seek(advance(0, c))
	}
}

// createdLine reports whether line, read up to its newline, is zmx's created
// line: its name, all the line holds between createdOpen and createdClose,
// one to maxCreatedName bytes.
func createdLine(line []byte) bool {
	name := len(line) - len(createdOpen) - len(createdClose)
	return name >= 1 && name <= maxCreatedName && string(line[len(line)-len(createdClose):]) == createdClose
}

// advance is how much of terminalReset the output ends with after c, when it
// ended with matched bytes of it before: a Knuth-Morris-Pratt step.
func advance(matched int, c byte) int {
	for matched > 0 && c != terminalReset[matched] {
		matched = resetBorders[matched-1]
	}
	if c == terminalReset[matched] {
		matched++
	}
	return matched
}

func borders(s string) []int {
	b := make([]int, len(s))
	for i, k := 1, 0; i < len(s); i++ {
		for k > 0 && s[i] != s[k] {
			k = b[k-1]
		}
		if s[i] == s[k] {
			k++
		}
		b[i] = k
	}
	return b
}
