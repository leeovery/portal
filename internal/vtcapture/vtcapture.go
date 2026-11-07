// Package vtcapture reads a capture of a terminal's screen as zmx's History
// gives one: a run of mode sequences, then the screen's rows, then the screen
// state that closes it. It is the one reader of that format; its integration
// tests hold it to the zmx build. It depends on the standard library alone,
// so the saver reads captures without reaching the zmx driver.
package vtcapture

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
)

// Mode is a mode a capture's lead sets or resets.
type Mode struct {
	// Param is the parameter naming the mode, and Private reports a DEC
	// private mode (ESC [ ? n) rather than an ANSI one (ESC [ n).
	Param   string
	Private bool
	// Set reports that the lead sets the mode (h) rather than resets it (l).
	Set bool
}

// Modes returns the modes that lead capture, in the order it sets or resets
// them, and what follows them. zmx writes each mode off its default by a
// sequence of its own, the ANSI ones and then the DEC private ones, ahead of
// the screen's first cell. No cell opens with a mode sequence, so the lead
// ends at the first sequence that isn't one, and nothing after it is read as
// a mode.
func Modes(capture []byte) ([]Mode, []byte) {
	var modes []Mode
	for {
		sequence, rest, ok := leadingMode(capture)
		if !ok {
			return modes, capture
		}
		modes = append(modes, sequence...)
		capture = rest
	}
}

// leadingMode reads the mode sequence capture starts with: a mode for each of
// its parameters, and what follows it.
func leadingMode(capture []byte) ([]Mode, []byte, bool) {
	body, ok := bytes.CutPrefix(capture, []byte("\x1b["))
	if !ok {
		return nil, nil, false
	}
	body, private := bytes.CutPrefix(body, []byte("?"))
	end := bytes.IndexFunc(body, func(r rune) bool { return r != ';' && (r < '0' || r > '9') })
	if end < 0 || body[end] != 'h' && body[end] != 'l' {
		return nil, nil, false
	}
	var modes []Mode
	for param := range strings.SplitSeq(string(body[:end]), ";") {
		modes = append(modes, Mode{Param: param, Private: private, Set: body[end] == 'h'})
	}
	return modes, body[end+1:], true
}

// alternateScreenModes are the DEC private modes that put a terminal on its
// alternate screen.
var alternateScreenModes = []string{"47", "1047", "1049"}

// AlternateScreen reports whether capture is of a terminal on its alternate
// screen: its lead sets private mode 47, 1047 or 1049, and it holds that
// screen alone, none of the main screen and its scrollback.
func AlternateScreen(capture []byte) bool {
	modes, _ := Modes(capture)
	return slices.ContainsFunc(modes, func(m Mode) bool {
		return m.Private && m.Set && slices.Contains(alternateScreenModes, m.Param)
	})
}

// The parts of the closing state below that recur: a pen (SGR) and a
// hyperlink, opened or closed (OSC 8).
const (
	sgr  = `\x1b\[[\d;:]*m`
	osc8 = `\x1b\]8;[^\x1b]*\x1b\\`
)

// closingState is the screen state zmx writes after a capture's last row, as
// ghostty's formatter and zmx write it. Only the cursor's placement and its
// pen are always there; every other part is written when it is set.
var closingState = regexp.MustCompile(`` +
	// The terminal's: scrolling region, left and right margins, modifyOtherKeys.
	`(?:\x1b\[\d+;\d+r)?(?:\x1b\[\d+;\d+s)?(?:\x1b\[>4;2m)?` +
	// The cursor's placement. A pending wrap is restored by reprinting the
	// cell at the edge, or a blank there with a save, erase and restore.
	`\x1b\[\d+;\d+H(?: \x1b7\x1b\[X\x1b8|(?:` + sgr + `|` + osc8 + `)*[^\x1b\r\n]+(?:` + sgr + `|` + osc8 + `)*)?` +
	// The pen: a reset, then each attribute.
	`\x1b\[0m(?:` + sgr + `)*` +
	// The hyperlink, protection and kitty keyboard flags.
	`(?:` + osc8 + `)?(?:\x1b\[1"q)?(?:\x1b\[=\d+;1u)?` +
	// The character sets: G0 to G3 designations, then the GL and GR shifts.
	`(?:\x1b[()*+][AB0])*(?:\x0e|\x1b[no])?(?:\x1b[~|])?` +
	// zmx's own: the working directory.
	`(?:\x1b\]7;[^\x1b]*\x1b\\)?\z`)

// TrimClose returns capture without the screen state that closes it. That
// state places the cursor on the screen captured, so replayed into a terminal
// of another height, or after a clear that kept the scrollback, it lands
// among the rows. A capture not closed by that state, one not from zmx among
// them, is returned whole; no row places the cursor, so no row is cut.
// capture may be the end of a longer one.
func TrimClose(capture []byte) []byte {
	at := closingState.FindIndex(capture)
	if at == nil {
		return capture
	}
	return capture[:at[0]]
}
