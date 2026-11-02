package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
)

// framesDir is where the golden frames are kept, each by its stem, as the text
// and ANSI its fixture draws.
const framesDir = "../../testdata/vhs/frames"

func TestFramesKeepEachFrameAsTextAndANSI(t *testing.T) {
	entries, err := os.ReadDir(framesDir)
	if err != nil {
		t.Fatalf("read %s: %v", framesDir, err)
	}
	kinds := map[string][]string{}
	for _, e := range entries {
		ext := filepath.Ext(e.Name())
		stem := strings.TrimSuffix(e.Name(), ext)
		kinds[stem] = append(kinds[stem], ext)
	}
	for stem, exts := range kinds {
		slices.Sort(exts)
		if want := []string{ansiExt, textExt}; !slices.Equal(exts, want) {
			t.Errorf("frames/ keeps %s as %v, want %v", stem, exts, want)
		}
	}
}

func TestEveryFrameIsDrawnByAFixture(t *testing.T) {
	drawn := map[string]bool{}
	for _, d := range Drawings() {
		drawn[d.Stem()] = true
	}
	for _, stem := range frameStems(t) {
		if !drawn[stem] {
			t.Errorf("no fixture draws %s at that size: regenerate its fixture's frame and delete this one", stem)
		}
	}
}

func TestEveryFixtureKeepsAFrame(t *testing.T) {
	stems := frameStems(t)
	for _, d := range Drawings() {
		if !slices.Contains(stems, d.Stem()) {
			t.Errorf("%s keeps no frame %s: go run ./cmd/capturetool --frames testdata/vhs/frames %s", d.Name(), d.Stem(), d.Name())
		}
	}
}

func TestEveryFixtureIsADrawing(t *testing.T) {
	names := make([]string, 0, len(Drawings()))
	for _, d := range Drawings() {
		names = append(names, d.Name())
	}
	if got, want := names, FixtureNames(); !slices.Equal(got, want) {
		t.Errorf("Drawings() are named %v, want every fixture and standalone surface %v", got, want)
	}
}

func TestEveryFramesANSIParsesToItsTextAtItsSize(t *testing.T) {
	for _, stem := range frameStems(t) {
		t.Run(stem, func(t *testing.T) {
			g, text := keptFrame(t, framesDir, stem)
			if got := g.text(); !slices.Equal(got, text) {
				t.Errorf("its ANSI reads\n%s\nwant its .txt\n%s", strings.Join(got, "\n"), strings.Join(text, "\n"))
			}
			size := sizeOf(t, stem)
			if len(g) != size.Height {
				t.Errorf("it's %d rows, want %d", len(g), size.Height)
			}
			for r, row := range g {
				if len(row) > size.Width {
					t.Errorf("row %d is %d columns, want %d at most", r+1, len(row), size.Width)
				}
			}
		})
	}
}

func TestEveryFixtureDrawsItsFrameCellForCell(t *testing.T) {
	th := framesTheme(t)
	for _, d := range Drawings() {
		t.Run(d.Stem(), func(t *testing.T) {
			frame, _ := keptFrame(t, framesDir, d.Stem())
			heldToItsFrame(t, d, th, frame)
		})
	}
}

// Every lookup builds its fixture's world afresh, so two lookups draw from two
// worlds. In one process, one environment and one clock, this catches state one
// draw leaves for the next and anything drawn in a map's order; a dependence on
// the environment or the time of day it cannot see.
func TestFramesAreDeterministic(t *testing.T) {
	th := framesTheme(t)
	for _, name := range FixtureNames() {
		t.Run(name, func(t *testing.T) {
			first, second := drawingNamed(t, name), drawingNamed(t, name)
			if a, b := first.Frame(th, false, first.Size()), second.Frame(th, false, second.Size()); a != b {
				t.Errorf("two frames of %s differ:\n%s\nthen\n%s", name, a, b)
			}
		})
	}
}

// A frame the capture tool writes is what the test holds its fixture to: a
// picker fixture, a geometry fixture and a surface, each read back from where
// it was written, its .txt its ANSI's text, and its ANSI ending on its last
// row's cells, with no newline after.
func TestAFrameWrittenFromItsFixtureIsHeldToIt(t *testing.T) {
	dir := t.TempDir()
	th := framesTheme(t)
	written := []string{"sessions-flat", "theme-panel-min-height-message", "resume-panel-waiting-degraded"}
	var stems []string
	for _, name := range written {
		d := drawingNamed(t, name)
		if err := d.WriteFrame(dir); err != nil {
			t.Fatalf("WriteFrame(%s): %v", name, err)
		}
		stems = append(stems, d.Stem())
	}
	slices.Sort(stems)
	if got, err := FramesIn(dir); err != nil || !slices.Equal(got, stems) {
		t.Errorf("FramesIn lists %v (%v), want %v", got, err, stems)
	}
	for _, name := range written {
		d := drawingNamed(t, name)
		t.Run(d.Stem(), func(t *testing.T) {
			frame, text := keptFrame(t, dir, d.Stem())
			heldToItsFrame(t, d, th, frame)
			if got := frame.text(); !slices.Equal(got, text) {
				t.Errorf("its ANSI reads\n%s\nwant its .txt\n%s", strings.Join(got, "\n"), strings.Join(text, "\n"))
			}
			ansi, err := os.ReadFile(filepath.Join(dir, d.Stem()+ansiExt))
			if err != nil {
				t.Fatalf("read %s's ANSI: %v", d.Stem(), err)
			}
			if strings.HasSuffix(string(ansi), "\n") {
				t.Error("its ANSI ends in a newline, want its last row's cells")
			}
		})
	}
}

func TestAStemNamesTheFixtureAndTheSizeItIsDrawnAt(t *testing.T) {
	tests := []struct {
		name, want string
	}{
		{name: "sessions-flat", want: "sessions-flat-" + harnessSize.String()},
		{name: "theme-panel-dir-unreadable", want: fmt.Sprintf("theme-panel-dir-unreadable-%dx%d", harnessSize.Width, themePanelDirUnreadableFrameHeight)},
		{name: "resume-panel-waiting", want: fmt.Sprintf("resume-panel-waiting-%dx%d", resumeSurfaceRoomyWidth, resumeSurfaceRoomyHeight)},
		{name: "resume-panel-discard-degraded", want: fmt.Sprintf("resume-panel-discard-degraded-%dx%d", resumeSurfaceDegradedWidth, resumeSurfaceDegradedHeight)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := drawingNamed(t, tt.name).Stem(); got != tt.want {
				t.Errorf("DrawingByName(%s).Stem() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// The declared render size wins over the frame size it would otherwise take,
// so the stem names the size the frame is truly drawn at.
func TestAGeometryFixturesStemIsItsDeclaredSize(t *testing.T) {
	fx, err := FixtureByName("theme-panel-min-height-message")
	if err != nil {
		t.Fatalf("FixtureByName: %v", err)
	}
	want := Size{Width: fx.width, Height: fx.height}
	if got := drawingNamed(t, fx.Name()).Size(); got != want {
		t.Errorf("theme-panel-min-height-message's frame size = %v, want its declared %v", got, want)
	}
}

func TestDrawingByNameRefusesAnUnknownName(t *testing.T) {
	_, err := DrawingByName("sessions-2")
	if err == nil || !strings.Contains(err.Error(), `unknown fixture "sessions-2"`) || !strings.Contains(err.Error(), "sessions-flat") {
		t.Errorf("DrawingByName(sessions-2) error = %v, want one naming it and listing the fixtures", err)
	}
}

func TestDrawingSetRefusesANameGivenTwice(t *testing.T) {
	defer func() {
		if got, want := recover(), `capture: two drawings named "sessions-flat"`; got != want {
			t.Errorf("drawingSet panicked with %v, want %q", got, want)
		}
	}()
	drawingSet([]Drawing{{name: "sessions-flat"}, {name: "projects"}, {name: "sessions-flat"}})
}

// The picker and the swatch set the terminal's background to their canvas with
// OSC 11; a resume surface is drawn into a pane and sets none, as a colourless
// fixture sets none.
func TestEachDrawingShowsTheTerminalBackgroundItSets(t *testing.T) {
	th := framesTheme(t)
	canvas := colourOf(th.Canvas.Color())
	tests := []struct {
		name string
		want colour
	}{
		{name: "sessions-flat", want: canvas},
		{name: "theme-panel-min-height-message", want: canvas},
		{name: ContrastValidationFixture, want: canvas},
		{name: "sessions-pending-resume-colourless", want: colour{}},
		{name: "resume-panel-waiting", want: colour{}},
		{name: "resume-panel-discard-degraded", want: colour{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := drawingNamed(t, tt.name)
			if _, got := d.Draw(th, false, d.Size()); colourOf(got) != tt.want {
				t.Errorf("%s shows the terminal background %s, want %s", tt.name, colourOf(got), tt.want)
			}
		})
	}
}

func TestAResumeSurfaceStrippedOfItsCanvasDiffers(t *testing.T) {
	th := framesTheme(t)
	d := drawingNamed(t, "resume-panel-waiting")
	frame, _ := keptFrame(t, framesDir, d.Stem())
	stripped, n := recoloured(frame, colourOf(th.Canvas.Color()), colour{})
	if n == 0 {
		t.Fatalf("%s paints no cell with the canvas, so stripping it proves nothing", d.Stem())
	}

	if diffs := drawnAgainst(t, d, th, stripped); len(diffs) != n {
		t.Errorf("%s with %d cells' canvas stripped differs in %d cells, want all %d — a pane draw sets no terminal background, so an unpainted cell is not the canvas", d.Stem(), n, len(diffs), n)
	}
}

func TestAColourlessFixturePaintedWithTheCanvasDiffers(t *testing.T) {
	th := framesTheme(t)
	canvas := colourOf(th.Canvas.Color())
	d := drawingNamed(t, "sessions-pending-resume-colourless")
	frame, _ := keptFrame(t, framesDir, d.Stem())
	painted, n := recoloured(frame, colour{}, canvas)
	if n == 0 {
		t.Fatalf("%s leaves no cell unpainted, so painting it proves nothing", d.Stem())
	}

	if diffs := drawnAgainst(t, d, th, painted); len(diffs) != n {
		t.Errorf("%s with %d cells painted with the canvas differs in %d cells, want all %d — a colourless screen sets no terminal background", d.Stem(), n, len(diffs), n)
	}
}

// The control for the two above: where the screen sets the terminal's
// background to its canvas, an unpainted cell shows it.
func TestThePickerStrippedOfItsCanvasDrawsAlike(t *testing.T) {
	th := framesTheme(t)
	d := drawingNamed(t, "sessions-flat")
	frame, _ := keptFrame(t, framesDir, d.Stem())
	stripped, n := recoloured(frame, colourOf(th.Canvas.Color()), colour{})
	if n == 0 {
		t.Fatalf("%s paints no cell with the canvas, so stripping it proves nothing", d.Stem())
	}

	if diffs := drawnAgainst(t, d, th, stripped); len(diffs) > 0 {
		t.Error(describe(d.Stem(), diffs))
	}
}

// A frame whose last row is unwritten is stored without it, so it is read
// back at its stem's height.
func TestAFrameWhoseLastRowIsUnwrittenIsHeld(t *testing.T) {
	dir := t.TempDir()
	d := Drawing{name: "blank-tail", size: Size{Width: 3, Height: 2}, draw: func(theme.Theme, bool, Size) (tea.View, Size) {
		return tea.NewView("ab"), Size{Width: 3, Height: 2}
	}}
	if err := d.WriteFrame(dir); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	frame, text := keptFrame(t, dir, d.Stem())

	if len(frame) != 2 {
		t.Errorf("read %d rows back, want the stem's 2", len(frame))
	}
	if want := []string{"ab", ""}; !slices.Equal(text, want) || !slices.Equal(frame.text(), want) {
		t.Errorf("read its text %q and its cells %q, want %q", text, frame.text(), want)
	}
	heldToItsFrame(t, d, framesTheme(t), frame)
}

// recoloured is g with every cell whose background is from given to instead,
// and how many it changed.
func recoloured(g grid, from, to colour) (grid, int) {
	changed := clone(g)
	n := 0
	for _, row := range changed {
		for i := range row {
			if row[i].bg == from {
				row[i].bg = to
				n++
			}
		}
	}
	return changed, n
}

func TestPlainStripsEscapesAndTrailingBlanks(t *testing.T) {
	got := Plain("\x1b[1;38;2;1;2;3mA \x1b[m  \n \x1b[48;2;1;2;3mb\x1b[m \n")
	if want := "A\n b\n"; got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

// heldToItsFrame holds what d draws, at its own size in the frames' theme, to
// frame, cell for cell, on the frames' canvas.
func heldToItsFrame(t *testing.T, d Drawing, th theme.Theme, frame grid) {
	t.Helper()
	if diffs := drawnAgainst(t, d, th, frame); len(diffs) > 0 {
		t.Error(describe(d.Stem(), diffs))
	}
}

// drawnAgainst lists the cells d, drawn at its own size in th, differs from
// frame in, on the background the terminal would show beneath it.
func drawnAgainst(t *testing.T, d Drawing, th theme.Theme, frame grid) []difference {
	t.Helper()
	ansi, background := d.Draw(th, false, d.Size())
	drawn, err := parse(ansi)
	if err != nil {
		t.Fatalf("parsing what %s draws: %v", d.Name(), err)
	}
	diffs, err := compare(frame, drawn, colourOf(background))
	if err != nil {
		t.Fatalf("%s: %v", d.Stem(), err)
	}
	return diffs
}

func framesTheme(t *testing.T) theme.Theme {
	t.Helper()
	th, err := FramesTheme()
	if err != nil {
		t.Fatalf("FramesTheme: %v", err)
	}
	return th
}

func drawingNamed(t *testing.T, name string) Drawing {
	t.Helper()
	d, err := DrawingByName(name)
	if err != nil {
		t.Fatalf("DrawingByName(%s): %v", name, err)
	}
	return d
}

// frameStems are the frames frames/ keeps, sorted.
func frameStems(t *testing.T) []string {
	t.Helper()
	stems, err := FramesIn(framesDir)
	if err != nil {
		t.Fatalf("FramesIn: %v", err)
	}
	if len(stems) == 0 {
		t.Fatal("frames/ keeps no frames")
	}
	return stems
}

// keptFrame is the frame dir keeps by the stem given, as its ANSI's cells and
// its .txt's lines.
func keptFrame(t *testing.T, dir, stem string) (grid, []string) {
	t.Helper()
	ansi, err := os.ReadFile(filepath.Join(dir, stem+ansiExt))
	if err != nil {
		t.Fatalf("read %s's ANSI: %v", stem, err)
	}
	txt, err := os.ReadFile(filepath.Join(dir, stem+textExt))
	if err != nil {
		t.Fatalf("read %s's text: %v", stem, err)
	}
	g, err := parse(string(ansi))
	if err != nil {
		t.Fatalf("parsing %s.ansi: %v", stem, err)
	}
	// A stored frame ends without its last newline, so an unwritten last row
	// leaves nothing to parse: the stem's height restores it.
	for len(g) < sizeOf(t, stem).Height {
		g = append(g, nil)
	}
	return g, strings.Split(strings.TrimSuffix(string(txt), "\n"), "\n")
}

// sizeOf is the size a frame's stem ends with, as in -120x40.
func sizeOf(t *testing.T, stem string) Size {
	t.Helper()
	suffix := stem[strings.LastIndex(stem, "-")+1:]
	var s Size
	if _, err := fmt.Sscanf(suffix, "%dx%d", &s.Width, &s.Height); err != nil || s.String() != suffix {
		t.Fatalf("%s ends with no size WxH: %q", stem, suffix)
	}
	return s
}
