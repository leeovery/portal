package zmx

import "regexp"

// The parts of the screen state below that recur: a pen (SGR) and a
// hyperlink, opened or closed (OSC 8).
const (
	sgr  = `\x1b\[[\d;:]*m`
	osc8 = `\x1b\]8;[^\x1b]*\x1b\\`
)

// screenState is the screen state zmx closes a History with, after the last
// row, as ghostty's formatter and zmx write it. Only the cursor placement and
// the pen are always there; every other part is written when it is set.
var screenState = regexp.MustCompile(`` +
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

// HistoryContent is history, as History answers it or a transcript saved from
// that holds it, without the screen state zmx closes it with. That state
// places the cursor on the screen it was captured from, so replayed into a
// terminal of another height, or after a clear that kept the scrollback, it
// lands among the rows. Without it a replay leaves the cursor where the last
// row ends. A history not closed by that state, one not from zmx among them,
// is returned whole; the body never places the cursor, so nothing in it is
// cut. history may be the end of a longer one.
func HistoryContent(history []byte) []byte {
	at := screenState.FindIndex(history)
	if at == nil {
		return history
	}
	return history[:at[0]]
}
