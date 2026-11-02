package capture

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// FramesThemeSlug is the one theme every golden frame is drawn in. A literal
// rather than the shipped default: moving the default must not rewrite every
// frame.
const FramesThemeSlug = "tokyo-night"

// The kinds of file a frame is kept as, by its stem: its colours and its text.
const (
	ansiExt = ".ansi"
	textExt = ".txt"
)

// Size is a terminal's size in cells.
type Size struct {
	Width, Height int
}

// String is the size as a frame's stem ends with it, such as 120x40.
func (s Size) String() string {
	return fmt.Sprintf("%dx%d", s.Width, s.Height)
}

// harnessSize is the terminal a frame is drawn at where its screen is designed
// for no other: the size the package's tests have always rendered at.
var harnessSize = Size{Width: 120, Height: 40}

// Drawing is one named screen the harness draws, a picker fixture or a
// standalone surface, at the size its golden frame is kept at.
type Drawing struct {
	name string
	size Size
	// draw renders the screen at the size asked and reports the size it was
	// drawn at, which a screen that pins a dimension substitutes.
	draw func(th theme.Theme, colourless bool, size Size) (tea.View, Size)
}

func (d Drawing) Name() string { return d.name }

// Size is the terminal the drawing's frame is kept at.
func (d Drawing) Size() Size { return d.size }

// Stem is the drawing's frame's file name without its extension: its name and
// its size.
func (d Drawing) Stem() string { return d.name + "-" + d.size.String() }

// Frame draws the screen as a terminal of the given size shows it: a line per
// row, each ending in a newline, cut or padded to the height it was drawn at.
func (d Drawing) Frame(th theme.Theme, colourless bool, size Size) string {
	frame, _ := d.Draw(th, colourless, size)
	return frame
}

// Draw is Frame, with the background a terminal shows beneath the frame's
// unpainted cells: the colour the screen sets with OSC 11, or nil where it sets
// none and the terminal's own shows through.
func (d Drawing) Draw(th theme.Theme, colourless bool, size Size) (string, color.Color) {
	view, drawn := d.draw(th, colourless, size)
	lines := strings.Split(view.Content, "\n")
	lines = lines[:min(len(lines), drawn.Height)]
	for len(lines) < drawn.Height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n", view.BackgroundColor
}

// WriteFrame writes the drawing's frame, at its own size, in the frames' theme
// and in colour unless the drawing itself is colourless, into dir: <stem>.ansi,
// as drawn but for the newline after its last row, so cat in a terminal of its
// size doesn't scroll it; and <stem>.txt, its text, as Plain gives it.
func (d Drawing) WriteFrame(dir string) error {
	th, err := FramesTheme()
	if err != nil {
		return err
	}
	frame := d.Frame(th, false, d.size)
	stem := filepath.Join(dir, d.Stem())
	if err := os.WriteFile(stem+ansiExt, []byte(strings.TrimSuffix(frame, "\n")), 0o644); err != nil {
		return fmt.Errorf("write %s's colours: %w", d.Stem(), err)
	}
	if err := os.WriteFile(stem+textExt, []byte(Plain(frame)), 0o644); err != nil {
		return fmt.Errorf("write %s's text: %w", d.Stem(), err)
	}
	return nil
}

// FramesTheme is the palette every golden frame is drawn in.
func FramesTheme() (theme.Theme, error) {
	result, rejection, found := theme.NewSilentLoader().LoadBuiltin(FramesThemeSlug)
	switch {
	case !found:
		return theme.Theme{}, fmt.Errorf("the frames' theme %q is no built-in", FramesThemeSlug)
	case rejection != nil:
		return theme.Theme{}, fmt.Errorf("the frames' theme %q is not usable: %w", FramesThemeSlug, rejection)
	}
	return result.Theme, nil
}

// DrawingByName resolves every name FixtureNames lists: a picker fixture, the
// contrast swatch or a resume surface. Each call builds its world afresh.
func DrawingByName(name string) (Drawing, error) {
	d, ok := drawingSet(drawings())[name]
	if !ok {
		return Drawing{}, unknownFixture(name)
	}
	return d, nil
}

// Drawings are every screen with a golden frame, sorted by name, each built
// afresh.
func Drawings() []Drawing {
	all := drawings()
	slices.SortFunc(all, func(a, b Drawing) int { return strings.Compare(a.name, b.name) })
	return all
}

func drawings() []Drawing {
	builders := fixtureBuilders()
	surfaces := surfaceRegistry()
	all := make([]Drawing, 0, len(builders)+1+len(surfaces))
	for _, build := range builders {
		all = append(all, build().drawing())
	}
	all = append(all, swatchDrawing())
	for i := range surfaces {
		all = append(all, surfaces[i].drawing())
	}
	return all
}

// drawingSet indexes the drawings by name. It panics on a name given twice, as
// the second would be unreachable.
func drawingSet(all []Drawing) map[string]Drawing {
	set := make(map[string]Drawing, len(all))
	for _, d := range all {
		if _, dup := set[d.name]; dup {
			panic(fmt.Sprintf("capture: two drawings named %q", d.name))
		}
		set[d.name] = d
	}
	return set
}

// FramesIn are the stems of the frames dir keeps, by their text files, sorted.
func FramesIn(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*"+textExt))
	if err != nil {
		return nil, fmt.Errorf("list the frames in %s: %w", dir, err)
	}
	stems := make([]string, len(paths))
	for i, p := range paths {
		stems[i] = strings.TrimSuffix(filepath.Base(p), textExt)
	}
	return stems, nil
}

// Plain is a frame as text alone: its escape codes stripped, and each line's
// trailing spaces trimmed, each ending in a newline.
func Plain(frame string) string {
	lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(ansi.Strip(line), " ")
	}
	return strings.Join(lines, "\n") + "\n"
}
