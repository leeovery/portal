package capture

import (
	"errors"
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// cell is a terminal's cell as a frame draws it: its glyph, in the pen it's
// drawn with.
type cell struct {
	glyph rune
	pen
}

// continuation is the glyph of the cell a wide glyph covers after its own, in
// the wide glyph's pen, as a terminal draws it. No decoded rune is negative.
const continuation rune = -1

func (c cell) String() string {
	if c.glyph == continuation {
		return fmt.Sprintf("a wide glyph's second cell on %s", c.bg)
	}
	weight := ""
	if c.bold {
		weight = " bold"
	}
	reversed := ""
	if c.reversed {
		reversed = " reversed"
	}
	link := ""
	if c.link != "" {
		link = " linked to " + c.link
	}
	return fmt.Sprintf("%q %s on %s%s%s%s", c.glyph, c.fg, c.bg, weight, reversed, link)
}

// pen is what a glyph is drawn in: its colours, whether it's bold, whether
// its colours are reversed, and the hyperlink it opens, if any.
type pen struct {
	fg, bg   colour
	bold     bool
	reversed bool
	link     string
}

// colour is a cell's 24-bit colour, or its palette index, or none where the
// frame leaves it to the terminal. A palette colour is the terminal's own to
// choose, so it is held as its index, never as an RGB it might show.
type colour struct {
	r, g, b uint8
	painted bool
	// palette marks index, not r, g and b, as the colour.
	palette bool
	index   uint8
}

func rgb(r, g, b uint8) colour {
	return colour{r: r, g: g, b: b, painted: true}
}

func paletteColour(index uint8) colour {
	return colour{index: index, palette: true, painted: true}
}

// colourOf is c as an SGR paints it, or unpainted where c is nil.
func colourOf(c color.Color) colour {
	if c == nil {
		return colour{}
	}
	r, g, b, _ := c.RGBA()
	return rgb(uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// String reads as a hex colour, or "default" where none is painted.
func (c colour) String() string {
	if !c.painted {
		return "default"
	}
	if c.palette {
		return fmt.Sprintf("palette %d", c.index)
	}
	return fmt.Sprintf("#%02X%02X%02X", c.r, c.g, c.b)
}

// or is c, or e where c isn't painted.
func (c colour) or(e colour) colour {
	if c.painted {
		return c
	}
	return e
}

// grid is a frame's cells, a row of them a line.
type grid [][]cell

// text is the grid's glyphs, a line a row, right-trimmed as a frame's .txt
// gives them.
func (g grid) text() []string {
	lines := make([]string, len(g))
	for i, row := range g {
		var line strings.Builder
		for _, c := range row {
			if c.glyph != continuation {
				line.WriteRune(c.glyph)
			}
		}
		lines[i] = strings.TrimRight(line.String(), " ")
	}
	return lines
}

// parse reads a frame's ANSI into its cells as a terminal draws them, one glyph
// a cell, each in the pen its SGR left set, carried from one cell to the next
// and across lines until reset, and in the OSC 8 hyperlink open over it until
// closed. A glyph two cells wide, by the width rule frames are drawn with,
// covers the cell after it too. A newline ends a row, and the frame's last may
// end in one or not. Anything else, an escape that is neither SGR nor a
// hyperlink, an SGR attribute but bold or reversed, a colour neither 24-bit nor a palette
// index, a control character, a glyph neither one cell wide nor two, or one of
// a cluster of code points, whose width a terminal takes from the cluster
// rather than from each, is an error, rather than read otherwise than a
// terminal shows it.
func parse(frame string) (grid, error) {
	g := grid{nil}
	var p pen
	for i := 0; i < len(frame); {
		r, size := utf8.DecodeRuneInString(frame[i:])
		var err error
		switch {
		case r == '\x1b':
			size, err = p.apply(frame[i:])
		case r == '\n':
			g = append(g, nil)
		case r == utf8.RuneError && size == 1:
			err = errors.New("not UTF-8")
		case r < ' ' || r == '\x7f':
			err = fmt.Errorf("control character %q", r)
		case clusterOfMany(frame[i:]):
			cluster, _ := ansi.FirstGraphemeCluster(frame[i:], ansi.GraphemeWidth)
			err = fmt.Errorf("%q is a cluster of %d code points, where a glyph is read as one", cluster, utf8.RuneCountInString(cluster))
		case ansi.StringWidth(string(r)) == 1:
			g[len(g)-1] = append(g[len(g)-1], cell{glyph: r, pen: p})
		case ansi.StringWidth(string(r)) == 2:
			g[len(g)-1] = append(g[len(g)-1], cell{glyph: r, pen: p}, cell{glyph: continuation, pen: p})
		default:
			err = fmt.Errorf("%q is %d cells wide, where a glyph takes one or two", r, ansi.StringWidth(string(r)))
		}
		if err != nil {
			return nil, fmt.Errorf("row %d, column %d: %w", len(g), len(g[len(g)-1])+1, err)
		}
		i += size
	}
	if len(g[len(g)-1]) == 0 {
		g = g[:len(g)-1]
	}
	return g, nil
}

// clusterOfMany reports whether the grapheme cluster s starts with holds more
// than one code point.
func clusterOfMany(s string) bool {
	cluster, _ := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
	return utf8.RuneCountInString(cluster) > 1
}

// apply sets the pen as the SGR or hyperlink sequence s starts with says, and
// returns the sequence's length in bytes.
func (p *pen) apply(s string) (int, error) {
	if strings.HasPrefix(s, hyperlinkIntro) {
		return p.hyperlink(s)
	}
	if !strings.HasPrefix(s, "\x1b[") {
		return 0, fmt.Errorf("escape %q isn't SGR", s[:min(len(s), 2)])
	}
	end := strings.IndexFunc(s[2:], func(r rune) bool { return r >= 0x40 && r <= 0x7e })
	if end < 0 {
		return 0, fmt.Errorf("escape %q never ends", s)
	}
	if s[2+end] != 'm' {
		return 0, fmt.Errorf("escape %q isn't SGR", s[:3+end])
	}
	return 3 + end, p.set(s[2 : 2+end])
}

// hyperlinkIntro opens an OSC 8 sequence: its parameters, a semicolon, then the
// URI, ended by BEL or ST.
const hyperlinkIntro = "\x1b]8;"

// hyperlink opens the link the OSC 8 sequence s starts with names over the
// cells after it, or closes the one open where it names none, and returns the
// sequence's length in bytes.
func (p *pen) hyperlink(s string) (int, error) {
	body := s[len(hyperlinkIntro):]
	end := strings.IndexAny(body, "\a\x1b")
	if end < 0 {
		return 0, fmt.Errorf("hyperlink %q never ends", s)
	}
	terminator := 1
	if body[end] == '\x1b' {
		if !strings.HasPrefix(body[end:], "\x1b\\") {
			return 0, fmt.Errorf("hyperlink %q isn't ended by BEL or ST", s[:len(hyperlinkIntro)+end+1])
		}
		terminator = 2
	}
	_, uri, ok := strings.Cut(body[:end], ";")
	if !ok {
		return 0, fmt.Errorf("hyperlink %q has no URI", s[:len(hyperlinkIntro)+end])
	}
	p.link = uri
	return len(hyperlinkIntro) + end + terminator, nil
}

// set sets the pen as the SGR parameters given say, in turn.
func (p *pen) set(params string) error {
	codes := strings.Split(params, ";")
	for i := 0; i < len(codes); i++ {
		n, err := attribute(codes[i])
		if err != nil {
			return fmt.Errorf("SGR %q: %w", params, err)
		}
		switch n {
		case 0:
			// A reset ends no hyperlink: only OSC 8 naming no URI does.
			*p = pen{link: p.link}
		case 1:
			p.bold = true
		case 22:
			p.bold = false
		case 7:
			p.reversed = true
		case 27:
			p.reversed = false
		case 39:
			p.fg = colour{}
		case 49:
			p.bg = colour{}
		case 38, 48:
			c, used, err := extendedColour(codes[i+1:])
			if err != nil {
				return fmt.Errorf("SGR %q: %w", params, err)
			}
			if n == 38 {
				p.fg = c
			} else {
				p.bg = c
			}
			i += used
		default:
			return fmt.Errorf("SGR %q: attribute %d isn't read", params, n)
		}
	}
	return nil
}

// attribute is the SGR parameter given, an empty one 0, as a terminal reads it.
func attribute(code string) (int, error) {
	if code == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(code)
	if err != nil {
		return 0, fmt.Errorf("parameter %q isn't a number", code)
	}
	return n, nil
}

// errColour is a colour SGR gives otherwise than in 24 bits or as a palette
// index.
var errColour = errors.New("its colour is neither 24-bit nor a palette index")

// extendedColour is the colour the SGR parameters after 38 or 48 give, and how
// many of them it takes: 2, then its red, green and blue, or 5, then its
// palette index. The 30–37 and 90–97 forms aren't read: on a terminal that
// shows bold as bright, bold 31 is not bold 38;5;1.
func extendedColour(codes []string) (colour, int, error) {
	switch {
	case len(codes) >= 4 && codes[0] == "2":
		var channels [3]uint8
		for i, code := range codes[1:4] {
			n, err := channel(code)
			if err != nil {
				return colour{}, 0, err
			}
			channels[i] = n
		}
		return rgb(channels[0], channels[1], channels[2]), 4, nil
	case len(codes) >= 2 && codes[0] == "5":
		n, err := channel(codes[1])
		return paletteColour(n), 2, err
	}
	return colour{}, 0, errColour
}

// channel is an SGR colour's part: a channel, or a palette index, 0 to 255.
func channel(code string) (uint8, error) {
	n, err := strconv.ParseUint(code, 10, 8)
	if err != nil {
		return 0, errColour
	}
	return uint8(n), nil
}

// difference is a cell, at its row and column counted from 1, that a fixture
// draws otherwise than its frame.
type difference struct {
	row, col     int
	frame, drawn cell
}

// compare lists the cells drawn differs from frame in. A cell whose background
// isn't painted shows the terminal's background, canvas: the colour the screen
// set with OSC 11, or unpainted where it set none, so there an unpainted cell
// matches only an unpainted one. A blank's foreground and weight aren't
// compared, as nothing shows them. A drawn row shorter than the frame's is
// filled to its width with blank, unpainted cells, as a terminal shows what
// wasn't written. The grids must have as many rows, and no drawn row may be
// wider.
func compare(frame, drawn grid, canvas colour) ([]difference, error) {
	if len(drawn) != len(frame) {
		return nil, fmt.Errorf("drawn %d rows, where the frame has %d", len(drawn), len(frame))
	}
	var diffs []difference
	for r := range frame {
		if len(drawn[r]) > len(frame[r]) {
			return nil, fmt.Errorf("row %d drawn %d columns, where the frame has %d", r+1, len(drawn[r]), len(frame[r]))
		}
		row := filled(drawn[r], len(frame[r]))
		for c := range frame[r] {
			if alike(frame[r][c], row[c], canvas) {
				continue
			}
			diffs = append(diffs, difference{row: r + 1, col: c + 1, frame: frame[r][c], drawn: row[c]})
		}
	}
	return diffs, nil
}

// filled is row, with blank, unpainted cells after it to width, which is no
// less than its own.
func filled(row []cell, width int) []cell {
	return slices.Concat(row, slices.Repeat([]cell{{glyph: ' '}}, width-len(row)))
}

// alike reports whether cells a and b look and act the same on a terminal
// whose background is canvas: a cell's link, blank or not, opens on a click,
// and a reversed blank shows its foreground as a block.
func alike(a, b cell, canvas colour) bool {
	if a.glyph != b.glyph || a.bg.or(canvas) != b.bg.or(canvas) || a.link != b.link || a.reversed != b.reversed {
		return false
	}
	inked := a.glyph != ' '
	return (!inked || a.bold == b.bold) && (!inked && !a.reversed || a.fg == b.fg)
}

// shown is how many cells a failure lists before it counts the rest.
const shown = 20

// describe says where a fixture draws otherwise than its frame, stem: a line a
// cell, for the first shown of them, then how many more.
func describe(stem string, diffs []difference) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s draws %d cells otherwise than its frame:", stem, len(diffs))
	for _, d := range diffs[:min(len(diffs), shown)] {
		fmt.Fprintf(&b, "\n  row %d, column %d: the frame's %s, drawn %s", d.row, d.col, d.frame, d.drawn)
	}
	if more := len(diffs) - shown; more > 0 {
		fmt.Fprintf(&b, "\n  and %d more", more)
	}
	return b.String()
}

func TestParseReadsSGRAsATerminalDoes(t *testing.T) {
	ink, canvas, other := rgb(192, 202, 245), rgb(11, 12, 20), rgb(1, 2, 3)
	tests := []struct {
		name, frame string
		want        grid
	}{
		{
			name:  "every cell painted, reset before each",
			frame: "\x1b[0;1;38;2;192;202;245;48;2;11;12;20mA\x1b[0;38;2;1;2;3;48;2;11;12;20mb \x1b[0m",
			want:  grid{slices.Concat(inked("A", pen{fg: ink, bg: canvas, bold: true}), inked("b ", pen{fg: other, bg: canvas, bold: false}))},
		},
		{
			name:  "Lip Gloss's, a run in one pen, then reset by an empty SGR",
			frame: "\x1b[1;38;2;192;202;245;48;2;11;12;20mAb\x1b[m c",
			want:  grid{slices.Concat(inked("Ab", pen{fg: ink, bg: canvas, bold: true}), inked(" c", pen{}))},
		},
		{
			name:  "each attribute carried, until changed alone",
			frame: "\x1b[1mA\x1b[38;2;1;2;3mB\x1b[22mC\x1b[48;2;11;12;20mD\x1b[39mE\x1b[49mF",
			want: grid{slices.Concat(
				inked("A", pen{bold: true}), inked("B", pen{fg: other, bold: true}), inked("C", pen{fg: other}),
				inked("D", pen{fg: other, bg: canvas, bold: false}), inked("E", pen{bg: canvas}), inked("F", pen{}),
			)},
		},
		{
			name:  "carried across lines",
			frame: "\x1b[1mA\nB\x1b[0m\nC",
			want:  grid{inked("A", pen{bold: true}), inked("B", pen{bold: true}), inked("C", pen{})},
		},
		{
			name:  "a last newline, ending the last row",
			frame: "A\n\nB\n",
			want:  grid{inked("A", pen{}), nil, inked("B", pen{})},
		},
		{
			name:  "a hyperlink ended by BEL, carried through a reset, closed by an empty one",
			frame: "A\x1b]8;;https://x.test\aB\x1b[1mC\x1b[mD\x1b]8;;\aE",
			want: grid{slices.Concat(
				inked("A", pen{}), inked("B", pen{link: "https://x.test"}), inked("C", pen{bold: true, link: "https://x.test"}),
				inked("D", pen{link: "https://x.test"}), inked("E", pen{}),
			)},
		},
		{
			name:  "a palette's colours, each held as its index",
			frame: "\x1b[38;5;1;48;5;236mA\x1b[39mB\x1b[1;38;5;255;49mC",
			want: grid{slices.Concat(
				inked("A", pen{fg: paletteColour(1), bg: paletteColour(236)}), inked("B", pen{bg: paletteColour(236)}),
				inked("C", pen{fg: paletteColour(255), bold: true}),
			)},
		},
		{
			name:  "a wide glyph covering the cell after it, in its pen",
			frame: "A\x1b[48;2;11;12;20m世\x1b[0mB",
			want: grid{slices.Concat(
				inked("A", pen{}),
				[]cell{{glyph: '世', pen: pen{bg: canvas}}, {glyph: continuation, pen: pen{bg: canvas}}},
				inked("B", pen{}),
			)},
		},
		{
			name:  "reversed, carried until turned off alone or reset",
			frame: "\x1b[7;38;2;1;2;3mA\x1b[1mB\x1b[27mC\x1b[7mD\x1b[0mE",
			want: grid{slices.Concat(
				inked("A", pen{fg: other, reversed: true}), inked("B", pen{fg: other, bold: true, reversed: true}),
				inked("C", pen{fg: other, bold: true}), inked("D", pen{fg: other, bold: true, reversed: true}), inked("E", pen{}),
			)},
		},
		{
			name:  "a hyperlink with parameters, ended by ST",
			frame: "\x1b]8;id=1;https://x.test\x1b\\A\x1b]8;;\x1b\\B",
			want:  grid{slices.Concat(inked("A", pen{link: "https://x.test"}), inked("B", pen{}))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(tt.frame)
			if err != nil {
				t.Fatalf("parse(%q) error = %v", tt.frame, err)
			}
			if !equal(got, tt.want) {
				t.Errorf("parse(%q) = %v, want %v", tt.frame, got, tt.want)
			}
		})
	}
}

func TestParseRefusesWhatItWouldReadOtherwiseThanATerminal(t *testing.T) {
	tests := []struct {
		name, frame, want string
	}{
		{name: "faint", frame: "\x1b[2mA", want: `row 1, column 1: SGR "2": attribute 2 isn't read`},
		{name: "underlined", frame: "AB\x1b[4mC", want: `row 1, column 3: SGR "4": attribute 4 isn't read`},
		{name: "italic", frame: "\x1b[3mA", want: `SGR "3": attribute 3 isn't read`},
		{name: "blinking, beside reversed", frame: "\x1b[7;5mA", want: `SGR "7;5": attribute 5 isn't read`},
		{name: "a basic colour", frame: "\x1b[31mA", want: `SGR "31": attribute 31 isn't read`},
		{name: "a bright colour", frame: "\x1b[102mA", want: "attribute 102 isn't read"},
		{name: "a colour cut short", frame: "\x1b[48;2;1;2mA", want: "neither 24-bit nor a palette index"},
		{name: "a channel past 255", frame: "\x1b[38;2;1;2;256mA", want: "neither 24-bit nor a palette index"},
		{name: "a palette colour with no index", frame: "\x1b[38;5mA", want: "neither 24-bit nor a palette index"},
		{name: "a palette index past 255", frame: "\x1b[48;5;256mA", want: "neither 24-bit nor a palette index"},
		{name: "a colour of another space", frame: "\x1b[38;3;1;2;3mA", want: "neither 24-bit nor a palette index"},
		{name: "a colour's parts apart by colons", frame: "\x1b[38:2::1:2:3mA", want: `parameter "38:2::1:2:3" isn't a number`},
		{name: "an escape that isn't SGR", frame: "A\n\x1b[2JB", want: `row 2, column 1: escape "\x1b[2J" isn't SGR`},
		{name: "a window title", frame: "\x1b]0;x\aA", want: `escape "\x1b]" isn't SGR`},
		{name: "a hyperlink never ended", frame: "\x1b]8;;https://x.testA", want: "never ends"},
		{name: "a hyperlink ended by another escape", frame: "\x1b]8;;x\x1b[mA", want: "isn't ended by BEL or ST"},
		{name: "a hyperlink with no URI", frame: "\x1b]8;x\aA", want: "has no URI"},
		{name: "an escape never ended", frame: "A\x1b[1", want: "never ends"},
		{name: "a tab", frame: "A\tB", want: `row 1, column 2: control character '\t'`},
		{name: "a carriage return", frame: "A\r\nB", want: `control character '\r'`},
		{name: "a combining mark", frame: "e\u0301", want: "row 1, column 1: \"e\u0301\" is a cluster of 2 code points, where a glyph is read as one"},
		{name: "an emoji with its modifier", frame: "A\U0001F44D\U0001F3FD", want: "row 1, column 2: \"\U0001F44D\U0001F3FD\" is a cluster of 2 code points"},
		{name: "a flag", frame: "\U0001F1EC\U0001F1E7", want: "is a cluster of 2 code points"},
		{name: "a modifier after a letter", frame: "a\U0001F3FD", want: "is a cluster of 2 code points"},
		{name: "a mark with no glyph before it", frame: "\u0301", want: "is 0 cells wide, where a glyph takes one or two"},
		{name: "bytes that aren't UTF-8", frame: "A\xff", want: "not UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := parse(tt.frame)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("parse(%q) = %v, error %v, want an error with %q", tt.frame, g, err, tt.want)
			}
		})
	}
}

func TestCompareHoldsEachCellToItsFrame(t *testing.T) {
	canvas := rgb(11, 12, 20)
	frame, err := parse("\x1b[0;1;38;2;192;202;245;48;2;11;12;20mAb\x1b[0;38;2;1;2;3;48;2;11;12;20m c\n" +
		"\x1b[0;38;2;1;2;3;48;2;40;36;58mde \x1b[0;38;2;1;2;3;48;2;11;12;20mf\x1b[0m")
	if err != nil {
		t.Fatalf("parse the frame: %v", err)
	}
	tests := []struct {
		name   string
		change func(g grid)
		// want are the cells found to differ, each as its row and column.
		want [][2]int
	}{
		{name: "the frame itself", change: func(grid) {}},
		{name: "a glyph changed", change: func(g grid) { g[0][1].glyph = 'B' }, want: [][2]int{{1, 2}}},
		{name: "a foreground a channel apart", change: func(g grid) { g[0][3].fg.g++ }, want: [][2]int{{1, 4}}},
		{name: "a background a channel apart", change: func(g grid) { g[1][1].bg.r-- }, want: [][2]int{{2, 2}}},
		{name: "bold changed", change: func(g grid) { g[1][3].bold = true }, want: [][2]int{{2, 4}}},
		{name: "a foreground left to the terminal", change: func(g grid) { g[0][0].fg = colour{} }, want: [][2]int{{1, 1}}},
		{name: "a background unpainted, on the canvas", change: func(g grid) { g[0][1].bg = colour{}; g[1][3].bg = colour{} }},
		{name: "a background unpainted, off the canvas", change: func(g grid) { g[1][0].bg = colour{} }, want: [][2]int{{2, 1}}},
		{name: "a blank's foreground and weight", change: func(g grid) { g[0][2].fg = rgb(9, 9, 9); g[1][2].bold = true }},
		{name: "reversed changed", change: func(g grid) { g[0][1].reversed = true }, want: [][2]int{{1, 2}}},
		{name: "a blank reversed", change: func(g grid) { g[0][2].reversed = true }, want: [][2]int{{1, 3}}},
		{name: "a blank's background", change: func(g grid) { g[0][2].bg = rgb(9, 9, 9) }, want: [][2]int{{1, 3}}},
		{name: "a glyph linked", change: func(g grid) { g[0][3].link = "https://x.test" }, want: [][2]int{{1, 4}}},
		{name: "a foreground the palette's", change: func(g grid) { g[0][3].fg = paletteColour(1) }, want: [][2]int{{1, 4}}},
		{name: "a blank linked", change: func(g grid) { g[0][2].link = "https://x.test" }, want: [][2]int{{1, 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn := clone(frame)
			tt.change(drawn)

			diffs, err := compare(frame, drawn, canvas)

			if err != nil {
				t.Fatalf("compare error = %v", err)
			}
			var got [][2]int
			for _, d := range diffs {
				got = append(got, [2]int{d.row, d.col})
				if want := (difference{row: d.row, col: d.col, frame: frame[d.row-1][d.col-1], drawn: drawn[d.row-1][d.col-1]}); d != want {
					t.Errorf("row %d, column %d is told as the frame's %s, drawn %s, want the frame's %s, drawn %s",
						d.row, d.col, d.frame, d.drawn, want.frame, want.drawn)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("compare found the cells %v differing, want %v", got, tt.want)
			}
		})
	}
}

// A text input's cursor is a reversed blank: its foreground is the block a
// terminal shows, so it is held where a plain blank's is not.
func TestCompareHoldsAReversedBlanksForeground(t *testing.T) {
	canvas, ink, other := rgb(11, 12, 20), rgb(255, 158, 100), rgb(1, 2, 3)
	frame := grid{inked(" ", pen{fg: ink, bg: canvas, reversed: true})}
	drawn := grid{inked(" ", pen{fg: other, bg: canvas, reversed: true})}

	diffs, err := compare(frame, drawn, canvas)

	if err != nil {
		t.Fatalf("compare error = %v", err)
	}
	if len(diffs) != 1 || diffs[0].col != 1 {
		t.Errorf("compare found %v differing, want the reversed blank at column 1", diffs)
	}
}

func TestCompareFillsAShortRowAsATerminalShowsIt(t *testing.T) {
	canvas, ink, other := rgb(11, 12, 20), rgb(192, 202, 245), rgb(40, 36, 58)
	tests := []struct {
		name  string
		frame []cell
		// want are the columns found to differ, each with the cell drawn there.
		want map[int]cell
	}{
		{name: "the frame blank on the canvas past it", frame: slices.Concat(inked("ab", pen{fg: ink, bg: canvas, bold: true}), inked("  ", pen{fg: ink, bg: canvas, bold: false}))},
		{
			name:  "a glyph in the frame past it",
			frame: slices.Concat(inked("ab", pen{fg: ink, bg: canvas, bold: true}), inked(" c", pen{fg: ink, bg: canvas, bold: false})),
			want:  map[int]cell{4: {glyph: ' '}},
		},
		{
			name:  "another background in the frame past it",
			frame: slices.Concat(inked("ab", pen{fg: ink, bg: canvas, bold: true}), inked(" ", pen{fg: ink, bg: canvas, bold: false}), inked(" ", pen{fg: ink, bg: other, bold: false})),
			want:  map[int]cell{4: {glyph: ' '}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drawn := grid{inked("ab", pen{fg: ink, bg: canvas, bold: true})}

			diffs, err := compare(grid{tt.frame}, drawn, canvas)

			if err != nil {
				t.Fatalf("compare error = %v", err)
			}
			got := map[int]cell{}
			for _, d := range diffs {
				got[d.col] = d.drawn
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("compare found %v differing, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareRefusesAGridOfAnotherSize(t *testing.T) {
	frame := grid{inked("ab", pen{}), inked("cd", pen{})}
	tests := []struct {
		name  string
		drawn grid
		want  string
	}{
		{name: "a row fewer", drawn: frame[:1], want: "drawn 1 rows, where the frame has 2"},
		{name: "a row more", drawn: append(clone(frame), nil), want: "drawn 3 rows, where the frame has 2"},
		{name: "a row wider", drawn: grid{inked("abc", pen{}), frame[1]}, want: "row 1 drawn 3 columns, where the frame has 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := compare(frame, tt.drawn, colour{}); err == nil || err.Error() != tt.want {
				t.Errorf("compare error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestDescribeNamesEachCellThenCountsTheRest(t *testing.T) {
	frame := cell{glyph: 'b', pen: pen{fg: rgb(1, 2, 3), bg: rgb(11, 12, 20)}}
	drawn := cell{glyph: 'x', pen: pen{bold: true}}
	diffs := make([]difference, shown+3)
	for i := range diffs {
		diffs[i] = difference{row: 2, col: i + 1, frame: frame, drawn: drawn}
	}

	got := strings.Split(describe("sessions-flat-120x40", diffs), "\n")

	want := []string{
		"sessions-flat-120x40 draws 23 cells otherwise than its frame:",
		"  row 2, column 1: the frame's 'b' #010203 on #0B0C14, drawn 'x' default on default bold",
	}
	if !slices.Equal(got[:2], want) {
		t.Errorf("describe starts\n%s\nwant\n%s", strings.Join(got[:2], "\n"), strings.Join(want, "\n"))
	}
	if last := got[len(got)-1]; len(got) != shown+2 || last != "  and 3 more" {
		t.Errorf("describe is %d lines, the last %q, want %d, the last %q", len(got), last, shown+2, "  and 3 more")
	}
	reversed := describe("sessions-filtering-120x40", []difference{{row: 5, col: 10, frame: cell{glyph: ' ', pen: pen{reversed: true}}, drawn: cell{glyph: ' '}}})
	if want := "  row 5, column 10: the frame's ' ' default on default reversed, drawn ' ' default on default"; !strings.HasSuffix(reversed, want) {
		t.Errorf("describe of a reversed cell says %q, want it to end %q", reversed, want)
	}
	if one := describe("sessions-flat-120x40", diffs[:1]); strings.Contains(one, "more") {
		t.Errorf("describe of one cell says %q, want no count of more", one)
	}
}

// inked is a row of text's cells, each drawn in p.
func inked(text string, p pen) []cell {
	var row []cell
	for _, r := range text {
		row = append(row, cell{glyph: r, pen: p})
	}
	return row
}

// equal reports whether a and b are the same cells, row for row.
func equal(a, b grid) bool {
	return slices.EqualFunc(a, b, func(x, y []cell) bool { return slices.Equal(x, y) })
}

// clone is a copy of g, each row its own.
func clone(g grid) grid {
	c := make(grid, len(g))
	for i, row := range g {
		c[i] = slices.Clone(row)
	}
	return c
}
