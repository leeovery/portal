package tui

import (
	"strings"
	"unicode/utf8"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// renderHistory lays a session's history, as the backend's History returns
// it, out as a terminal would draw it, one line per row in SGR alone, so the
// viewport can clip and scroll it without a cursor move or erase reaching the
// real terminal mid-frame.
//
// zmx serialises its screen through ghostty's VT formatter: rows of text and
// SGR joined by CR LF, a run of blank cells stepped over with CUF and a run
// of background-only cells erased with ECH then stepped over. Modes, such as
// the alternate screen, come before the rows; after them comes only state that
// leaves the screen as drawn: margins and keyboard modes, the cursor's CUP,
// under a pending wrap a reprint of the cell beneath the cursor, the pen and
// the cwd. The rows contain no CUP, so they end at the first one, and the
// reprint, which would otherwise land one cell past the row it repeats, is
// never drawn (internal/backend/zmx/testdata/history.vt holds one). Every
// other sequence draws nothing. Hyperlinks are dropped too, keeping their
// text, so none can run on past a clipped row.
func renderHistory(vt []byte) string {
	p := ansi.GetParser()
	defer ansi.PutParser(p)

	var s historyScreen
	var state byte
	for len(vt) > 0 {
		seq, width, n, next := ansi.DecodeSequence(vt, state, p)
		state, vt = next, vt[n:]
		if !s.apply(seq, width, p) {
			break
		}
	}
	return s.String()
}

// historyMaxColumns bounds how far a CUF or ECH count can reach: wider than any
// terminal a session runs in, so a corrupt count costs a row no wider than it.
const historyMaxColumns = 1024

// historyScreen is the grid renderHistory draws into. A row is as wide as what
// was drawn on it, and rows are added as the cursor moves down to draw on them.
type historyScreen struct {
	rows []uv.Line
	x, y int
	pen  uv.Style
	// glyph is the cell the sequence just applied drew, which a mark with no
	// width of its own, decoded apart from an ASCII base, joins.
	glyph struct {
		x  int
		ok bool
	}
}

// apply draws one decoded sequence, reporting false once the rows are over.
func (s *historyScreen) apply(seq []byte, width int, p *ansi.Parser) bool {
	glyph := s.glyph
	s.glyph.ok = false
	switch {
	case width > 0:
		s.set(s.x, uv.Cell{Content: string(seq), Width: width, Style: s.pen})
		s.glyph.x, s.glyph.ok = s.x, true
		s.x += width
	case glyph.ok && isMark(seq):
		s.join(glyph.x, string(seq))
		s.glyph = glyph
	case string(seq) == "\r":
		s.x = 0
	// A bare LF also returns the carriage, as a tty's ONLCR does, so text
	// joined by LF alone lays out the same as zmx's CR LF.
	case string(seq) == "\n":
		s.x, s.y = 0, s.y+1
	case ansi.HasCsiPrefix(seq):
		return s.csi(p)
	}
	return true
}

// csi applies a control sequence. The command carries any prefix or
// intermediate byte, so a CSI > 4 ; 2 m keyboard mode is never read as SGR.
func (s *historyScreen) csi(p *ansi.Parser) bool {
	switch p.Command() {
	case 'm':
		uv.ReadStyle(p.Params(), &s.pen)
	case 'C':
		s.x += s.csiCount(p)
	case 'X':
		erased := uv.Cell{Content: " ", Width: 1, Style: uv.Style{Bg: s.pen.Bg}}
		for i := range s.csiCount(p) {
			s.set(s.x+i, erased)
		}
	case 'H', 'f':
		return false
	}
	return true
}

// csiCount is a CUF or ECH count, where 0 and absent both mean 1, clamped so
// it reaches no further right than historyMaxColumns.
func (s *historyScreen) csiCount(p *ansi.Parser) int {
	n, _ := p.Param(0, 1)
	return max(0, min(max(1, n), historyMaxColumns-s.x))
}

// isMark reports whether seq is text that draws no cell of its own, such as a
// combining accent or the variation selector and keycap of 1️⃣. C1 controls
// sit below U+00A0, and a stray byte decodes as utf8.RuneError.
func isMark(seq []byte) bool {
	r, _ := utf8.DecodeRune(seq)
	return r >= 0xa0 && r != utf8.RuneError
}

// join adds mark to the cell at x, measuring the cluster afresh as the
// viewport will measure the row: a variation selector can widen it.
func (s *historyScreen) join(x int, mark string) {
	c := *s.rows[s.y].At(x)
	c.Content += mark
	c.Width = ansi.StringWidth(c.Content)
	s.set(x, c)
	s.x = x + c.Width
}

func (s *historyScreen) set(x int, c uv.Cell) {
	for len(s.rows) <= s.y {
		s.rows = append(s.rows, nil)
	}
	if grow := x + c.Width - len(s.rows[s.y]); grow > 0 {
		s.rows[s.y] = append(s.rows[s.y], uv.NewLine(grow)...)
	}
	s.rows[s.y].Set(x, &c)
}

func (s *historyScreen) String() string {
	lines := make([]string, len(s.rows))
	for i, row := range s.rows {
		lines[i] = row.Render()
	}
	return strings.Join(lines, "\n")
}
