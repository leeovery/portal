package main

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

func TestPrintsAFixturesFrameAsTextAtItsSize(t *testing.T) {
	out := capturing(t, "--fixture", "sessions-flat", "--print")

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 40 {
		t.Errorf("printed %d lines, want the frame's 40", len(lines))
	}
	if !strings.Contains(out, "P O R T A L") || !strings.Contains(out, "orchard-pipeline-codify") {
		t.Errorf("printed\n%s\nwant the sessions page", out)
	}
	if strings.ContainsRune(out, ansi.ESC) {
		t.Errorf("printed escape codes:\n%q\nwant text alone", out)
	}
	for i, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Errorf("line %d, %q, ends in spaces, want them trimmed", i+1, line)
		}
	}
}

func TestPrintsItsColoursWithAnsi(t *testing.T) {
	out := capturing(t, "--fixture", "sessions-flat", "--print", "--ansi")

	if !strings.ContainsRune(out, ansi.ESC) {
		t.Errorf("printed\n%s\nwant it in colour", out)
	}
	if text := capturing(t, "--fixture", "sessions-flat", "--print"); capture.Plain(out) != text {
		t.Errorf("in colour, the frame reads\n%s\nwant it to read as it does without\n%s", capture.Plain(out), text)
	}
}

func TestPrintsAtTheSizeGiven(t *testing.T) {
	out := capturing(t, "--fixture", "sessions-flat", "--print", "--size", "100x20")

	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 20 {
		t.Errorf("printed %d lines, want 20", len(lines))
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > 100 {
			t.Errorf("line %d is %d cells wide, want 100 at most", i+1, width)
		}
	}
}

// A surface pins its size, so the size asked for can't decide which of the
// card and the degraded stack is printed.
func TestPrintsAPinnedSurfaceAtItsOwnSize(t *testing.T) {
	out := capturing(t, "--fixture", "resume-panel-waiting-degraded", "--print", "--size", "120x40")

	if lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n"); len(lines) != 10 {
		t.Errorf("printed %d lines, want the surface's pinned 10", len(lines))
	}
}

func TestPrintsEveryStandaloneSurface(t *testing.T) {
	for _, name := range capture.StandaloneNames() {
		t.Run(name, func(t *testing.T) {
			if out := capturing(t, "--fixture", name, "--print"); strings.TrimSpace(out) == "" {
				t.Errorf("printed nothing for %s", name)
			}
		})
	}
}

func TestRefusesWhatTheCommandLineCantMean(t *testing.T) {
	available := fmt.Sprintf("(available: %v)", capture.FixtureNames())
	listed := "(available: " + strings.Join(capture.FixtureNames(), ", ") + ")"
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "no fixture printed", args: []string{"--print"}, want: "--fixture is required " + available},
		{name: "no fixture live", args: []string{}, want: "--fixture is required " + available},
		{name: "an unknown fixture", args: []string{"--fixture", "sessions-2", "--print"}, want: `unknown fixture "sessions-2" ` + listed},
		{name: "a size live", args: []string{"--fixture", "sessions-flat", "--size", "80x24"}, want: "--size takes --print: live, the terminal gives the size"},
		{name: "colours live", args: []string{"--fixture", "sessions-flat", "--ansi"}, want: "--ansi takes --print: live, the frame is drawn in colour"},
		{name: "an argument", args: []string{"--fixture", "sessions-flat", "--print", "now"}, want: `unexpected argument "now"`},
		{name: "a malformed size", args: []string{"--fixture", "sessions-flat", "--print", "--size", "120"}, want: `--size "120": give it as WxH, such as 120x40`},
		{name: "live without a terminal", args: []string{"--fixture", "sessions-flat"}, want: "the live view needs a terminal: --print draws without one"},
		{name: "frames for no fixture", args: []string{"--frames", t.TempDir(), "sessions-flat", "sessions-2"}, want: `--frames: unknown fixture "sessions-2" ` + listed},
		{name: "frames in another theme", args: []string{"--theme", "nord", "--frames", t.TempDir()}, want: "--frames takes the fixtures' names alone, not --theme"},
		{name: "frames printed", args: []string{"--frames", t.TempDir(), "--print", "--fixture", "sessions-flat"}, want: "--frames takes the fixtures' names alone, not --fixture, --print"},
		{name: "a flag after the frames' names", args: []string{"--frames", t.TempDir(), "sessions-flat", "--theme", "nord"}, want: "--frames takes the fixtures' names alone, not --theme"},
		{name: "frames into no directory", args: []string{"--frames=", "sessions-flat"}, want: "--frames takes the directory to write the frames into"},
		{name: "a flag there isn't", args: []string{"--fixture", "sessions-flat", "--nope"}, want: "flag provided but not defined: -nope (--help lists the flags)"},
		{name: "a flag's value missing", args: []string{"--fixture"}, want: "flag needs an argument: -fixture (--help lists the flags)"},
		{name: "a scenario and a fixture", args: []string{"--scenario", "portal-cold", "--fixture", "sessions-flat"}, want: "--scenario and --fixture: name one"},
		{name: "a scenario printed", args: []string{"--scenario", "portal-cold", "--print"}, want: "--print takes a fixture: a scenario plays in time, full screen"},
		{name: "a scenario at a size", args: []string{"--scenario", "portal-cold", "--size", "80x24"}, want: "--size takes a fixture's --print: a scenario plays at the terminal's size"},
		{name: "a scenario's colours", args: []string{"--scenario", "portal-cold", "--ansi"}, want: "--ansi takes a fixture's --print: a scenario plays in colour, full screen"},
		{name: "a scenario's frames", args: []string{"--frames", t.TempDir(), "--scenario", "portal-cold"}, want: "--frames takes the fixtures' names alone, not --scenario"},
		{name: "an unknown scenario", args: []string{"--scenario", "tour"}, want: `--scenario: unknown scenario "tour" (available: ` + strings.Join(capture.ScenarioNames(), ", ") + ")"},
		{name: "a scenario without a terminal", args: []string{"--scenario", "portal-cold"}, want: "a scenario plays full screen: it needs a terminal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(tt.args, &stdout, &stderr, noEnv)
			if err == nil || err.Error() != tt.want {
				t.Errorf("run(%q) error = %v, want %s", tt.args, err, tt.want)
			}
			if stdout.Len() > 0 {
				t.Errorf("run(%q) printed %q, want nothing", tt.args, stdout.String())
			}
			if stderr.Len() > 0 {
				t.Errorf("run(%q) wrote %q to stderr, want the refusal said once, by main, from its error", tt.args, stderr.String())
			}
		})
	}
}

func TestRefusesToPrintInAThemeThatDoesntLoad(t *testing.T) {
	broken := themetest.WriteWithCanvas(t, t.TempDir(), "broken.theme", "blue")
	tests := []struct {
		name, theme, want string
	}{
		{name: "no such built-in", theme: "solarized", want: `--theme "solarized" names no built-in theme`},
		{name: "a broken file", theme: broken, want: string(theme.ReasonBadColour)},
		{name: "no such file", theme: filepath.Join(t.TempDir(), "gone.theme"), want: string(theme.ReasonUnreadable)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run([]string{"--fixture", "sessions-flat", "--print", "--theme", tt.theme}, &stdout, &stderr, noEnv)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("run() error = %v, want one saying %s", err, tt.want)
			}
			if stdout.Len() > 0 {
				t.Errorf("run() printed %q, want nothing drawn in a theme it wasn't given", stdout.String())
			}
		})
	}
}

func TestPrintsInTheThemeGiven(t *testing.T) {
	path := themetest.WriteWithCanvas(t, t.TempDir(), "lake.theme", "#102030")
	tests := []struct {
		name string
		args []string
		// want is the canvas the frame is painted on, as a background's SGR.
		want string
	}{
		{name: "the shipped dark default, unless given", want: sgrBackground(themetest.Builtin(t, defaultThemeSlug).Canvas)},
		{name: "a built-in", args: []string{"--theme", "nord"}, want: sgrBackground(themetest.Builtin(t, "nord").Canvas)},
		{name: "a theme's file", args: []string{"--theme", path}, want: "48;2;16;32;48"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := capturing(t, append([]string{"--fixture", "sessions-flat", "--print", "--ansi"}, tt.args...)...)
			for i, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				if !strings.Contains(line, tt.want) {
					t.Fatalf("line %d is off the canvas %s: %q", i+1, tt.want, line)
				}
			}
		})
	}
}

func TestPrintsWithoutColourUnderNoColour(t *testing.T) {
	var stdout, stderr bytes.Buffer
	noColour := func(key string) string { return map[string]string{"NO_COLOR": "1"}[key] }
	if err := run([]string{"--fixture", "sessions-flat", "--print", "--ansi"}, &stdout, &stderr, noColour); err != nil {
		t.Fatalf("run: %v", err)
	}

	if out := stdout.String(); strings.Contains(out, "38;2;") || strings.Contains(out, "48;2;") {
		t.Errorf("under NO_COLOR, printed\n%q\nwant no colour, nor canvas", out)
	}
}

func TestPrintWarnsOfAThemeFilesNameOnStderr(t *testing.T) {
	path := themetest.Write(t, t.TempDir(), "nord.theme", themetest.Lines())
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--fixture", "sessions-flat", "--print", "--theme", path}, &stdout, &stderr, noEnv); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stderr.String(), "reserved name") {
		t.Errorf("stderr = %q, want the reserved-name warning", stderr.String())
	}
	if strings.Contains(stdout.String(), "warning") {
		t.Errorf("the warning reached stdout, where the frame is printed:\n%s", stdout.String())
	}
}

func TestWritesTheFramesNamedFromTheirFixtures(t *testing.T) {
	dir := t.TempDir()
	out := capturing(t, "--frames", dir, "sessions-flat", "resume-panel-waiting-degraded")

	if want := "wrote sessions-flat-120x40\nwrote resume-panel-waiting-degraded-40x10\n"; out != want {
		t.Errorf("printed %q, want %q", out, want)
	}
	for _, name := range []string{"sessions-flat", "resume-panel-waiting-degraded"} {
		heldAsWritten(t, dir, name)
	}
	if got, _ := capture.FramesIn(dir); len(got) != 2 {
		t.Errorf("wrote the frames %v, want the two named", got)
	}
}

func TestWritesEveryFixturesFrameWhereNoneIsNamed(t *testing.T) {
	dir := t.TempDir()
	capturing(t, "--frames", dir)

	got, err := capture.FramesIn(dir)
	if err != nil {
		t.Fatalf("FramesIn: %v", err)
	}
	if len(got) != len(capture.FixtureNames()) {
		t.Errorf("wrote %d frames, want one for each of the %d fixtures", len(got), len(capture.FixtureNames()))
	}
	heldAsWritten(t, dir, "contrast-validation")
}

// The frames are the frames' theme in colour, whatever the environment says.
func TestWritesFramesInColourUnderNoColour(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	noColour := func(key string) string { return map[string]string{"NO_COLOR": "1"}[key] }
	if err := run([]string{"--frames", dir, "sessions-flat"}, &stdout, &stderr, noColour); err != nil {
		t.Fatalf("run: %v", err)
	}
	heldAsWritten(t, dir, "sessions-flat")
}

func TestWritesNoFrameWhereOneNamedHasNoFixture(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--frames", dir, "sessions-flat", "sessions-2"}, &stdout, &stderr, noEnv); err == nil {
		t.Fatal("wrote frames where one named has no fixture, want an error")
	}
	if entries, _ := os.ReadDir(dir); len(entries) > 0 {
		t.Errorf("wrote %d files, want none", len(entries))
	}
}

func TestHelpListsEveryFixture(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--help"}, &stdout, &stderr, noEnv); err != nil {
		t.Fatalf("run(--help): %v", err)
	}
	for _, name := range capture.FixtureNames() {
		if !strings.Contains(stderr.String(), name) {
			t.Errorf("--help does not list %s", name)
		}
	}
}

func TestHelpListsEveryScenario(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--help"}, &stdout, &stderr, noEnv); err != nil {
		t.Fatalf("run(--help): %v", err)
	}
	for _, name := range capture.ScenarioNames() {
		if !strings.Contains(stderr.String(), name) {
			t.Errorf("--help does not list the scenario %s", name)
		}
	}
}

// A scenario plays in the theme given, painting its canvas, or without colour,
// painting none, as a fixture does.
func TestPlaysAScenarioInTheThemeGivenOrWithoutColour(t *testing.T) {
	nord := themetest.Builtin(t, "nord")
	tests := []struct {
		name       string
		colourless bool
		want       color.Color
	}{
		{name: "in the theme given", want: nord.Canvas.Color()},
		{name: "without colour", colourless: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, refusal, err := liveProgram(options{scenario: "portal-tour", theme: "nord"}, io.Discard, tt.colourless)
			if err != nil {
				t.Fatalf("liveProgram: %v", err)
			}
			if refusal != "a scenario plays full screen: it needs a terminal" {
				t.Errorf("without a terminal it would say %q, want a scenario's refusal", refusal)
			}
			if _, ok := m.(capture.ScenarioModel); !ok {
				t.Fatalf("it would play %T, want the scenario", m)
			}
			if canvas := m.View().BackgroundColor; !sameColour(canvas, tt.want) {
				t.Errorf("it paints the canvas %v, want %v", canvas, tt.want)
			}
		})
	}
}

// The program's end owes the terminal what a scenario's picker owes it, the
// background it painted set back, as a fixture's does.
func TestAScenariosEndRestoresWhatItsPickerOwes(t *testing.T) {
	m, _, err := liveProgram(options{scenario: "portal-tour", theme: "nord"}, io.Discard, false)
	if err != nil {
		t.Fatalf("liveProgram: %v", err)
	}
	if _, ok := unwrapped(m).(tui.Model); !ok {
		t.Errorf("a scenario's end is %T, want its picker, whose background the end restores", unwrapped(m))
	}
	fixture, _, err := liveProgram(options{fixture: "sessions-flat", theme: "nord"}, io.Discard, false)
	if err != nil {
		t.Fatalf("liveProgram: %v", err)
	}
	if _, ok := unwrapped(fixture).(tui.Model); !ok {
		t.Errorf("a fixture's end is %T, want the fixture's model itself", unwrapped(fixture))
	}
}

// sameColour reports whether two colours are both none, or the same red, green
// and blue.
func sameColour(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar == br && ag == bg && ab == bb
}

func TestDefaultsToTheShippedDarkTheme(t *testing.T) {
	o, err := parse([]string{"--fixture", "sessions-flat"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.theme != defaultThemeSlug {
		t.Errorf("--theme defaults to %q, want %q", o.theme, defaultThemeSlug)
	}
}

func TestReadsASizeGivenAsWxH(t *testing.T) {
	tests := []struct {
		given string
		want  capture.Size
	}{
		{given: "120x40", want: capture.Size{Width: 120, Height: 40}},
		{given: "40x10", want: capture.Size{Width: 40, Height: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.given, func(t *testing.T) {
			if got, err := parseSize(tt.given); err != nil || got != tt.want {
				t.Errorf("parseSize(%q) = %+v, %v, want %+v", tt.given, got, err, tt.want)
			}
		})
	}
}

func TestRefusesASizeNotGivenAsWxH(t *testing.T) {
	for _, given := range []string{"120", "120×40", "x40", "120x", "0x40", "120x-1", "wide x tall"} {
		t.Run(given, func(t *testing.T) {
			want := `--size "` + given + `": give it as WxH, such as 120x40`
			if _, err := parseSize(given); err == nil || err.Error() != want {
				t.Errorf("parseSize(%q) error = %v, want %s", given, err, want)
			}
		})
	}
}

// heldAsWritten fails unless dir keeps the named fixture's frame as it's drawn
// at its own size in the frames' theme: its colours, without the newline after
// the last row, and its text.
func heldAsWritten(t *testing.T, dir, name string) {
	t.Helper()
	d, err := capture.DrawingByName(name)
	if err != nil {
		t.Fatalf("DrawingByName(%s): %v", name, err)
	}
	th, err := capture.FramesTheme()
	if err != nil {
		t.Fatalf("FramesTheme: %v", err)
	}
	frame := d.Frame(th, false, d.Size())
	for ext, want := range map[string]string{".ansi": strings.TrimSuffix(frame, "\n"), ".txt": capture.Plain(frame)} {
		got, err := os.ReadFile(filepath.Join(dir, d.Stem()+ext))
		if err != nil {
			t.Fatalf("read %s%s: %v", d.Stem(), ext, err)
		}
		if string(got) != want {
			t.Errorf("%s%s reads\n%s\nwant\n%s", d.Stem(), ext, got, want)
		}
	}
}

// sgrBackground is the token painted as a background, as an SGR gives it.
func sgrBackground(tok theme.Token) string {
	r, g, b, _ := tok.Color().RGBA()
	return fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
}

// capturing runs the command with args, returning what it prints.
func capturing(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := run(args, &stdout, &stderr, noEnv); err != nil {
		t.Fatalf("run(%q): %v\n%s", args, err, stderr.String())
	}
	return stdout.String()
}

// noEnv is an environment with no variable set.
func noEnv(string) string { return "" }
