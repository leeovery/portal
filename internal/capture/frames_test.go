package capture

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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
// worlds: anything left to chance, a clock, a map's order, shows as a
// difference.
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
	drawn, err := parse(d.Frame(th, false, d.Size()))
	if err != nil {
		t.Fatalf("parsing what %s draws: %v", d.Name(), err)
	}
	diffs, err := compare(frame, drawn, colourOf(th.Canvas))
	if err != nil {
		t.Fatalf("%s: %v", d.Stem(), err)
	}
	if len(diffs) > 0 {
		t.Error(describe(d.Stem(), diffs))
	}
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
